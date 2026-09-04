package codec

import (
	"math/big"
	"strings"
	"testing"
)

func TestEncoderBufferPrimitives(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{name: "null", val: nil, want: "null"},
		{name: "bool_true", val: true, want: "true"},
		{name: "bool_false", val: false, want: "false"},
		{name: "int", val: 42, want: "42"},
		{name: "float", val: 3.14, want: "3.14"},
		{name: "string_clean", val: "hello", want: "hello"},
		{name: "string_quoted", val: "hello world", want: "hello world"},
		{name: "string_with_comma", val: "a,b", want: `"a,b"`},
		{name: "big_int", val: big.NewInt(9007199254740992), want: `"9007199254740992"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bOut, err := Marshal(tc.val)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			sOut, err := MarshalString(tc.val)
			if err != nil {
				t.Fatalf("MarshalString failed: %v", err)
			}
			if string(bOut) != sOut {
				t.Errorf("Marshal and MarshalString mismatch: %q vs %q", string(bOut), sOut)
			}
			if string(bOut) != tc.want {
				t.Errorf("got %q, want %q", string(bOut), tc.want)
			}
		})
	}
}

func TestEncoderBufferObjects(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{
			name: "empty_object",
			val:  NewObject(),
			want: "",
		},
		{
			name: "simple_object",
			val: NewObject(
				Field{Key: "id", Value: 1},
				Field{Key: "name", Value: "Ada"},
				Field{Key: "active", Value: true},
			),
			want: strings.Join([]string{
				"id: 1",
				"name: Ada",
				"active: true",
			}, "\n"),
		},
		{
			name: "nested_object",
			val: NewObject(
				Field{Key: "user", Value: NewObject(
					Field{Key: "name", Value: "Bob"},
					Field{Key: "age", Value: 30},
				)},
			),
			want: strings.Join([]string{
				"user:",
				"  name: Bob",
				"  age: 30",
			}, "\n"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bOut, err := Marshal(tc.val)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			sOut, err := MarshalString(tc.val)
			if err != nil {
				t.Fatalf("MarshalString failed: %v", err)
			}
			if string(bOut) != sOut {
				t.Errorf("Marshal and MarshalString mismatch: %q vs %q", string(bOut), sOut)
			}
			if string(bOut) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", string(bOut), tc.want)
			}
		})
	}
}

func TestEncoderBufferArrays(t *testing.T) {
	tests := []struct {
		name string
		val  any
		opts []EncoderOption
		want string
	}{
		{
			name: "empty_array_root",
			val:  []int{},
			want: "[]",
		},
		{
			name: "primitive_array_root",
			val:  []int{1, 2, 3},
			want: "[3]: 1,2,3",
		},
		{
			name: "primitive_array_pipe",
			val:  []string{"a", "b", "c"},
			opts: []EncoderOption{WithDelimiter(DelimiterPipe)},
			want: "[3|]: a|b|c",
		},
		{
			name: "primitive_array_tab",
			val:  []int{10, 20},
			opts: []EncoderOption{WithDelimiter(DelimiterTab)},
			want: "[2\t]: 10\t20",
		},
		{
			name: "tabular_array",
			val: []any{
				NewObject(Field{Key: "id", Value: 1}, Field{Key: "name", Value: "Ada"}),
				NewObject(Field{Key: "id", Value: 2}, Field{Key: "name", Value: "Bob"}),
			},
			want: strings.Join([]string{
				"[2]{id,name}:",
				"  1,Ada",
				"  2,Bob",
			}, "\n"),
		},
		{
			name: "tabular_array_pipe",
			val: []any{
				NewObject(Field{Key: "id", Value: 1}, Field{Key: "name", Value: "Ada"}),
				NewObject(Field{Key: "id", Value: 2}, Field{Key: "name", Value: "Bob"}),
			},
			opts: []EncoderOption{WithDelimiter(DelimiterPipe)},
			want: strings.Join([]string{
				"[2|]{id|name}:",
				"  1|Ada",
				"  2|Bob",
			}, "\n"),
		},
		{
			name: "object_with_empty_array",
			val:  NewObject(Field{Key: "items", Value: []int{}}),
			want: "items: []",
		},
		{
			name: "object_with_tabular_array",
			val: NewObject(
				Field{Key: "users", Value: []any{
					NewObject(Field{Key: "id", Value: 1}, Field{Key: "role", Value: "admin"}),
					NewObject(Field{Key: "id", Value: 2}, Field{Key: "role", Value: "user"}),
				}},
			),
			want: strings.Join([]string{
				"users[2]{id,role}:",
				"  1,admin",
				"  2,user",
			}, "\n"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bOut, err := Marshal(tc.val, tc.opts...)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			sOut, err := MarshalString(tc.val, tc.opts...)
			if err != nil {
				t.Fatalf("MarshalString failed: %v", err)
			}
			if string(bOut) != sOut {
				t.Errorf("Marshal and MarshalString mismatch: %q vs %q", string(bOut), sOut)
			}
			if string(bOut) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", string(bOut), tc.want)
			}
		})
	}
}

func TestEncoderBufferPreallocationEstimates(t *testing.T) {
	// Verify that estimateBufferSize returns reasonable values
	cfg := defaultEncoderOptions()

	primitives := []any{nil, true, false, 12345, 3.14159, "test string"}
	for _, p := range primitives {
		norm, err := normalize(p, cfg)
		if err != nil {
			t.Fatalf("normalize failed: %v", err)
		}
		hint := estimateBufferSize(norm)
		if hint <= 0 {
			t.Errorf("estimateBufferSize(%v) = %d <= 0", p, hint)
		}
	}

	tabular := make([]any, 100)
	for i := 0; i < 100; i++ {
		tabular[i] = NewObject(Field{Key: "id", Value: i}, Field{Key: "val", Value: "sample"})
	}
	norm, err := normalize(tabular, cfg)
	if err != nil {
		t.Fatalf("normalize tabular failed: %v", err)
	}
	hint := estimateBufferSize(norm)
	if hint < 1000 {
		t.Errorf("estimateBufferSize for 100 rows = %d, expected >= 1000", hint)
	}
}
