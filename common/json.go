package common

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/bytedance/sonic"
)

// jsonAPI is the sonic configuration used for all marshal/unmarshal operations.
// ConfigStd mirrors encoding/json semantics exactly (HTML escaping, sorted map
// keys, compact marshaler output) so swapping in sonic changes performance only,
// not output bytes. On platforms where sonic lacks JIT support (non amd64/arm64,
// or unsupported Go versions) sonic transparently falls back to encoding/json.
var jsonAPI = sonic.ConfigStd

func Unmarshal(data []byte, v any) error {
	return jsonAPI.Unmarshal(data, v)
}

func UnmarshalJsonStr(data string, v any) error {
	return jsonAPI.UnmarshalFromString(data, v)
}

func DecodeJson(reader io.Reader, v any) error {
	return jsonAPI.NewDecoder(reader).Decode(v)
}

func Marshal(v any) ([]byte, error) {
	return jsonAPI.Marshal(v)
}

func GetJsonType(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "unknown"
	}
	firstChar := trimmed[0]
	switch firstChar {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// JsonRawMessageToString returns JSON strings as their decoded value and other JSON values as raw text.
func JsonRawMessageToString(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] != '"' {
		return string(trimmed)
	}
	var value string
	if err := Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	return value
}
