package codec

import (
	"fmt"
	"testing"
)

func TestTryParseHeaderBuildsRecursiveFieldTree(t *testing.T) {
	header, ok, err := tryParseHeader(`orders[2]{id,customer{name,country},total}:`)
	if err != nil || !ok {
		t.Fatalf("tryParseHeader() = %#v, %v, %v", header, ok, err)
	}
	if len(header.fieldTree) != 3 || header.fieldTree[1].name != "customer" || len(header.fieldTree[1].children) != 2 {
		t.Fatalf("field tree = %#v", header.fieldTree)
	}
	want := []string{"id", "name", "country", "total"}
	if len(header.leafFields) != len(want) {
		t.Fatalf("leaf fields = %#v", header.leafFields)
	}
	for i := range want {
		if header.leafFields[i] != want[i] {
			t.Fatalf("leaf fields = %#v, want %#v", header.leafFields, want)
		}
	}
}

func TestTryParseHeaderPreservesKeyPresenceAndKeyedMarker(t *testing.T) {
	keyless, ok, err := tryParseHeader(`[2:]{x}:`)
	if err != nil || !ok || keyless.keyPresent || !keyless.keyed {
		t.Fatalf("keyless keyed header = %#v, %v, %v", keyless, ok, err)
	}
	empty, ok, err := tryParseHeader(`""[2]{x}:`)
	if err != nil || !ok || !empty.keyPresent || empty.keyed {
		t.Fatalf("empty-key header = %#v, %v, %v", empty, ok, err)
	}
}

func TestTryParseHeaderQuotedFieldNames(t *testing.T) {
	header, ok, err := tryParseHeader(`items[1|]{"a{b}"|"x|y"|nested{"a:b"}}:`)
	if err != nil || !ok {
		t.Fatalf("tryParseHeader() = %#v, %v, %v", header, ok, err)
	}
	want := []string{"a{b}", "x|y", "a:b"}
	for i := range want {
		if header.leafFields[i] != want[i] {
			t.Fatalf("leaf fields = %#v, want %#v", header.leafFields, want)
		}
	}
}

func TestParseBracketSegmentGrammar(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		length int
		delim  Delimiter
		keyed  bool
		ok     bool
	}{
		{"comma", "12", 12, DelimiterComma, false, true},
		{"pipe", "12|", 12, DelimiterPipe, false, true},
		{"tab", "12\t", 12, DelimiterTab, false, true},
		{"keyed", "12:", 12, DelimiterComma, true, true},
		{"keyed pipe", "12:|", 12, DelimiterPipe, true, true},
		{"keyed tab", "12:\t", 12, DelimiterTab, true, true},
		{"legacy marker", "#12", 0, DelimiterComma, false, false},
		{"negative", "-1", 0, DelimiterComma, false, false},
		{"missing", "", 0, DelimiterComma, false, false},
		{"leading zero", "012", 0, DelimiterComma, false, false},
		{"misplaced keyed marker", "12|:", 0, DelimiterComma, false, false},
		{"space", "12 ", 0, DelimiterComma, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			length, delim, keyed, err := parseBracketSegment(tt.input)
			if (err == nil) != tt.ok {
				t.Fatalf("parseBracketSegment(%q) error = %v, want success %v", tt.input, err, tt.ok)
			}
			if tt.ok && (length != tt.length || delim != tt.delim || keyed != tt.keyed) {
				t.Fatalf("parseBracketSegment(%q) = %d, %v, %v", tt.input, length, delim, keyed)
			}
		})
	}
}

func TestTryParseHeaderRejectsFieldDelimiterMismatch(t *testing.T) {
	for _, input := range []string{
		`items[2|]{a,b}:`,
		"items[2\t]{a,b}:",
		`items[2]{a|b}:`,
		`items[2]{nested{a|b}}:`,
	} {
		if _, ok, err := tryParseHeader(input); ok || err == nil {
			t.Errorf("tryParseHeader(%q) = ok %v, err %v; want delimiter error", input, ok, err)
		}
	}
}

func TestHeaderEmissionParity(t *testing.T) {
	delimiters := []struct {
		name  string
		delim Delimiter
	}{
		{"comma", DelimiterComma},
		{"tab", DelimiterTab},
		{"pipe", DelimiterPipe},
	}

	keys := []string{"", "users", "items_list", "special:key", "with space"}
	lengths := []int{0, 1, 15, 1000}

	fieldStructures := []struct {
		name   string
		fields []fieldNode
	}{
		{"empty", nil},
		{"flat", []fieldNode{{name: "id"}, {name: "name"}, {name: "score"}}},
		{"nested_single", []fieldNode{
			{name: "id"},
			{name: "user", children: []fieldNode{{name: "name"}, {name: "email"}}},
			{name: "total"},
		}},
		{"nested_multi_level", []fieldNode{
			{name: "id"},
			{name: "customer", children: []fieldNode{
				{name: "name"},
				{name: "address", children: []fieldNode{{name: "city"}, {name: "country"}}},
			}},
			{name: "status"},
		}},
		{"quoted_fields", []fieldNode{
			{name: "user id"},
			{name: "field:with:colon"},
		}},
	}

	for _, d := range delimiters {
		for _, key := range keys {
			for _, length := range lengths {
				for _, fs := range fieldStructures {
					testName := fmt.Sprintf("Header_%s_key=%q_len=%d_fields=%s", d.name, key, length, fs.name)
					t.Run(testName, func(t *testing.T) {
						var keyLiteral string
						if key != "" {
							lit, _ := encodeKey(key)
							keyLiteral = lit
						}

						want := renderHeader(keyLiteral, length, d.delim, false, fs.fields)

						var buf encBuffer
						buf.appendHeader(keyLiteral, length, d.delim, fs.fields)
						got := buf.String()

						if got != want {
							t.Fatalf("appendHeader mismatch:\ngot:  %q\nwant: %q", got, want)
						}
					})

					// Keyed header parity (only applicable when fields are present)
					if len(fs.fields) > 0 {
						keyedName := fmt.Sprintf("KeyedHeader_%s_key=%q_len=%d_fields=%s", d.name, key, length, fs.name)
						t.Run(keyedName, func(t *testing.T) {
							var keyLiteral string
							if key != "" {
								lit, _ := encodeKey(key)
								keyLiteral = lit
							}

							want := renderKeyedHeader(keyLiteral, length, d.delim, fs.fields)

							var buf encBuffer
							buf.appendKeyedHeader(keyLiteral, length, d.delim, fs.fields)
							got := buf.String()

							if got != want {
								t.Fatalf("appendKeyedHeader mismatch:\ngot:  %q\nwant: %q", got, want)
							}
						})
					}
				}
			}
		}
	}
}

func TestHeaderEmissionZeroAlloc(t *testing.T) {
	fields := []fieldNode{
		{name: "id"},
		{name: "name"},
		{name: "email"},
		{name: "active"},
		{name: "score"},
	}

	buf := newEncBuffer(512)

	allocs := testing.AllocsPerRun(1000, func() {
		buf.Reset()
		buf.appendHeader("users", 1000, DelimiterComma, fields)
	})
	if allocs != 0 {
		t.Errorf("appendHeader allocated %f times, want 0", allocs)
	}

	keyedAllocs := testing.AllocsPerRun(1000, func() {
		buf.Reset()
		buf.appendKeyedHeader("users", 1000, DelimiterComma, fields)
	})
	if keyedAllocs != 0 {
		t.Errorf("appendKeyedHeader allocated %f times, want 0", keyedAllocs)
	}
}
