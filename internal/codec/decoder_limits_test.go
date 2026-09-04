package codec

import (
	"strings"
	"testing"
)

func TestDecoderObjectNestingLimit(t *testing.T) {
	if _, err := DecodeString(nestedObjectDocument(maxDecodeDepth)); err != nil {
		t.Fatalf("depth %d should be accepted: %v", maxDecodeDepth, err)
	}
	if _, err := DecodeString(nestedObjectDocument(maxDecodeDepth + 1)); err == nil {
		t.Fatalf("depth %d should be rejected", maxDecodeDepth+1)
	}
}

func nestedObjectDocument(depth int) string {
	var b strings.Builder
	b.WriteString("root:\n")
	for i := 1; i < depth; i++ {
		b.WriteString(strings.Repeat("  ", i))
		b.WriteString("child:\n")
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("leaf: value")
	return b.String()
}

func TestDecoderFieldNestingLimit(t *testing.T) {
	for depth := 1; depth <= maxDecodeDepth; depth++ {
		if _, err := DecodeString(nestedFieldDocument(depth)); err != nil {
			t.Fatalf("field depth %d should be accepted: %v", depth, err)
		}
	}
	if _, err := DecodeString(nestedFieldDocument(maxDecodeDepth + 1)); err == nil {
		t.Fatalf("field depth %d should be rejected", maxDecodeDepth+1)
	}
}

func nestedFieldDocument(depth int) string {
	var b strings.Builder
	b.WriteString("[1]{")
	for i := 0; i < depth; i++ {
		b.WriteString("field{")
	}
	b.WriteString("leaf")
	b.WriteString(strings.Repeat("}", depth+1))
	b.WriteString(":\n  value")
	return b.String()
}

func TestDecoderBoundsHeaderAndDeclaredCountWork(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "maximum header size",
			input: "items[" + strings.Repeat("1", maxHeaderBytes) + "]: 1",
		},
		{
			name:  "declared count overflow",
			input: "items[" + strings.Repeat("9", 100) + "]: 1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeString(tc.input); err == nil {
				t.Fatalf("expected bounded header/count error")
			}
		})
	}
}

func TestDecoderHandlesLongBlankAndCommentRuns(t *testing.T) {
	input := strings.Repeat("# comment\n\n", 10_000) + "value"
	got, err := DecodeString(input)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if got != "value" {
		t.Fatalf("decoded value = %#v, want value", got)
	}
}
