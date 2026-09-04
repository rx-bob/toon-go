package parse

import (
	"fmt"
	"strings"
	"testing"
)

func buildDelimitedSegment(count int, delim rune, mode string) string {
	d := string(delim)
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		switch mode {
		case "clean":
			parts[i] = fmt.Sprintf("token_%d_field_value", i)
		case "mixed":
			if i%3 == 0 {
				parts[i] = fmt.Sprintf(`"quoted %s value %d"`, d, i)
			} else if i%3 == 1 {
				parts[i] = fmt.Sprintf("unquoted_%d", i)
			} else {
				parts[i] = fmt.Sprintf(`"item:%d"`, i)
			}
		case "heavy_escape":
			parts[i] = fmt.Sprintf(`"item_%d_with_\"escapes\"_and_\\slashes\\_and_\nlines\r\ttabs_%s"`, i, d)
		}
	}
	return strings.Join(parts, d)
}

func BenchmarkSplitInlineValues(b *testing.B) {
	delimiters := []struct {
		name string
		char rune
	}{
		{name: "Comma", char: ','},
		{name: "Pipe", char: '|'},
		{name: "Tab", char: '\t'},
	}

	modes := []struct {
		name string
		mode string
	}{
		{name: "CleanASCII", mode: "clean"},
		{name: "MixedQuoted", mode: "mixed"},
		{name: "HeavyEscape", mode: "heavy_escape"},
	}

	counts := []int{10, 100, 1000}

	for _, delim := range delimiters {
		for _, m := range modes {
			for _, count := range counts {
				name := fmt.Sprintf("Delim=%s/Mode=%s/Fields=%d", delim.name, m.name, count)
				segment := buildDelimitedSegment(count, delim.char, m.mode)

				b.Run(name, func(b *testing.B) {
					b.SetBytes(int64(len(segment)))
					b.ReportAllocs()
					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						tokens, err := SplitInlineValues(segment, delim.char)
						if err != nil {
							b.Fatalf("SplitInlineValues failed: %v", err)
						}
						_ = tokens
					}
				})
			}
		}
	}
}

func TestSplitInlineValuesBenchmarks(t *testing.T) {
	delimiters := []rune{',', '|', '\t'}
	modes := []string{"clean", "mixed", "heavy_escape"}
	counts := []int{10, 100}

	for _, delim := range delimiters {
		for _, m := range modes {
			for _, count := range counts {
				segment := buildDelimitedSegment(count, delim, m)
				tokens, err := SplitInlineValues(segment, delim)
				if err != nil {
					t.Fatalf("SplitInlineValues(%c, %s, %d) error: %v", delim, m, count, err)
				}
				if len(tokens) != count {
					t.Fatalf("SplitInlineValues(%c, %s, %d) count mismatch: got %d, want %d", delim, m, count, len(tokens), count)
				}
			}
		}
	}
}
