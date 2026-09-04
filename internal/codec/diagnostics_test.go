package codec

import (
	"errors"
	"strings"
	"testing"
)

func TestDecoderDiagnosticsPreserveSourceLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		line  int
	}{
		{
			name:  "row after comments and blanks",
			input: "# ignored\n\nitems[1]{id,name}:\n  1",
			line:  4,
		},
		{
			name:  "nested list item",
			input: "# ignored\nitems[1]:\n  - name: \"unterminated",
			line:  3,
		},
		{
			name:  "invalid utf8",
			input: "ok: yes\nitems: \xff",
			line:  2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.name == "invalid utf8" {
				data := []byte{'o', 'k', ':', ' ', 'y', 'e', 's', '\n', 'i', 't', 'e', 'm', 's', ':', ' ', 0xff}
				_, err = Decode(data)
			} else {
				_, err = DecodeString(tc.input)
			}
			if err == nil {
				t.Fatal("expected error")
			}
			var pe parseError
			if !errors.As(err, &pe) {
				t.Fatalf("error %T does not preserve parse context: %v", err, err)
			}
			if pe.line != tc.line {
				t.Fatalf("error line = %d, want %d (%v)", pe.line, tc.line, err)
			}
		})
	}
}

func TestErrorWrapPreservesCause(t *testing.T) {
	sentinel := errors.New("sentinel")
	err := errorWrap(7, sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatalf("wrapped error does not preserve cause: %v", err)
	}
}

func TestMalformedDocumentsNeverPanic(t *testing.T) {
	inputs := []string{
		"[",
		"[1]{a{b}:\n  1",
		"items[1]:\n    - value",
		"items[1]{a}:\n\n  \"unterminated",
		"m[1:]{a}:\n  \"unterminated: 1",
		"root:\n  child:\n    grand:\n      value",
	}
	for _, input := range inputs {
		t.Run(strings.ReplaceAll(input, "\n", ";"), func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("DecodeString panicked: %v", recovered)
				}
			}()
			_, _ = DecodeString(input)
		})
	}
}
