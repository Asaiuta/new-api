package helper

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func FlushWriter(c *gin.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flush panic recovered: %v", r)
		}
	}()

	if c == nil || c.Writer == nil {
		return nil
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming error: flusher not found")
	}

	flusher.Flush()
	return nil
}

func SetEventStreamHeaders(c *gin.Context) {
	// 检查是否已经设置过头部
	if _, exists := c.Get("event_stream_headers_set"); exists {
		return
	}

	// 设置标志，表示头部已经设置过
	c.Set("event_stream_headers_set", true)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

func ClaudeData(c *gin.Context, resp dto.ClaudeResponse) error {
	jsonData, err := common.Marshal(resp)
	if err != nil {
		common.SysError("error marshalling stream response: " + err.Error())
	} else {
		if err := WriteEventBytesData(c, resp.Type, jsonData); err != nil {
			return err
		}
	}
	_ = FlushWriter(c)
	return nil
}

func ClaudeChunkData(c *gin.Context, resp dto.ClaudeResponse, data string) {
	_ = WriteEventStringData(c, resp.Type, data)
	_ = FlushWriter(c)
}

func ResponseChunkData(c *gin.Context, resp dto.ResponsesStreamResponse, data string) {
	_ = WriteEventStringData(c, resp.Type, data)
	_ = FlushWriter(c)
}

func StringData(c *gin.Context, str string) error {
	if err := WriteStringData(c, str); err != nil {
		return err
	}
	return FlushWriter(c)
}

func BytesData(c *gin.Context, data []byte) error {
	if err := WriteBytesData(c, data); err != nil {
		return err
	}
	return FlushWriter(c)
}

func EventStringData(c *gin.Context, event, data string) error {
	if err := WriteEventStringData(c, event, data); err != nil {
		return err
	}
	return FlushWriter(c)
}

func EventBytesData(c *gin.Context, event string, data []byte) error {
	if err := WriteEventBytesData(c, event, data); err != nil {
		return err
	}
	return FlushWriter(c)
}

func WriteStringData(c *gin.Context, str string) error {
	if err := validateStreamWriter(c); err != nil {
		return err
	}

	writeFastEventStreamHeaders(c)
	return writeStringPayload(c, str)
}

func WriteBytesData(c *gin.Context, data []byte) error {
	if err := validateStreamWriter(c); err != nil {
		return err
	}

	writeFastEventStreamHeaders(c)
	return writeBytesPayload(c, data)
}

func WriteEventStringData(c *gin.Context, event, data string) error {
	if err := validateStreamWriter(c); err != nil {
		return err
	}

	writeFastEventStreamHeaders(c)
	if err := writeEventName(c, event); err != nil {
		return err
	}
	return writeStringPayload(c, data)
}

func WriteEventBytesData(c *gin.Context, event string, data []byte) error {
	if err := validateStreamWriter(c); err != nil {
		return err
	}

	writeFastEventStreamHeaders(c)
	if err := writeEventName(c, event); err != nil {
		return err
	}
	return writeBytesPayload(c, data)
}

func validateStreamWriter(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}
	return nil
}

func writeStringPayload(c *gin.Context, str string) error {
	if _, err := io.WriteString(c.Writer, "data: "); err != nil {
		return fmt.Errorf("write stream data prefix failed: %w", err)
	}
	if strings.Contains(str, "\r") {
		str = strings.ReplaceAll(str, "\r", "\\r")
	}
	if _, err := io.WriteString(c.Writer, str); err != nil {
		return fmt.Errorf("write stream data failed: %w", err)
	}
	if _, err := io.WriteString(c.Writer, "\n\n"); err != nil {
		return fmt.Errorf("write stream data suffix failed: %w", err)
	}
	return nil
}

func writeBytesPayload(c *gin.Context, data []byte) error {
	if _, err := io.WriteString(c.Writer, "data: "); err != nil {
		return fmt.Errorf("write stream data prefix failed: %w", err)
	}
	if bytes.IndexByte(data, '\r') >= 0 {
		data = bytes.ReplaceAll(data, []byte{'\r'}, []byte("\\r"))
	}
	if _, err := c.Writer.Write(data); err != nil {
		return fmt.Errorf("write stream data failed: %w", err)
	}
	if _, err := io.WriteString(c.Writer, "\n\n"); err != nil {
		return fmt.Errorf("write stream data suffix failed: %w", err)
	}
	return nil
}

func writeEventName(c *gin.Context, event string) error {
	if event == "" {
		return nil
	}
	if _, err := io.WriteString(c.Writer, "event: "); err != nil {
		return fmt.Errorf("write stream event prefix failed: %w", err)
	}
	if strings.ContainsAny(event, "\r\n") {
		event = strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(event)
	}
	if _, err := io.WriteString(c.Writer, event); err != nil {
		return fmt.Errorf("write stream event failed: %w", err)
	}
	if _, err := io.WriteString(c.Writer, "\n"); err != nil {
		return fmt.Errorf("write stream event suffix failed: %w", err)
	}
	return nil
}

func writeFastEventStreamHeaders(c *gin.Context) {
	header := c.Writer.Header()
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "text/event-stream")
	}
	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "no-cache")
	}
}

func PingData(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	if _, err := c.Writer.Write([]byte(": PING\n\n")); err != nil {
		return fmt.Errorf("write ping data failed: %w", err)
	}
	return FlushWriter(c)
}

func ObjectData(c *gin.Context, object interface{}) error {
	if object == nil {
		return errors.New("object is nil")
	}
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	return BytesData(c, jsonData)
}

func Done(c *gin.Context) {
	_ = StringData(c, "[DONE]")
}

func WssString(c *gin.Context, ws *websocket.Conn, str string) error {
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", str))
	return ws.WriteMessage(1, []byte(str))
}

func WssObject(c *gin.Context, ws *websocket.Conn, object interface{}) error {
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", jsonData))
	return ws.WriteMessage(1, jsonData)
}

func WssError(c *gin.Context, ws *websocket.Conn, openaiError types.OpenAIError) {
	if ws == nil {
		return
	}
	errorObj := &dto.RealtimeEvent{
		Type:    "error",
		EventId: GetLocalRealtimeID(c),
		Error:   &openaiError,
	}
	_ = WssObject(c, ws, errorObj)
}

func GetResponseID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("chatcmpl-%s", logID)
}

func GetLocalRealtimeID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("evt_%s", logID)
}

func GenerateStartEmptyResponse(id string, createAt int64, model string, systemFingerprint *string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: systemFingerprint,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role:    "assistant",
					Content: common.GetPointer(""),
				},
			},
		},
	}
}

func GenerateStopResponse(id string, createAt int64, model string, finishReason string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				FinishReason: &finishReason,
			},
		},
	}
}

func GenerateFinalUsageResponse(id string, createAt int64, model string, usage dto.Usage) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices:           make([]dto.ChatCompletionsStreamResponseChoice, 0),
		Usage:             &usage,
	}
}
