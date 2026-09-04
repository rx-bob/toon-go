package codec

import "testing"

func TestNormalizeRejectsDuplicateObjectFieldsRecursively(t *testing.T) {
	_, err := normalize(NewObject(
		Field{Key: "outer", Value: NewObject(
			Field{Key: "x", Value: 1},
			Field{Key: "x", Value: 2},
		)},
		Field{Key: "outer", Value: NewObject()},
	), defaultEncoderOptions())
	if err == nil {
		t.Fatal("expected duplicate object key error")
	}
}

func TestNormalizeRejectsDuplicateStructNames(t *testing.T) {
	type payload struct {
		First  int `toon:"value"`
		Second int `toon:"value"`
	}
	_, err := normalize(payload{}, defaultEncoderOptions())
	if err == nil {
		t.Fatal("expected duplicate struct field error")
	}
}

func TestNormalizeKeepsDistinctUnicodeKeys(t *testing.T) {
	_, err := normalize(NewObject(
		Field{Key: "é", Value: 1},
		Field{Key: "e\u0301", Value: 2},
	), defaultEncoderOptions())
	if err != nil {
		t.Fatalf("distinct Unicode keys rejected: %v", err)
	}
}
