package toon_test

import (
	"reflect"
	"testing"

	"github.com/toon-format/toon-go"
)

// TestSection14ValidationChecklist keeps the normative validation matrix
// visible beside the fixture-based spec tests. The fixture names are useful
// for broad coverage; these focused cases make mode-dependent behavior
// executable and reviewable in one place.
func TestSection14ValidationChecklist(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		strict    bool
		wantError bool
		want      any
	}{
		// §14.1 counts and widths: strict rejects, non-strict consumes all
		// complete values/rows and omits underflow or surplus content.
		{name: "inline count strict", input: "items[2]: 1,2,3", strict: true, wantError: true},
		{name: "inline count non-strict", input: "items[2]: 1,2,3", strict: false, want: map[string]any{"items": []any{float64(1), float64(2), float64(3)}}},
		{name: "list count strict", input: "items[2]:\n  - one", strict: true, wantError: true},
		{name: "list count non-strict", input: "items[2]:\n  - one", strict: false, want: map[string]any{"items": []any{"one"}}},
		{name: "tabular row count strict", input: "[1]{id}:\n  1\n  2", strict: true, wantError: true},
		{name: "tabular row count non-strict", input: "[1]{id}:\n  1\n  2", strict: false, want: []any{map[string]any{"id": float64(1)}, map[string]any{"id": float64(2)}}},
		{name: "tabular width strict", input: "items[1]{id,name}:\n  1", strict: true, wantError: true},
		{name: "tabular width non-strict", input: "items[1]{id,name}:\n  1", strict: false, want: map[string]any{"items": []any{map[string]any{"id": float64(1)}}}},
		{name: "keyed count strict", input: "m[2:]{v}:\n  a: 1", strict: true, wantError: true},
		{name: "keyed count non-strict", input: "m[2:]{v}:\n  a: 1", strict: false, want: map[string]any{"m": map[string]any{"a": map[string]any{"v": float64(1)}}}},
		{name: "keyed width strict", input: "m[1:]{a,b}:\n  k: 1", strict: true, wantError: true},
		{name: "keyed width non-strict", input: "m[1:]{a,b}:\n  k: 1", strict: false, want: map[string]any{"m": map[string]any{"k": map[string]any{"a": float64(1)}}}},

		// §14.2 structural conditions, including conditions that apply in any
		// mode rather than only in strict mode.
		{name: "delimiter mismatch", input: "items[2\t]{a,b}:\n  1\t2\n  3\t4", strict: true, wantError: true},
		{name: "depth jump", input: "a:\n    b: 1", strict: true, wantError: true},
		{name: "blank inside header span strict", input: "items[2]:\n  - one\n\n  - two", strict: true, wantError: true},
		{name: "blank inside header span non-strict", input: "items[2]:\n  - one\n\n  - two", strict: false, want: map[string]any{"items": []any{"one", "two"}}},
		{name: "malformed header", input: "items[03]: 1,2,3", strict: true, wantError: true},
		{name: "duplicate fields", input: "items[1]{a,a}:\n  1,2", strict: true, wantError: true},
		{name: "scalar placement strict", input: "items[1]:\n  value", strict: true, wantError: true},
		{name: "scalar placement non-strict", input: "items[1]:\n  value", strict: false, wantError: true},
		{name: "trailing root content strict", input: "[1]: 1\nextra: value", strict: true, wantError: true},
		{name: "trailing root content non-strict", input: "[1]: 1\nextra: value", strict: false, want: []any{float64(1)}},
		{name: "missing colon any mode strict", input: "items:\n  value", strict: true, wantError: true},
		{name: "missing colon any mode non-strict", input: "items:\n  value", strict: false, wantError: true},

		// §14.3 duplicate sibling keys: strict errors; non-strict is
		// deterministic last-write-wins.
		{name: "duplicate key strict", input: "name: Ada\nname: Bob", strict: true, wantError: true},
		{name: "duplicate key non-strict last-write-wins", input: "name: Ada\nname: Bob", strict: false, want: map[string]any{"name": "Bob"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toon.DecodeString(tc.input, toon.WithStrictMode(tc.strict))
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected validation error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeString: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decoded value = %#v, want %#v", got, tc.want)
			}
		})
	}
}
