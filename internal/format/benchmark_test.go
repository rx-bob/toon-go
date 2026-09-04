package format

import (
	"fmt"
	"strings"
	"testing"
)

func buildStringBatch(count int, kind string) []string {
	res := make([]string, count)
	for i := 0; i < count; i++ {
		switch kind {
		case "clean":
			res[i] = fmt.Sprintf("clean_identifier_token_field_%d", i)
		case "control":
			res[i] = fmt.Sprintf("line_%d\nsecond_line_%d\r\twith_tab", i, i)
		case "special":
			res[i] = fmt.Sprintf("field[%d]:{key_%d}=\"val\\%d\"", i, i, i)
		case "heavy_escape":
			res[i] = fmt.Sprintf("heavy_escape_\"quotes\"_\\backslashes\\_\nlines_\r\ttabs_:colons_[brackets]_{braces}_%d", i)
		case "mixed":
			switch i % 4 {
			case 0:
				res[i] = fmt.Sprintf("clean_token_%d", i)
			case 1:
				res[i] = fmt.Sprintf("special[%d]:value", i)
			case 2:
				res[i] = fmt.Sprintf("control_%d\nline", i)
			default:
				res[i] = fmt.Sprintf("heavy_\"quoted\"_\\%d\\", i)
			}
		}
	}
	return res
}

func BenchmarkNeedsQuoting(b *testing.B) {
	defaultCtx := Context{
		Active:   ',',
		Document: ',',
		InArray:  true,
	}

	singleCases := []struct {
		name string
		text string
		ctx  Context
	}{
		{name: "Clean/Short", text: "user_id", ctx: defaultCtx},
		{name: "Clean/Medium", text: "benchmark_identifier_token_field_status", ctx: defaultCtx},
		{name: "Clean/Long_500B", text: strings.Repeat("clean_unquoted_text_", 25), ctx: defaultCtx},
		{name: "Control/Newline", text: "line1\nline2\nline3", ctx: defaultCtx},
		{name: "Control/Tab", text: "tabbed\tcolumn\tdata", ctx: defaultCtx},
		{name: "Control/LowByte", text: "text\x01control\x1fchars", ctx: defaultCtx},
		{name: "Special/Colon", text: "field:value:extra", ctx: defaultCtx},
		{name: "Special/BracketsBraces", text: "array[0].item{prop}", ctx: defaultCtx},
		{name: "Special/QuoteBackslash", text: `escaped\"quotes\"and\\slashes\\`, ctx: defaultCtx},
		{name: "Special/NumericLookalike", text: "-0123.450e+08", ctx: defaultCtx},
		{name: "Special/WhitespacePadded", text: "  padded string value  ", ctx: defaultCtx},
		{name: "HeavyEscape/500B", text: strings.Repeat(`\"escaped\":[brackets]{\\slash\\}\n`, 15), ctx: defaultCtx},

		// Delimiter configurations: comma vs pipe vs tab
		{
			name: "Delimiter/CommaInCommaCtx",
			text: "alpha,beta,gamma,delta",
			ctx:  Context{Active: ',', Document: ',', InArray: true},
		},
		{
			name: "Delimiter/CommaInPipeCtx",
			text: "alpha,beta,gamma,delta",
			ctx:  Context{Active: '|', Document: '|', InArray: true},
		},
		{
			name: "Delimiter/PipeInPipeCtx",
			text: "alpha|beta|gamma|delta",
			ctx:  Context{Active: '|', Document: '|', InArray: true},
		},
		{
			name: "Delimiter/TabInTabCtx",
			text: "alpha\tbeta\tgamma\tdelta",
			ctx:  Context{Active: '\t', Document: '\t', InArray: true},
		},
	}

	for _, tc := range singleCases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.text)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = NeedsQuoting(tc.text, tc.ctx)
			}
		})
	}

	batchKinds := []string{"clean", "control", "special", "heavy_escape", "mixed"}
	delims := []struct {
		name string
		char rune
	}{
		{name: "Comma", char: ','},
		{name: "Pipe", char: '|'},
		{name: "Tab", char: '\t'},
	}

	for _, d := range delims {
		ctx := Context{Active: d.char, Document: d.char, InArray: true}
		for _, kind := range batchKinds {
			batch := buildStringBatch(1000, kind)
			var totalBytes int64
			for _, s := range batch {
				totalBytes += int64(len(s))
			}

			b.Run(fmt.Sprintf("Batch1000/Delim=%s/Kind=%s", d.name, kind), func(b *testing.B) {
				b.SetBytes(totalBytes)
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					for _, s := range batch {
						_ = NeedsQuoting(s, ctx)
					}
				}
			})
		}
	}
}

func TestNeedsQuotingBenchmarks(t *testing.T) {
	ctxComma := Context{Active: ',', Document: ',', InArray: true}
	ctxPipe := Context{Active: '|', Document: '|', InArray: true}

	if NeedsQuoting("clean_token", ctxComma) {
		t.Error("clean_token should not need quoting")
	}
	if !NeedsQuoting("comma,separated", ctxComma) {
		t.Error("comma in comma context should need quoting")
	}
	if NeedsQuoting("comma,separated", ctxPipe) {
		t.Error("comma in pipe context should not need quoting")
	}
	if !NeedsQuoting("line\nbreak", ctxComma) {
		t.Error("newline should need quoting")
	}
	if !NeedsQuoting(" special:colon ", ctxComma) {
		t.Error("colon and padding should need quoting")
	}
}

func TestQuoteString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"plain text", `"plain text"`},
		{"quote: \" and slash: \\ ", `"quote: \" and slash: \\ "`},
		{"line\ncarriage\r tab\t control\x01", `"line\ncarriage\r tab\t control\u0001"`},
		{"東京", `"東京"`},
	}
	for _, tt := range tests {
		got, err := QuoteString(tt.input)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("QuoteString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
