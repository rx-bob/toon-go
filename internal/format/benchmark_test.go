package format

import (
	"fmt"
	"math/rand"
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

func TestAppendFormatStringDifferential(t *testing.T) {
	contexts := []struct {
		name string
		ctx  Context
	}{
		{"comma_in_array", Context{Active: ',', Document: ',', InArray: true}},
		{"comma_not_in_array", Context{Active: ',', Document: ',', InArray: false}},
		{"pipe_in_array", Context{Active: '|', Document: '|', InArray: true}},
		{"pipe_not_in_array", Context{Active: '|', Document: '|', InArray: false}},
		{"tab_in_array", Context{Active: '\t', Document: '\t', InArray: true}},
		{"tab_not_in_array", Context{Active: '\t', Document: '\t', InArray: false}},
	}

	deterministicStrings := []string{
		// Empty
		"",
		// Clean ASCII
		"hello",
		"alpha_numeric_123",
		"a-b-c",
		"property.name",
		// Clean UTF-8
		"東京",
		"こんにちは",
		"café",
		"üñîçødé",
		"🌟✨🎉",
		// Keywords
		"true",
		"false",
		"null",
		// Numeric lookalikes
		"0",
		"42",
		"-42",
		"+42",
		"0123",
		"-0123",
		"3.14159",
		"-0.0",
		".5",
		"5.",
		"1e10",
		"2.5e-3",
		"-1E+06",
		// Delimiters
		"a,b",
		"a|b",
		"a\tb",
		"a:b",
		"a#b",
		// Escapes
		"quote\"inside",
		"back\\slash",
		"line\nbreak",
		"carriage\rreturn",
		"tab\tinside",
		"all\\\"in\none\rstring\t!",
		// Whitespace padded
		" leading",
		"trailing ",
		" both ",
		"\tleadingTab",
		"trailingTab\t",
		// Prefixes
		"-dashPrefix",
		"#hashPrefix",
		// Control chars 0x00 to 0x1f
		"null\x00byte",
		"ctrl\x01byte",
		"bell\x07byte",
		"escape\x1bbyte",
		"unit\x1fsep",
	}

	for c := byte(0); c < 0x20; c++ {
		deterministicStrings = append(deterministicStrings, fmt.Sprintf("prefix_%c_suffix", c))
	}

	dst := make([]byte, 0, 1024)
	for _, tc := range contexts {
		for _, s := range deterministicStrings {
			want, wantErr := FormatString(s, tc.ctx)

			gotBytes, gotErr := AppendFormatString(dst[:0], s, tc.ctx)

			if (wantErr != nil) != (gotErr != nil) {
				t.Fatalf("FormatString(%q, %v) err mismatch: want %v, got %v", s, tc.name, wantErr, gotErr)
			}
			if wantErr != nil {
				if wantErr.Error() != gotErr.Error() {
					t.Fatalf("FormatString(%q, %v) err msg mismatch: want %q, got %q", s, tc.name, wantErr.Error(), gotErr.Error())
				}
				continue
			}

			if string(gotBytes) != want {
				t.Errorf("AppendFormatString(%q, %v) = %q, want %q", s, tc.name, string(gotBytes), want)
			}

			wantQuote, wantQuoteErr := QuoteString(s)
			gotQuoteBytes := AppendQuoteString(dst[:0], s)
			if wantQuoteErr != nil {
				t.Fatalf("QuoteString(%q) unexpected error: %v", s, wantQuoteErr)
			}
			if string(gotQuoteBytes) != wantQuote {
				t.Errorf("AppendQuoteString(%q) = %q, want %q", s, string(gotQuoteBytes), wantQuote)
			}
		}
	}

	// Randomized valid UTF-8 strings
	rng := rand.New(rand.NewSource(42))
	sampleRunes := []rune{
		'a', 'b', 'Z', '0', '9', '_', '-', '.', ':', ',', '|', '\t', '\n', '\r', '"', '\\',
		' ', ' ', ' ',
		'é', 'ñ', 'ø', 'ü', '日', '本', '語', '🔥', '🚀',
		rune(0x01), rune(0x08), rune(0x1b), rune(0x1f),
	}

	for i := 0; i < 500; i++ {
		length := rng.Intn(40)
		runes := make([]rune, length)
		for j := range runes {
			runes[j] = sampleRunes[rng.Intn(len(sampleRunes))]
		}
		s := string(runes)

		for _, tc := range contexts {
			want, wantErr := FormatString(s, tc.ctx)
			gotBytes, gotErr := AppendFormatString(dst[:0], s, tc.ctx)

			if (wantErr != nil) != (gotErr != nil) {
				t.Fatalf("Random string %q (ctx %v) err mismatch: want %v, got %v", s, tc.name, wantErr, gotErr)
			}
			if wantErr == nil && string(gotBytes) != want {
				t.Fatalf("Random string %q (ctx %v) mismatch: got %q, want %q", s, tc.name, string(gotBytes), want)
			}
		}

		wantQuote, _ := QuoteString(s)
		gotQuoteBytes := AppendQuoteString(dst[:0], s)
		if string(gotQuoteBytes) != wantQuote {
			t.Fatalf("Random string %q Quote mismatch: got %q, want %q", s, string(gotQuoteBytes), wantQuote)
		}
	}
}

func TestAppendFormatStringZeroAlloc(t *testing.T) {
	ctx := Context{Active: ',', Document: ',', InArray: true}
	cleanStr := "clean_identifier_without_quoting_needed"
	quotedStr := "string with \"quotes\", commas, and \n newlines"

	dst := make([]byte, 0, 1024)

	// Clean string zero allocation test
	cleanAllocs := testing.AllocsPerRun(1000, func() {
		var err error
		dst, err = AppendFormatString(dst[:0], cleanStr, ctx)
		if err != nil {
			t.Fatal(err)
		}
	})
	if cleanAllocs != 0 {
		t.Errorf("AppendFormatString clean string allocated %f times, want 0", cleanAllocs)
	}

	// Quoted string zero allocation test
	quotedAllocs := testing.AllocsPerRun(1000, func() {
		var err error
		dst, err = AppendFormatString(dst[:0], quotedStr, ctx)
		if err != nil {
			t.Fatal(err)
		}
	})
	if quotedAllocs != 0 {
		t.Errorf("AppendFormatString quoted string allocated %f times, want 0", quotedAllocs)
	}

	// AppendQuoteString zero allocation test
	quoteAllocs := testing.AllocsPerRun(1000, func() {
		dst = AppendQuoteString(dst[:0], quotedStr)
	})
	if quoteAllocs != 0 {
		t.Errorf("AppendQuoteString allocated %f times, want 0", quoteAllocs)
	}
}
