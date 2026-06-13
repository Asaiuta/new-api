package ollama

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

var benchmarkOllamaChunk ollamaChatStreamChunk

func TestOllamaThinkingContent(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
		ok   bool
	}{
		{name: "empty", raw: nil, ok: false},
		{name: "null", raw: json.RawMessage(` null `), ok: false},
		{name: "json string", raw: json.RawMessage(` "brief plan" `), want: "brief plan", ok: true},
		{name: "raw object fallback", raw: json.RawMessage(` {"step":1} `), want: `{"step":1}`, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ollamaThinkingContent(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok=%v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("content=%q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkOllamaStreamLineDecode(b *testing.B) {
	line := []byte(`  {"model":"llama3","created_at":"2026-06-13T12:00:00Z","message":{"role":"assistant","content":"hello","thinking":"brief plan","tool_calls":[{"function":{"name":"lookup","arguments":{"query":"weather"}}}]},"done":false}  `)

	b.Run("legacy_text_to_bytes", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			text := strings.TrimSpace(string(line))
			var chunk ollamaChatStreamChunk
			if err := json.Unmarshal([]byte(text), &chunk); err != nil {
				b.Fatal(err)
			}
			benchmarkOllamaChunk = chunk
		}
	})

	b.Run("bytes_trim_decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data := bytes.TrimSpace(line)
			var chunk ollamaChatStreamChunk
			if err := common.Unmarshal(data, &chunk); err != nil {
				b.Fatal(err)
			}
			benchmarkOllamaChunk = chunk
		}
	})
}

func BenchmarkOllamaNonStreamLineDecode(b *testing.B) {
	line := []byte(`{"model":"llama3","created_at":"2026-06-13T12:00:00Z","message":{"role":"assistant","content":"hello","thinking":"brief plan"},"done":false}`)
	linesFixture := make([][]byte, 64)
	for i := range linesFixture {
		linesFixture[i] = line
	}
	body := bytes.Join(linesFixture, []byte{'\n'})

	b.Run("legacy_string_split", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw := string(body)
			lines := strings.Split(raw, "\n")
			var last ollamaChatStreamChunk
			for _, ln := range lines {
				ln = strings.TrimSpace(ln)
				if ln == "" {
					continue
				}
				var chunk ollamaChatStreamChunk
				if err := json.Unmarshal([]byte(ln), &chunk); err != nil {
					b.Fatal(err)
				}
				last = chunk
			}
			benchmarkOllamaChunk = last
		}
	})

	b.Run("bytes_split_decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			lines := bytes.Split(body, []byte{'\n'})
			var last ollamaChatStreamChunk
			for _, ln := range lines {
				ln = bytes.TrimSpace(ln)
				if len(ln) == 0 {
					continue
				}
				var chunk ollamaChatStreamChunk
				if err := common.Unmarshal(ln, &chunk); err != nil {
					b.Fatal(err)
				}
				last = chunk
			}
			benchmarkOllamaChunk = last
		}
	})
}
