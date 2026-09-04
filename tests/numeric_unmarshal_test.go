package toon_test

import (
	"math"
	"testing"

	toon "github.com/toon-format/toon-go"
)

func TestUnmarshalExactIntegers(t *testing.T) {
	var value int64
	if err := toon.UnmarshalString("value: 9223372036854775806", &struct {
		Value *int64 `toon:"value"`
	}{Value: &value}); err != nil {
		t.Fatalf("exact integer rejected: %v", err)
	}
	if value != 9223372036854775806 {
		t.Fatalf("value = %d, want exact integer", value)
	}
}

func TestUnmarshalRejectsRoundedOrInvalidIntegers(t *testing.T) {
	tests := []string{
		"value: 9223372036854775808",
		"value: 1.5",
	}
	for _, doc := range tests {
		t.Run(doc, func(t *testing.T) {
			var value int64
			if err := toon.UnmarshalString(doc, &struct {
				Value *int64 `toon:"value"`
			}{Value: &value}); err == nil {
				t.Fatalf("accepted invalid integer assignment")
			}
		})
	}
	var unsigned uint64
	if err := toon.UnmarshalString("value: -1", &struct {
		Value *uint64 `toon:"value"`
	}{Value: &unsigned}); err == nil {
		t.Fatalf("accepted negative unsigned integer")
	}
}

func TestUnmarshalIntegerExponentAndFloat(t *testing.T) {
	var payload struct {
		Count int64   `toon:"count"`
		Ratio float64 `toon:"ratio"`
	}
	if err := toon.UnmarshalString("count: 1e3\nratio: 1.5", &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Count != 1000 || math.Abs(payload.Ratio-1.5) > 1e-12 {
		t.Fatalf("payload = %#v", payload)
	}
}
