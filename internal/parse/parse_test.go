package parse

import (
	"reflect"
	"testing"
)

func TestSplitInlineValues_Delimiters(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		delimiter rune
		want      []string
		wantErr   bool
	}{
		{
			name:      "Empty",
			input:     "",
			delimiter: ',',
			want:      nil,
			wantErr:   false,
		},
		{
			name:      "WhitespaceOnly",
			input:     "   ",
			delimiter: ',',
			want:      nil,
			wantErr:   false,
		},
		{
			name:      "CommaSeparated",
			input:     "1,2,3",
			delimiter: ',',
			want:      []string{"1", "2", "3"},
			wantErr:   false,
		},
		{
			name:      "CommaWithWhitespace",
			input:     " 1 , 2 , 3 ",
			delimiter: ',',
			want:      []string{"1", "2", "3"},
			wantErr:   false,
		},
		{
			name:      "PipeSeparated",
			input:     "apple|banana|cherry",
			delimiter: '|',
			want:      []string{"apple", "banana", "cherry"},
			wantErr:   false,
		},
		{
			name:      "TabSeparated",
			input:     "col1\tcol2\tcol3",
			delimiter: '\t',
			want:      []string{"col1", "col2", "col3"},
			wantErr:   false,
		},
		{
			name:      "ConsecutiveDelimitersEmptyTokens",
			input:     "1,,3",
			delimiter: ',',
			want:      []string{"1", "", "3"},
			wantErr:   false,
		},
		{
			name:      "LeadingAndTrailingEmptyTokens",
			input:     ",1,",
			delimiter: ',',
			want:      []string{"", "1", ""},
			wantErr:   false,
		},
		{
			name:      "SingleDelimiterOnly",
			input:     ",",
			delimiter: ',',
			want:      []string{"", ""},
			wantErr:   false,
		},
		{
			name:      "QuotedTokenWithEmbeddedDelimiter",
			input:     `"a,b",c,"d,e"`,
			delimiter: ',',
			want:      []string{`"a,b"`, "c", `"d,e"`},
			wantErr:   false,
		},
		{
			name:      "QuotedTokenWithEscapedQuotes",
			input:     `"escaped \" quote",plain`,
			delimiter: ',',
			want:      []string{`"escaped \" quote"`, "plain"},
			wantErr:   false,
		},
		{
			name:      "QuotedTokenWithEscapedBackslash",
			input:     `"double \\ backslash",plain`,
			delimiter: ',',
			want:      []string{`"double \\ backslash"`, "plain"},
			wantErr:   false,
		},
		{
			name:      "UnterminatedQuoteError",
			input:     `"unclosed string,val2`,
			delimiter: ',',
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "NonASCIIRuneDelimiter",
			input:     "alpha·beta·gamma",
			delimiter: '·',
			want:      []string{"alpha", "beta", "gamma"},
			wantErr:   false,
		},
		{
			name:      "MultiByteUTF8PayloadWithComma",
			input:     "東京,Paris,München,São Paulo",
			delimiter: ',',
			want:      []string{"東京", "Paris", "München", "São Paulo"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitInlineValues(tt.input, tt.delimiter)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SplitInlineValues() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitInlineValues() got = %#v, want %#v", got, tt.want)
			}

			// Also test SplitInlineValuesAppend with buffer reuse
			var dstBuf []string
			var delimsBuf []int
			gotAppend, _, errAppend := SplitInlineValuesAppend(tt.input, tt.delimiter, dstBuf, delimsBuf)
			if (errAppend != nil) != tt.wantErr {
				t.Fatalf("SplitInlineValuesAppend() error = %v, wantErr %v", errAppend, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(gotAppend, tt.want) {
				t.Errorf("SplitInlineValuesAppend() got = %#v, want %#v", gotAppend, tt.want)
			}
		})
	}
}

func TestIndexUnquoted_Cases(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		target rune
		want   int
	}{
		{"NotFound", "hello world", ':', -1},
		{"SimpleColon", "key: value", ':', 3},
		{"ColonInsideQuotes", `"key:with:colons": value`, ':', 17},
		{"EscapedQuoteBeforeColon", `"quote\"inside": value`, ':', 15},
		{"CommaDelimiter", "a,b,c", ',', 1},
		{"PipeDelimiter", "a|b|c", '|', 1},
		{"TabDelimiter", "a\tb\tc", '\t', 1},
		{"NonASCII", "hello·world", '·', 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IndexUnquoted(tt.input, tt.target)
			if got != tt.want {
				t.Errorf("IndexUnquoted(%q, %c) = %d, want %d", tt.input, tt.target, got, tt.want)
			}
		})
	}
}
