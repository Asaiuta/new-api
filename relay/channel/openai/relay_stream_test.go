package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newOpenAIStreamTestContext() (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-test",
		},
		IsStream:    true,
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		StartTime:   time.Now(),
	}
	return c, recorder, info
}

func waitRecorderContains(t *testing.T, recorder *httptest.ResponseRecorder, want string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(recorder.Body.String(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for response body to contain %q; got %q", want, recorder.Body.String())
}

func TestOaiStreamHandlerForwardsFirstChunkWithoutWaitingForSecond(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	reader, writer := io.Pipe()
	c, recorder, info := newOpenAIStreamTestContext()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       reader,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	done := make(chan struct {
		errTotal int
		usageNil bool
	}, 1)
	go func() {
		usage, err := OaiStreamHandler(c, info, resp)
		errTotal := 0
		if err != nil {
			errTotal = 1
		}
		done <- struct {
			errTotal int
			usageNil bool
		}{errTotal: errTotal, usageNil: usage == nil}
	}()

	firstChunk := `{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1781359200,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"}}]}`
	_, err := io.WriteString(writer, "data: "+firstChunk+"\n\n")
	require.NoError(t, err)

	waitRecorderContains(t, recorder, `"hello"`, 500*time.Millisecond)
	require.NotContains(t, recorder.Body.String(), `"usage"`)

	usageChunk := `{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1781359200,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`
	_, err = io.WriteString(writer, "data: "+usageChunk+"\n\n")
	require.NoError(t, err)
	_, err = io.WriteString(writer, "data: [DONE]\n\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	result := <-done
	require.Zero(t, result.errTotal)
	require.False(t, result.usageNil)
	require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	require.NotContains(t, recorder.Body.String(), `"prompt_tokens":3`)
}

func BenchmarkOpenAIStreamForwardingStrategy(b *testing.B) {
	chunks := []string{
		`{"id":"chatcmpl-test","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`{"id":"chatcmpl-test","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`{"id":"chatcmpl-test","choices":[{"index":0,"delta":{"content":" world"}}]}`,
	}

	b.Run("legacy_buffer_previous_chunk", func(b *testing.B) {
		c, recorder, _ := newOpenAIStreamTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			var last string
			for _, chunk := range chunks {
				if last != "" {
					if err := helper.WriteStringData(c, last); err != nil {
						b.Fatal(err)
					}
				}
				last = chunk
			}
			if err := helper.WriteStringData(c, last); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("immediate_current_chunk", func(b *testing.B) {
		c, recorder, _ := newOpenAIStreamTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			for _, chunk := range chunks {
				if err := helper.WriteStringData(c, chunk); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}
