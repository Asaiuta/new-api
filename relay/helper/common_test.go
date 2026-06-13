package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupStringDataTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

func TestStringDataWritesSSEFrame(t *testing.T) {
	t.Parallel()

	c, recorder := setupStringDataTestContext()

	require.NoError(t, StringData(c, "hello\rworld"))
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "data: hello\\rworld\n\n", recorder.Body.String())
}

func TestBytesDataWritesSSEFrame(t *testing.T) {
	t.Parallel()

	c, recorder := setupStringDataTestContext()

	require.NoError(t, BytesData(c, []byte("hello\rworld")))
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "data: hello\\rworld\n\n", recorder.Body.String())
}

func TestEventBytesDataWritesSSEFrame(t *testing.T) {
	t.Parallel()

	c, recorder := setupStringDataTestContext()

	require.NoError(t, EventBytesData(c, "message.delta", []byte("hello\rworld")))
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "event: message.delta\ndata: hello\\rworld\n\n", recorder.Body.String())
}

func BenchmarkStringDataWriter(b *testing.B) {
	payload := `{"id":"chatcmpl-test","choices":[{"delta":{"content":"hello world"}}]}`

	b.Run("legacy_custom_event", func(b *testing.B) {
		c, recorder := setupStringDataTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			c.Render(-1, common.CustomEvent{Data: "data: " + payload})
			if err := FlushWriter(c); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("direct_string_data", func(b *testing.B) {
		c, recorder := setupStringDataTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			if err := StringData(c, payload); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkEventDataWriter(b *testing.B) {
	payload := []byte(`{"type":"message.delta","delta":{"text":"hello world"}}`)
	event := "message.delta"

	b.Run("legacy_custom_event_pair", func(b *testing.B) {
		c, recorder := setupStringDataTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			c.Render(-1, common.CustomEvent{Data: "event: " + event + "\n"})
			c.Render(-1, common.CustomEvent{Data: "data: " + string(payload)})
			if err := FlushWriter(c); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("direct_event_bytes_data", func(b *testing.B) {
		c, recorder := setupStringDataTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			if err := EventBytesData(c, event, payload); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkObjectDataWriter(b *testing.B) {
	payload := map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion.chunk",
		"created": 1781359200,
		"model":   "gpt-test",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{"content": "hello world"},
			},
		},
	}

	b.Run("legacy_marshal_string_data", func(b *testing.B) {
		c, recorder := setupStringDataTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			jsonData, err := common.Marshal(payload)
			if err != nil {
				b.Fatal(err)
			}
			if err := StringData(c, string(jsonData)); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("object_bytes_data", func(b *testing.B) {
		c, recorder := setupStringDataTestContext()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			recorder.Body.Reset()
			if err := ObjectData(c, payload); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkStreamTextAccumulation(b *testing.B) {
	chunks := make([]string, 512)
	for i := range chunks {
		chunks[i] = "hello world "
	}

	b.Run("legacy_string_concat", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var responseText string
			for _, chunk := range chunks {
				responseText += chunk
			}
			if len(responseText) == 0 {
				b.Fatal("empty response text")
			}
		}
	})

	b.Run("strings_builder", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var responseText strings.Builder
			for _, chunk := range chunks {
				responseText.WriteString(chunk)
			}
			if responseText.Len() == 0 {
				b.Fatal("empty response text")
			}
		}
	})
}
