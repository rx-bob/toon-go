package codec

import (
	"fmt"
	"strings"
	"testing"
)

func buildMultiLineDocument(lines int, ending string, includeComments bool) string {
	var b strings.Builder
	b.Grow(lines * 60)
	for i := 0; i < lines; i++ {
		if includeComments && i%5 == 0 {
			b.WriteString("# comment line describing section ")
			b.WriteString(fmt.Sprintf("%d", i))
		} else if includeComments && i%7 == 0 {
			// blank line
		} else {
			indentDepth := (i % 6) * 2
			b.WriteString(strings.Repeat(" ", indentDepth))
			b.WriteString(fmt.Sprintf("field_%d: sample_value_number_%d", i, i))
		}
		b.WriteString(ending)
	}
	return b.String()
}

func BenchmarkScanLines(b *testing.B) {
	cases := []struct {
		name     string
		lines    int
		ending   string
		comments bool
	}{
		{name: "LF/Lines=100", lines: 100, ending: "\n", comments: false},
		{name: "LF/Lines=1000", lines: 1000, ending: "\n", comments: false},
		{name: "LF/Lines=10000", lines: 10000, ending: "\n", comments: false},
		{name: "CRLF/Lines=1000", lines: 1000, ending: "\r\n", comments: false},
		{name: "MixedCommentsBlanks/Lines=1000", lines: 1000, ending: "\n", comments: true},
		{name: "MixedCommentsBlanks/Lines=10000", lines: 10000, ending: "\n", comments: true},
	}

	for _, tc := range cases {
		tc := tc
		doc := buildMultiLineDocument(tc.lines, tc.ending, tc.comments)
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(doc)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				lines := scanLines(doc)
				_ = lines
			}
		})
	}
}

func BenchmarkComputeIndent(b *testing.B) {
	cfgStrict := defaultDecoderOptions()
	cfgNonStrict := decoderOptions{indentSize: 2, strict: false}

	singleLines := []struct {
		name string
		line string
		cfg  decoderOptions
	}{
		{name: "Indent0_NoSpaces", line: "field: value", cfg: cfgStrict},
		{name: "Indent2_Spaces", line: "  field: value", cfg: cfgStrict},
		{name: "Indent4_Spaces", line: "    field: value", cfg: cfgStrict},
		{name: "Indent8_Spaces", line: "        field: value", cfg: cfgStrict},
		{name: "Indent16_Spaces", line: "                field: value", cfg: cfgStrict},
		{name: "IndentTabs_NonStrict", line: "\t\tfield: value", cfg: cfgNonStrict},
		{name: "LongLine_Indent4", line: "    " + strings.Repeat("long_token_string_data_", 20) + ": value", cfg: cfgStrict},
	}

	for _, tc := range singleLines {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.line)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _, _ = computeIndent(tc.line, tc.cfg)
			}
		})
	}

	// Batched multi-line document indentation benchmark
	doc := buildMultiLineDocument(1000, "\n", false)
	scanned := scanLines(doc)
	var totalBytes int64
	for _, sl := range scanned {
		totalBytes += int64(len(sl.text))
	}

	b.Run("DocumentLines_1000/Strict", func(b *testing.B) {
		b.SetBytes(totalBytes)
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			for _, sl := range scanned {
				_, _, _ = computeIndent(sl.text, cfgStrict)
			}
		}
	})

	b.Run("DocumentLines_1000/NonStrict", func(b *testing.B) {
		b.SetBytes(totalBytes)
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			for _, sl := range scanned {
				_, _, _ = computeIndent(sl.text, cfgNonStrict)
			}
		}
	})
}

func TestScanLinesAndComputeIndentBenchmarks(t *testing.T) {
	doc := buildMultiLineDocument(50, "\n", true)
	lines := scanLines(doc)
	if len(lines) == 0 {
		t.Fatal("scanLines returned 0 lines")
	}

	cfg := defaultDecoderOptions()
	for _, sl := range lines {
		if sl.comment || sl.text == "" {
			continue
		}
		indent, content, err := computeIndent(sl.text, cfg)
		if err != nil {
			t.Fatalf("computeIndent(%q) failed: %v", sl.text, err)
		}
		if indent < 0 || content == "" {
			t.Fatalf("unexpected computeIndent result: indent=%d, content=%q", indent, content)
		}
	}
}
