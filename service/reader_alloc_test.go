package service

import (
	"bytes"
	"strings"
	"testing"
)

func BenchmarkRequestBodyReaderFromBytes(b *testing.B) {
	payload := bytes.Repeat([]byte(`{"prompt":"measure request body reader allocations","metadata":{"k":"v"}}`), 4096)

	b.Run("legacy_string_reader", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for i := 0; i < b.N; i++ {
			reader := strings.NewReader(string(payload))
			if reader.Len() != len(payload) {
				b.Fatalf("reader length=%d, want %d", reader.Len(), len(payload))
			}
		}
	})

	b.Run("bytes_reader", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for i := 0; i < b.N; i++ {
			reader := bytes.NewReader(payload)
			if reader.Len() != len(payload) {
				b.Fatalf("reader length=%d, want %d", reader.Len(), len(payload))
			}
		}
	})
}
