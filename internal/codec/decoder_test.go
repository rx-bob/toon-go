package codec

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestDecoder_ZeroCopy_ByteBoundaries_ASCII(t *testing.T) {
	input := []byte("users[2]{id,name,role}:\n  1,alice,admin\n  2,bob,engineer\n")
	dataPtr := uintptr(unsafe.Pointer(&input[0]))
	dataEnd := dataPtr + uintptr(len(input))

	dec := NewDecoder()
	res, err := dec.Decode(input)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	root, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}
	users, ok := root["users"].([]any)
	if !ok || len(users) != 2 {
		t.Fatalf("expected 2 users, got %v", root["users"])
	}

	expected := []struct {
		id   float64
		name string
		role string
	}{
		{id: 1, name: "alice", role: "admin"},
		{id: 2, name: "bob", role: "engineer"},
	}

	for i, exp := range expected {
		row, ok := users[i].(map[string]any)
		if !ok {
			t.Fatalf("row %d is not map: %v", i, users[i])
		}
		if row["id"] != exp.id {
			t.Errorf("row %d id = %v, want %v", i, row["id"], exp.id)
		}

		for _, field := range []string{"name", "role"} {
			val, ok := row[field].(string)
			if !ok {
				t.Fatalf("row %d field %q is not string: %v", i, field, row[field])
			}
			ptr := uintptr(unsafe.Pointer(unsafe.StringData(val)))
			if ptr < dataPtr || ptr >= dataEnd {
				t.Errorf("token %q for field %q (ptr %x) outside input bounds [%x, %x)", val, field, ptr, dataPtr, dataEnd)
			}
			offset := ptr - dataPtr
			sub := string(input[offset : offset+uintptr(len(val))])
			if sub != val {
				t.Errorf("offset string %q does not match token %q", sub, val)
			}
		}
	}
}

func TestDecoder_ZeroCopy_ByteBoundaries_UTF8(t *testing.T) {
	input := []byte("cities[2]{city,country}:\n  東京🗼,日本🇯🇵\n  München🍺,Deutschland🇩🇪\n")
	dataPtr := uintptr(unsafe.Pointer(&input[0]))
	dataEnd := dataPtr + uintptr(len(input))

	dec := NewDecoder()
	res, err := dec.Decode(input)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	root, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}
	cities, ok := root["cities"].([]any)
	if !ok || len(cities) != 2 {
		t.Fatalf("expected 2 cities, got %v", root["cities"])
	}

	for i, rowRaw := range cities {
		row, ok := rowRaw.(map[string]any)
		if !ok {
			t.Fatalf("row %d is not map: %v", i, rowRaw)
		}
		for _, field := range []string{"city", "country"} {
			val, ok := row[field].(string)
			if !ok {
				t.Fatalf("row %d field %q is not string: %v", i, field, row[field])
			}
			ptr := uintptr(unsafe.Pointer(unsafe.StringData(val)))
			if ptr < dataPtr || ptr >= dataEnd {
				t.Errorf("token %q for field %q (ptr %x) outside input bounds [%x, %x)", val, field, ptr, dataPtr, dataEnd)
			}
			offset := ptr - dataPtr
			sub := string(input[offset : offset+uintptr(len(val))])
			if sub != val {
				t.Errorf("offset string %q does not match token %q", sub, val)
			}
		}
	}
}

func TestDecoder_ZeroCopy_QuotedUnescaped(t *testing.T) {
	input := []byte("items[2]{name,tag}:\n  \"plain quoted\",tag1\n  \"another string\",tag2\n")
	dataPtr := uintptr(unsafe.Pointer(&input[0]))
	dataEnd := dataPtr + uintptr(len(input))

	dec := NewDecoder()
	res, err := dec.Decode(input)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	root, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}
	items, ok := root["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items, got %v", root["items"])
	}

	for i, itemRaw := range items {
		row, ok := itemRaw.(map[string]any)
		if !ok {
			t.Fatalf("row %d is not map: %v", i, itemRaw)
		}
		name, ok := row["name"].(string)
		if !ok {
			t.Fatalf("name not string: %v", row["name"])
		}
		ptr := uintptr(unsafe.Pointer(unsafe.StringData(name)))
		if ptr < dataPtr || ptr >= dataEnd {
			t.Errorf("quoted unescaped token %q (ptr %x) outside input bounds [%x, %x)", name, ptr, dataPtr, dataEnd)
		}
		offset := ptr - dataPtr
		sub := string(input[offset : offset+uintptr(len(name))])
		if sub != name {
			t.Errorf("offset string %q does not match %q", sub, name)
		}
	}
}

func TestDecoder_Equivalence_MapStructPrimitive(t *testing.T) {
	toonDoc := `users[2]{id,name,active}:
  1,alice,true
  2,bob,false
`
	type User struct {
		ID     int    `toon:"id"`
		Name   string `toon:"name"`
		Active bool   `toon:"active"`
	}
	type Doc struct {
		Users []User `toon:"users"`
	}

	// 1. Decode byte vs string
	fromBytes, err := Decode([]byte(toonDoc))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	fromString, err := DecodeString(toonDoc)
	if err != nil {
		t.Fatalf("DecodeString failed: %v", err)
	}
	if !reflect.DeepEqual(fromBytes, fromString) {
		t.Fatalf("Decode != DecodeString: %v vs %v", fromBytes, fromString)
	}

	// 2. Unmarshal byte vs string
	var structBytes Doc
	if err := Unmarshal([]byte(toonDoc), &structBytes); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	var structString Doc
	if err := UnmarshalString(toonDoc, &structString); err != nil {
		t.Fatalf("UnmarshalString failed: %v", err)
	}
	if !reflect.DeepEqual(structBytes, structString) {
		t.Fatalf("Unmarshal != UnmarshalString: %+v vs %+v", structBytes, structString)
	}

	expectedStruct := Doc{
		Users: []User{
			{ID: 1, Name: "alice", Active: true},
			{ID: 2, Name: "bob", Active: false},
		},
	}
	if !reflect.DeepEqual(structBytes, expectedStruct) {
		t.Fatalf("got %+v, want %+v", structBytes, expectedStruct)
	}
}
