package toon_test

import (
	"testing"

	toon "github.com/toon-format/toon-go"
)

func TestObjectPreservesEncodingOrder(t *testing.T) {
	doc, err := toon.MarshalString(toon.NewObject(
		toon.Field{Key: "second", Value: 2},
		toon.Field{Key: "first", Value: 1},
	))
	if err != nil {
		t.Fatalf("marshal ordered object: %v", err)
	}
	if doc != "second: 2\nfirst: 1" {
		t.Fatalf("doc = %q, want ordered fields", doc)
	}
}
