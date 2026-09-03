package codec

import "testing"

func TestScanLinesPreprocessesDocument(t *testing.T) {
	lines := scanLines("\ufeffa: 1  \r\n  # comment\r\nb: \u00a0v\n")
	if len(lines) != 3 {
		t.Fatalf("got %d scanned lines, want 3", len(lines))
	}
	if lines[0].number != 1 || lines[0].text != "a: 1" || lines[0].comment {
		t.Fatalf("first line = %#v", lines[0])
	}
	if lines[1].number != 2 || !lines[1].comment {
		t.Fatalf("comment line = %#v", lines[1])
	}
	if lines[2].number != 3 || lines[2].text != "b: \u00a0v" || lines[2].comment {
		t.Fatalf("third line = %#v", lines[2])
	}
}

func TestScanLinesDoesNotTreatTabIndentedHashAsComment(t *testing.T) {
	lines := scanLines("\t# data")
	if len(lines) != 1 || lines[0].comment || lines[0].text != "\t# data" {
		t.Fatalf("scanned tab-indented hash = %#v", lines)
	}
}

func TestDecodePreservesLineNumberAfterCommentRemoval(t *testing.T) {
	_, err := DecodeString("# removed\na: 1\n\tb: 2")
	if err == nil || err.Error() != "line 3: tabs are not allowed in indentation (strict mode)" {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeRejectsInvalidUTF8InStrictMode(t *testing.T) {
	cases := [][]byte{
		{'k', ':', ' ', 0xff},
		{'k', ':', ' ', '"', 0xc3, '"'},
		{'k', ':', ' ', 0xe2, 0x82},
		{0xed, 0xa0, 0x80, ':', ' ', 'v'},
		{0xff, ':', ' ', 'v'},
		{'k', '[', 0xff, ']', ':', ' ', 'v'},
	}
	for _, input := range cases {
		if _, err := Decode(input); err == nil {
			t.Errorf("Decode(%v) accepted invalid UTF-8", input)
		}
	}
}

func TestUnmarshalStringSkipsByteUTF8Validation(t *testing.T) {
	input := string([]byte{'k', ':', ' ', 0xff})
	var value map[string]any
	if err := UnmarshalString(input, &value); err != nil {
		t.Fatalf("UnmarshalString rejected host string with non-UTF-8 bytes: %v", err)
	}
	if got := value["k"]; got != string([]byte{0xff}) {
		t.Fatalf("decoded value = %q, want invalid byte preserved", got)
	}
}

func TestDecodeStringSkipsByteUTF8Validation(t *testing.T) {
	input := string([]byte{'k', ':', ' ', 0xff})
	value, err := DecodeString(input)
	if err != nil {
		t.Fatalf("DecodeString rejected host string with non-UTF-8 bytes: %v", err)
	}
	if got := value.(map[string]any)["k"]; got != string([]byte{0xff}) {
		t.Fatalf("decoded value = %q, want invalid byte preserved", got)
	}
}
