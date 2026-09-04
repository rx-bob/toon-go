package simd_test

import (
	"testing"

	"github.com/toon-format/toon-go/internal/format"
	"github.com/toon-format/toon-go/internal/simd"
)

func TestClassifierDifferentialAgainstFormatNeedsQuoting(t *testing.T) {
	testCases := []struct {
		input string
		delim byte
	}{
		{"normal", ','}, {"with,comma", ','}, {"with:colon", ','},
		{"with\\slash", ','}, {"with\"quote", ','}, {"with[bracket", ','},
		{"with]bracket", ','}, {"with{brace", ','}, {"with}brace", ','},
		{"with\nnewline", ','}, {"with\rcarriage", ','}, {"with\ttab", ','},
		{"with\x01control", ','}, {"with|pipe", '|'},
		{"long_clean_string_more_than_thirty_two_bytes_length", ','},
		{"emoji_has_quote_🚀\"🌟", ','},
	}

	for _, tc := range testCases {
		want := format.NeedsQuoting(tc.input, format.Context{InArray: true, Active: rune(tc.delim)})
		for name, classify := range map[string]func([]byte, byte) bool{
			"swar": simd.NeedsQuotingSWAR,
			"avx2": simd.NeedsQuotingAVX2,
			"neon": simd.NeedsQuotingNEON,
		} {
			if got := classify([]byte(tc.input), tc.delim); got && !want {
				t.Errorf("%s: %q with delimiter %q requires quoting for classifier but not format", name, tc.input, tc.delim)
			}
		}
	}
}
