package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 4 << 10   // 4KB; grows on large SSE lines up to the configured max.
	DefaultMaxScannerBufferSize = 128 << 20 // 128MB default SSE buffer size.
	DefaultPingInterval         = 10 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

func NewStreamScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	return scanner
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string, sr *StreamResult)) {

	if resp == nil || dataHandler == nil {
		return
	}

	// 无条件新建 StreamStatus
	info.StreamStatus = relaycommon.NewStreamStatus()

	// 确保响应体总是被关闭
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second

	var (
		scanner    = NewStreamScanner(resp.Body)
		timeout    = time.NewTimer(streamingTimeout)
		pingTicker *time.Ticker
		dataChan   = make(chan string, 10)
		scanDone   = make(chan struct{})
	)
	defer timeout.Stop()

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	logger.LogDebug(c, "relay timeout seconds: %d", common.RelayTimeout)
	logger.LogDebug(c, "relay max idle conns: %d", common.RelayMaxIdleConns)
	logger.LogDebug(c, "relay max idle conns per host: %d", common.RelayMaxIdleConnsPerHost)
	logger.LogDebug(c, "streaming timeout seconds: %d", int64(streamingTimeout.Seconds()))
	logger.LogDebug(c, "ping interval seconds: %d", int64(pingInterval.Seconds()))

	defer func() {
		if pingTicker != nil {
			pingTicker.Stop()
		}
	}()

	scanner.Split(bufio.ScanLines)
	SetEventStreamHeaders(c)

	baseCtx := context.Background()
	if c != nil && c.Request != nil {
		baseCtx = c.Request.Context()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	// Scanner goroutine with improved error handling
	common.RelayCtxGo(ctx, func() {
		defer func() {
			close(dataChan)
			close(scanDone)
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("scanner panic: %v", r))
			}
			logger.LogDebug(c, "scanner goroutine exited")
		}()

		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-ctx.Done():
				return
			default:
			}

			data := scanner.Text()
			logger.LogDebug(c, "stream scanner data: %s", data)

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}
			if !strings.HasPrefix(data, "[DONE]") {
				info.SetFirstResponseTime()
				info.ReceivedResponseCount++

				select {
				case dataChan <- data:
				case <-ctx.Done():
					return
				}
			} else {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				logger.LogDebug(c, "received [DONE], stopping scanner")
				return
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
			}
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	})

	resetTimeout := func() {
		if !timeout.Stop() {
			select {
			case <-timeout.C:
			default:
			}
		}
		timeout.Reset(streamingTimeout)
	}

	scannerDone := scanDone
	sr := newStreamResult(info.StreamStatus)
	running := true
	for running {
		select {
		case <-timeout.C:
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
			running = false
		case <-ctx.Done():
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, ctx.Err())
			running = false
		case <-scanDone:
			scanDone = nil
			if dataChan == nil {
				running = false
			}
		case data, ok := <-dataChan:
			if !ok {
				dataChan = nil
				if scanDone == nil {
					running = false
				}
				continue
			}
			resetTimeout()
			sr.reset()
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogError(c, fmt.Sprintf("data handler panic: %v", r))
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", r))
					}
				}()
				dataHandler(data, sr)
			}()
			if sr.IsStopped() || info.StreamStatus.EndReason == relaycommon.StreamEndReasonPanic {
				running = false
			}
		case <-pingTickerChan(pingTicker):
			if err := PingData(c); err != nil {
				logger.LogError(c, "ping data error: "+err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
				running = false
			} else {
				logger.LogDebug(c, "ping data sent")
			}
		}
	}

	cancel()
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	select {
	case <-scannerDone:
	case <-time.After(5 * time.Second):
		logger.LogError(c, "timeout waiting for scanner goroutine to exit")
	}

	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, fmt.Sprintf("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
}

func pingTickerChan(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
