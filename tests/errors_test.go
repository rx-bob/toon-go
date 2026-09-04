package toon_test

import (
	"testing"

	"github.com/toon-format/toon-go"
)

func TestUnmarshalNilTarget(t *testing.T) {
	err := toon.Unmarshal(nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil target")
	}
	if err.Error() != "toon: Unmarshal nil target" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalNonPointer(t *testing.T) {
	var value any
	err := toon.Unmarshal([]byte("foo: bar"), value)
	if err == nil {
		t.Fatalf("expected error for non-pointer target")
	}
}

func TestDecodeLiteralKeyOutsideEncoderPattern(t *testing.T) {
	doc := "1invalid: value"
	value, err := toon.DecodeString(doc)
	if err != nil {
		t.Fatalf("decode literal key: %v", err)
	}
	if got := value.(map[string]any)["1invalid"]; got != "value" {
		t.Fatalf("decoded value = %#v, want value", got)
	}
}

func TestDecodeInvalidQuotedString(t *testing.T) {
	doc := "name: \"unterminated"
	if _, err := toon.DecodeString(doc); err == nil {
		t.Fatalf("expected quoted string error")
	}
}
