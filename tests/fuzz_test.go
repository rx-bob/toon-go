package toon_test

import (
	"math"
	"reflect"
	"testing"

	toon "github.com/toon-format/toon-go"
)

// FuzzDecodeNeverPanics keeps malformed input on the error path. The seed
// corpus covers each structural form plus known lexical and header failures.
func FuzzDecodeNeverPanics(f *testing.F) {
	seeds := [][]byte{
		{}, []byte("value: text"), []byte("items[3]: 1,2,3"),
		[]byte("items[2]:\n  - one\n  - two"),
		[]byte("items[2]{id,name}:\n  1,Ada\n  2,Bob"),
		[]byte("items[2]{user{name,age}}:\n  Ada,37\n  Bob,38"),
		[]byte("users[2]:\n  ada: 1\n  bob: 2"),
		[]byte("root:\n  nested:\n    values[2]: x,y"),
		[]byte("items[2|]: a|b"), []byte("items[2\t]: a\tb"),
		[]byte("items[#2]: a,b"), []byte("items[999999999999999999999]:"),
		[]byte("items[2]{a,b}:\n  1"), []byte("items[2]{a,a}:\n  1,2"),
		[]byte("\xef\xbb\xbfroot: yes\r\n# comment\r\n"),
		[]byte("quoted: \"\\u0000\\u{1F600}\""), []byte("broken: \xff"),
		[]byte("items[1]:\n    - value"), []byte("["), []byte("root: []\nother: value"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Decode panicked for %q: %v", data, recovered)
			}
		}()
		_, _ = toon.Decode(data)
		_, _ = toon.Decode(data, toon.WithStrictMode(false))
	})
}

func FuzzStringRoundTrip(f *testing.F) {
	for _, seed := range []string{"", "hello", "1,2", "# hash", "quote: \"", "NBSP\u00a0", "emoji \U0001f600", "line\nnext"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		doc, err := toon.MarshalString(value)
		if err != nil {
			t.Skip()
		}
		decoded, err := toon.DecodeString(doc)
		if err != nil {
			t.Fatalf("round-trip decode: %v; document %q", err, doc)
		}
		if got, ok := decoded.(string); !ok || got != value {
			t.Fatalf("round-trip mismatch: got %#v, want %q; document %q", decoded, value, doc)
		}
	})
}

func FuzzJSONModelRoundTrip(f *testing.F) {
	for _, seed := range []string{"", "simple", "nested", "unicode", "arrays", "mixed"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		model := fuzzModel(data, 0)
		doc, err := toon.MarshalString(model)
		if err != nil {
			t.Fatalf("marshal model %#v: %v", model, err)
		}
		decoded, err := toon.DecodeString(doc)
		if err != nil {
			t.Fatalf("decode model document %q: %v", doc, err)
		}
		if !reflect.DeepEqual(decoded, fuzzDecodedModel(model)) {
			t.Fatalf("model round-trip mismatch: got %#v, want %#v; document %q", decoded, fuzzDecodedModel(model), doc)
		}
	})
}

func fuzzModel(data []byte, depth int) any {
	if depth >= 2 || len(data) == 0 {
		switch byteAt(data, 0) % 5 {
		case 0:
			return nil
		case 1:
			return byteAt(data, 1)%2 == 0
		case 2:
			return float64(int8(byteAt(data, 2)))
		default:
			return fuzzString(data)
		}
	}
	switch byteAt(data, 0) % 4 {
	case 0:
		return map[string]any{"a": fuzzModel(tail(data, 1), depth+1), "b": fuzzModel(tail(data, 2), depth+1)}
	case 1:
		return []any{fuzzModel(tail(data, 1), depth+1), fuzzModel(tail(data, 2), depth+1)}
	case 2:
		return fuzzString(data)
	default:
		return float64(int16(uint16(byteAt(data, 1))<<8 | uint16(byteAt(data, 2))))
	}
}

func tail(data []byte, offset int) []byte {
	if offset >= len(data) {
		return nil
	}
	return data[offset:]
}

func fuzzDecodedModel(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = fuzzDecodedModel(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = fuzzDecodedModel(item)
		}
		return out
	case float32:
		return float64(v)
	case float64:
		if math.IsNaN(v) {
			return nil
		}
		return v
	case int, int8, int16, int32, int64:
		return float64(reflect.ValueOf(v).Int())
	case uint, uint8, uint16, uint32, uint64:
		return float64(reflect.ValueOf(v).Uint())
	default:
		return value
	}
}

func byteAt(data []byte, index int) byte {
	if len(data) == 0 {
		return 0
	}
	return data[index%len(data)]
}

func fuzzString(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return string([]rune{'a' + rune(data[0]%26), '\u00a0', '\U0001f600'}[:1+int(data[0]%3)])
}
