package codec

import "testing"

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
