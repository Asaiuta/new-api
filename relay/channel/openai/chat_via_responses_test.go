package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var responsesTextTrackingSink int

func newResponsesToChatStreamTestContext() (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-test",
		},
		IsStream:    true,
		RelayFormat: types.RelayFormatOpenAI,
		StartTime:   time.Now(),
	}
	return c, recorder, info
}

func setResponsesStreamTestTimeout(t *testing.T) {
	t.Helper()

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})
}

func responsesStreamBody(t *testing.T, events ...dto.ResponsesStreamResponse) io.Reader {
	t.Helper()

	var body strings.Builder
	for _, event := range events {
		data, err := common.Marshal(event)
		require.NoError(t, err)
		body.WriteString("data: ")
		body.Write(data)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")
	return strings.NewReader(body.String())
}

func responsesCreatedEvent() dto.ResponsesStreamResponse {
	return dto.ResponsesStreamResponse{
		Type: "response.created",
		Response: &dto.OpenAIResponsesResponse{
			ID:        "resp_test",
			Model:     "gpt-test",
			CreatedAt: 123,
		},
	}
}

func responsesCompletedEvent() dto.ResponsesStreamResponse {
	return dto.ResponsesStreamResponse{
		Type: "response.completed",
		Response: &dto.OpenAIResponsesResponse{
			ID:        "resp_test",
			Model:     "gpt-test",
			CreatedAt: 123,
			Usage: &dto.Usage{
				InputTokens:  3,
				OutputTokens: 1,
				TotalTokens:  4,
			},
		},
	}
}

func responsesFunctionCallAddedEvent() dto.ResponsesStreamResponse {
	return dto.ResponsesStreamResponse{
		Type: dto.ResponsesOutputTypeItemAdded,
		Item: &dto.ResponsesOutput{
			Type:      "function_call",
			ID:        "item_lookup",
			CallId:    "call_lookup",
			Name:      "lookup",
			Arguments: json.RawMessage(`{"q":"x"}`),
		},
	}
}

func TestOaiResponsesToChatStreamHandler_TextOutputSkipsLaterToolCalls(t *testing.T) {
	setResponsesStreamTestTimeout(t)

	c, recorder, info := newResponsesToChatStreamTestContext()
	resp := &http.Response{
		Body: io.NopCloser(responsesStreamBody(t,
			responsesCreatedEvent(),
			dto.ResponsesStreamResponse{
				Type:  "response.output_text.delta",
				Delta: "hello",
			},
			responsesFunctionCallAddedEvent(),
			responsesCompletedEvent(),
		)),
	}

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 4, usage.TotalTokens)
	body := recorder.Body.String()
	require.Contains(t, body, `"content":"hello"`)
	require.NotContains(t, body, `"tool_calls"`)
	require.Contains(t, body, `"finish_reason":"stop"`)
}

func TestOaiResponsesToChatStreamHandler_ToolOnlyKeepsToolFinishReason(t *testing.T) {
	setResponsesStreamTestTimeout(t)

	c, recorder, info := newResponsesToChatStreamTestContext()
	resp := &http.Response{
		Body: io.NopCloser(responsesStreamBody(t,
			responsesCreatedEvent(),
			responsesFunctionCallAddedEvent(),
			responsesCompletedEvent(),
		)),
	}

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 4, usage.TotalTokens)
	body := recorder.Body.String()
	require.Contains(t, body, `"tool_calls"`)
	require.Contains(t, body, `"finish_reason":"tool_calls"`)
}

func BenchmarkResponsesToChatTextTracking(b *testing.B) {
	deltas := make([]string, 1000)
	for i := range deltas {
		deltas[i] = "hello "
	}

	b.Run("legacy_duplicate_output_builder", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var outputText strings.Builder
			var usageText strings.Builder
			for _, delta := range deltas {
				outputText.WriteString(delta)
				usageText.WriteString(delta)
			}
			responsesTextTrackingSink = outputText.Len() + usageText.Len()
		}
	})

	b.Run("bool_text_seen", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var usageText strings.Builder
			sawText := false
			for _, delta := range deltas {
				usageText.WriteString(delta)
				sawText = true
			}
			if sawText {
				responsesTextTrackingSink = usageText.Len()
			}
		}
	})
}
