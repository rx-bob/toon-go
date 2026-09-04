package codec

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestAppendPrimitiveParity(t *testing.T) {
	cfg := defaultEncoderOptions()

	delimiters := []struct {
		name  string
		delim Delimiter
	}{
		{"comma", DelimiterComma},
		{"tab", DelimiterTab},
		{"pipe", DelimiterPipe},
	}

	testValues := []struct {
		name string
		val  any
	}{
		{"nil", nil},
		{"true", true},
		{"false", false},
		{"int_zero", 0},
		{"int_42", 42},
		{"int_negative", -100},
		{"int_max_safe", int64(9007199254740991)},
		{"int_min_safe", int64(-9007199254740991)},
		{"big_int_beyond_safe", big.NewInt(9007199254740992)},
		{"float_zero", 0.0},
		{"float_neg_zero", math.Copysign(0, -1)},
		{"float_pi", 3.14159},
		{"float_exp_small", 1e-7},
		{"float_exp_large", 1e20},
		{"float_nan", math.NaN()},
		{"float_pos_inf", math.Inf(1)},
		{"float_neg_inf", math.Inf(-1)},
		{"str_empty", ""},
		{"str_clean", "cleanText"},
		{"str_clean_sentence", "hello world"},
		{"str_kw_true", "true"},
		{"str_kw_false", "false"},
		{"str_kw_null", "null"},
		{"str_num_lookalike", "123"},
		{"str_num_decimal", "0123"},
		{"str_leading_dash", "-item"},
		{"str_leading_hash", "#comment"},
		{"str_leading_space", " hello"},
		{"str_trailing_space", "world "},
		{"str_with_comma", "foo,bar"},
		{"str_with_tab", "foo\tbar"},
		{"str_with_pipe", "foo|bar"},
		{"str_with_quotes", `foo "bar" baz`},
		{"str_with_escapes", "line1\nline2\r\t"},
		{"str_with_control_byte", "ctrl\x01test"},
		{"str_unicode", "東京🗼café"},
	}

	for _, d := range delimiters {
		for _, inArray := range []bool{false, true} {
			ctx := formatContext{
				active:   d.delim,
				document: d.delim,
				inArray:  inArray,
			}
			for _, tv := range testValues {
				testName := fmt.Sprintf("%s_%s_inArray=%v", tv.name, d.name, inArray)
				t.Run(testName, func(t *testing.T) {
					norm, normErr := normalize(tv.val, cfg)
					if normErr != nil {
						t.Fatalf("normalize failed: %v", normErr)
					}

					wantStr, wantErr := formatPrimitive(norm, ctx)

					var b encBuffer
					gotErr := b.appendPrimitive(norm, ctx)

					if (wantErr != nil) != (gotErr != nil) {
						t.Fatalf("err mismatch: formatPrimitive err=%v, appendPrimitive err=%v", wantErr, gotErr)
					}
					if wantErr != nil {
						if wantErr.Error() != gotErr.Error() {
							t.Fatalf("err msg mismatch: got %q, want %q", gotErr.Error(), wantErr.Error())
						}
						return
					}

					gotStr := b.String()
					if gotStr != wantStr {
						t.Errorf("output mismatch:\ngot:  %q\nwant: %q", gotStr, wantStr)
					}
				})
			}
		}
	}

	// Test unsupported primitive error parity
	t.Run("unsupported_primitive", func(t *testing.T) {
		ctx := formatContext{active: DelimiterComma, document: DelimiterComma, inArray: false}
		unsupported := struct{}{}
		wantErr := fmt.Sprintf("toon: unsupported primitive %T", unsupported)

		var b encBuffer
		err := b.appendPrimitive(unsupported, ctx)
		if err == nil || err.Error() != wantErr {
			t.Errorf("got %v, want error %q", err, wantErr)
		}
	})

	// Test invalid UTF-8 string error parity
	t.Run("invalid_utf8", func(t *testing.T) {
		ctx := formatContext{active: DelimiterComma, document: DelimiterComma, inArray: false}
		invalidStr := "hello\xffworld"
		wantErr := "toon: string is not valid UTF-8"

		var b encBuffer
		err := b.appendPrimitive(invalidStr, ctx)
		if err == nil || err.Error() != wantErr {
			t.Errorf("got %v, want error %q", err, wantErr)
		}
	})
}

func legacyDetectTabular(values []normalizedValue) ([]fieldNode, bool) {
	if len(values) == 0 {
		return nil, false
	}
	first, ok := values[0].(Object)
	if !ok || first.IsEmpty() {
		return nil, false
	}
	fields := make([]fieldNode, len(first.Fields))
	fieldSet := make(map[string]struct{}, len(first.Fields))
	for i, field := range first.Fields {
		column := make([]normalizedValue, 0, len(values))
		for _, value := range values {
			obj, rowOK := value.(Object)
			if !rowOK {
				return nil, false
			}
			column = append(column, objField(obj, field.Key))
		}
		var fieldOK bool
		fields[i], fieldOK = legacyDetectFieldNode(field.Key, field.Value, column)
		if !fieldOK {
			return nil, false
		}
		fieldSet[field.Key] = struct{}{}
	}
	for _, value := range values[1:] {
		obj, ok := value.(Object)
		if !ok {
			return nil, false
		}
		if len(obj.Fields) != len(fields) {
			return nil, false
		}
		seen := make(map[string]struct{}, len(fields))
		for _, field := range obj.Fields {
			if _, ok := fieldSet[field.Key]; !ok {
				return nil, false
			}
			seen[field.Key] = struct{}{}
		}
		if len(seen) != len(fields) {
			return nil, false
		}
	}
	return fields, true
}

func legacyDetectFieldNode(name string, firstValue normalizedValue, rows []normalizedValue) (fieldNode, bool) {
	if isPrimitive(firstValue) {
		for _, value := range rows {
			if !isPrimitive(value) {
				return fieldNode{}, false
			}
		}
		return fieldNode{name: name}, true
	}

	firstObject, ok := firstValue.(Object)
	if !ok || firstObject.IsEmpty() {
		return fieldNode{}, false
	}
	children := make([]fieldNode, len(firstObject.Fields))
	childSet := make(map[string]struct{}, len(firstObject.Fields))
	for i, child := range firstObject.Fields {
		childRows := make([]normalizedValue, 0, len(rows))
		for _, value := range rows {
			obj, ok := value.(Object)
			if !ok {
				return fieldNode{}, false
			}
			if obj.IsEmpty() {
				return fieldNode{}, false
			}
			childRows = append(childRows, objField(obj, child.Key))
		}
		var childOK bool
		children[i], childOK = legacyDetectFieldNode(child.Key, child.Value, childRows)
		if !childOK {
			return fieldNode{}, false
		}
		childSet[child.Key] = struct{}{}
	}
	for _, value := range rows {
		obj, ok := value.(Object)
		if !ok {
			return fieldNode{}, false
		}
		if len(obj.Fields) != len(childSet) {
			return fieldNode{}, false
		}
		for _, child := range obj.Fields {
			if _, ok := childSet[child.Key]; !ok {
				return fieldNode{}, false
			}
		}
	}
	return fieldNode{name: name, children: children}, true
}

func TestTabularLayoutEligibilityParity(t *testing.T) {
	cases := []struct {
		name   string
		values []normalizedValue
	}{
		{
			name: "stable_order_flat",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}, Field{Key: "b", Value: "x"}),
				NewObject(Field{Key: "a", Value: 2}, Field{Key: "b", Value: "y"}),
			},
		},
		{
			name: "stable_order_nested",
			values: []normalizedValue{
				NewObject(Field{Key: "id", Value: 1}, Field{Key: "u", Value: NewObject(Field{Key: "name", Value: "Ada"})}),
				NewObject(Field{Key: "id", Value: 2}, Field{Key: "u", Value: NewObject(Field{Key: "name", Value: "Bob"})}),
			},
		},
		{
			name: "reordered_keys_top",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}, Field{Key: "b", Value: 2}),
				NewObject(Field{Key: "b", Value: 3}, Field{Key: "a", Value: 4}),
			},
		},
		{
			name: "reordered_keys_nested",
			values: []normalizedValue{
				NewObject(Field{Key: "u", Value: NewObject(Field{Key: "x", Value: 1}, Field{Key: "y", Value: 2})}),
				NewObject(Field{Key: "u", Value: NewObject(Field{Key: "y", Value: 3}, Field{Key: "x", Value: 4})}),
			},
		},
		{
			name: "missing_key",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}, Field{Key: "b", Value: 2}),
				NewObject(Field{Key: "a", Value: 3}),
			},
		},
		{
			name: "extra_key",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}),
				NewObject(Field{Key: "a", Value: 2}, Field{Key: "b", Value: 3}),
			},
		},
		{
			name: "duplicate_key_first",
			values: []normalizedValue{
				Object{Fields: []Field{{Key: "a", Value: 1}, {Key: "a", Value: 2}}},
			},
		},
		{
			name: "duplicate_key_second",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}, Field{Key: "b", Value: 2}),
				Object{Fields: []Field{{Key: "a", Value: 3}, {Key: "a", Value: 4}}},
			},
		},
		{
			name: "empty_first",
			values: []normalizedValue{
				NewObject(),
			},
		},
		{
			name: "empty_second",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}),
				NewObject(),
			},
		},
		{
			name: "nested_empty_first",
			values: []normalizedValue{
				NewObject(Field{Key: "u", Value: NewObject()}),
			},
		},
		{
			name: "nested_empty_second",
			values: []normalizedValue{
				NewObject(Field{Key: "u", Value: NewObject(Field{Key: "x", Value: 1})}),
				NewObject(Field{Key: "u", Value: NewObject()}),
			},
		},
		{
			name: "mixed_primitive_object",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}),
				NewObject(Field{Key: "a", Value: NewObject(Field{Key: "x", Value: 2})}),
			},
		},
		{
			name: "mixed_object_primitive",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: NewObject(Field{Key: "x", Value: 1})}),
				NewObject(Field{Key: "a", Value: 2}),
			},
		},
		{
			name: "slice_column",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: []normalizedValue{1, 2}}),
				NewObject(Field{Key: "a", Value: []normalizedValue{3, 4}}),
			},
		},
		{
			name: "non_object_row",
			values: []normalizedValue{
				NewObject(Field{Key: "a", Value: 1}),
				42,
			},
		},
		{
			name:   "empty_values",
			values: []normalizedValue{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotFields, gotOK := detectTabular(tc.values)
			wantFields, wantOK := legacyDetectTabular(tc.values)

			if gotOK != wantOK {
				t.Fatalf("detectTabular eligibility mismatch: got %v, want %v", gotOK, wantOK)
			}
			if gotOK {
				gotLeaves := flattenFields(gotFields)
				wantLeaves := flattenFields(wantFields)
				if len(gotLeaves) != len(wantLeaves) {
					t.Fatalf("leaf count mismatch: got %v, want %v", gotLeaves, wantLeaves)
				}
				for i := range gotLeaves {
					if gotLeaves[i] != wantLeaves[i] {
						t.Fatalf("leaf [%d] mismatch: got %q, want %q", i, gotLeaves[i], wantLeaves[i])
					}
				}
			}
		})
	}
}

func TestTabularLayoutAllocsScaling(t *testing.T) {
	cfg := defaultEncoderOptions()
	payload := generateTabularPayload(1000)
	norm, err := normalize(payload.Users, cfg)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	rows := norm.([]normalizedValue)

	// Tabular detection should allocate O(1) in row count (compiles 1 layout, 0 maps, 0 column slices)
	allocs := testing.AllocsPerRun(100, func() {
		_, ok := detectTabular(rows)
		if !ok {
			t.Fatal("expected tabular detection to succeed")
		}
	})

	// 11 fields: cols slice (1), nodes slice (1), tabularLayout pointer (1) = 3 allocs.
	// Certainly far below 1,000 allocs (which would indicate per-row maps or column slices).
	if allocs > 10 {
		t.Errorf("detectTabular(1000 rows) allocated %f times, expected <= 10 (no per-row map or slice)", allocs)
	}
}

func TestTabularStreamEncoding(t *testing.T) {
	t.Run("flat_objects", func(t *testing.T) {
		val := []map[string]any{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		}
		got, err := Marshal(val)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		want := "[2]{id,name}:\n  1,Alice\n  2,Bob"
		if string(got) != want {
			t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
		}
	})

	t.Run("nested_objects", func(t *testing.T) {
		val := []map[string]any{
			{"id": 1, "meta": map[string]any{"x": 10, "y": 20}},
			{"id": 2, "meta": map[string]any{"x": 30, "y": 40}},
		}
		got, err := Marshal(val)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		want := "[2]{id,meta{x,y}}:\n  1,10,20\n  2,30,40"
		if string(got) != want {
			t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
		}
	})

	t.Run("reordered_row_fields", func(t *testing.T) {
		cfg := defaultEncoderOptions()
		norm1, err := normalize(map[string]any{"a": 1, "b": "first"}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		norm2, err := normalize(map[string]any{"b": "second", "a": 2}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		// ensure norm2 has keys in reversed order
		obj2 := norm2.(Object)
		if len(obj2.Fields) == 2 && obj2.Fields[0].Key == "a" {
			obj2.Fields[0], obj2.Fields[1] = obj2.Fields[1], obj2.Fields[0]
		}
		val := []normalizedValue{norm1, obj2}
		got, err := Marshal(val)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		want := "[2]{a,b}:\n  1,first\n  2,second"
		if string(got) != want {
			t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
		}
	})

	t.Run("multi_level_nested", func(t *testing.T) {
		val := []map[string]any{
			{"a": 1, "b": map[string]any{"c": map[string]any{"d": "v1"}}},
			{"a": 2, "b": map[string]any{"c": map[string]any{"d": "v2"}}},
		}
		got, err := Marshal(val)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		want := "[2]{a,b{c{d}}}:\n  1,v1\n  2,v2"
		if string(got) != want {
			t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
		}
	})

	t.Run("keyed_object_tabular", func(t *testing.T) {
		val := map[string]any{
			"users": []map[string]any{
				{"id": 1, "name": "Ada"},
				{"id": 2, "name": "Alan"},
			},
		}
		got, err := Marshal(val)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		want := "users[2]{id,name}:\n  1,Ada\n  2,Alan"
		if string(got) != want {
			t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
		}
	})
}

func TestTabularMissingFieldInternalError(t *testing.T) {
	cfg := defaultEncoderOptions()
	norm, err := normalize([]map[string]any{{"a": 1, "b": 2}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	rows := norm.([]normalizedValue)
	layout, ok := compileTabularLayout(rows)
	if !ok {
		t.Fatal("expected layout compilation to succeed")
	}

	ctx := formatContext{active: DelimiterComma, document: DelimiterComma, inArray: true}
	var buf encBuffer

	// Row missing field "b"
	brokenNorm, err := normalize(map[string]any{"a": 10}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = layout.appendRow(&buf, brokenNorm.(Object), 0, ctx, ',')
	if err == nil {
		t.Fatal("expected error for missing field, got nil")
	}
	if !strings.Contains(err.Error(), `toon: field "b" not found in row`) {
		t.Errorf("unexpected error message: %v", err)
	}

	// Corrupted nested row
	nestedNorm, err := normalize([]map[string]any{{"meta": map[string]any{"x": 1}}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	nestedLayout, ok := compileTabularLayout(nestedNorm.([]normalizedValue))
	if !ok {
		t.Fatal("expected nested layout compilation to succeed")
	}
	brokenNestedNorm, err := normalize(map[string]any{"meta": "not-an-object"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = nestedLayout.appendRow(&buf, brokenNestedNorm.(Object), 0, ctx, ',')
	if err == nil {
		t.Fatal("expected error for non-object nested field, got nil")
	}
	if !strings.Contains(err.Error(), "expected nested object") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestTabularRowStreamingAllocationScaling(t *testing.T) {
	cfg := defaultEncoderOptions()
	payload100 := generateTabularPayload(100)
	norm100, err := normalize(payload100.Users, cfg)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	rows100 := norm100.([]normalizedValue)

	payload1000 := generateTabularPayload(1000)
	norm1000, err := normalize(payload1000.Users, cfg)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	rows1000 := norm1000.([]normalizedValue)

	layout100, ok := compileTabularLayout(rows100)
	if !ok {
		t.Fatal("expected layout100 compile to succeed")
	}
	layout1000, ok := compileTabularLayout(rows1000)
	if !ok {
		t.Fatal("expected layout1000 compile to succeed")
	}

	ctx := formatContext{active: DelimiterComma, document: DelimiterComma, inArray: true}
	delim := DelimiterComma.rune()

	var buf100 encBuffer
	buf100.Grow(100 * 256)
	var buf1000 encBuffer
	buf1000.Grow(1000 * 256)

	allocs100 := testing.AllocsPerRun(20, func() {
		buf100.Reset()
		for i, r := range rows100 {
			if err := layout100.appendRow(&buf100, r.(Object), i, ctx, delim); err != nil {
				t.Fatal(err)
			}
		}
	})

	allocs1000 := testing.AllocsPerRun(20, func() {
		buf1000.Reset()
		for i, r := range rows1000 {
			if err := layout1000.appendRow(&buf1000, r.(Object), i, ctx, delim); err != nil {
				t.Fatal(err)
			}
		}
	})

	// Zero per-row allocations
	if allocs100 != 0 {
		t.Errorf("100 rows streaming allocated %f times, want 0", allocs100)
	}
	if allocs1000 != 0 {
		t.Errorf("1000 rows streaming allocated %f times, want 0", allocs1000)
	}
}

type (
	testAliasInt    int
	testAliasString string
	testAliasBool   bool
	testAliasFloat  float32

	testValueStringer int
	testPtrStringer   int
)

func (v testValueStringer) String() string {
	return fmt.Sprintf("val:%d", v)
}

func (p *testPtrStringer) String() string {
	return fmt.Sprintf("ptr:%d", *p)
}

type (
	testRowAliases struct {
		I testAliasInt    `toon:"i"`
		S testAliasString `toon:"s"`
		B testAliasBool   `toon:"b"`
		F testAliasFloat  `toon:"f"`
	}

	testRowIntWidths struct {
		I8  int8   `toon:"i8"`
		I16 int16  `toon:"i16"`
		I32 int32  `toon:"i32"`
		I64 int64  `toon:"i64"`
		I   int    `toon:"i"`
		U8  uint8  `toon:"u8"`
		U16 uint16 `toon:"u16"`
		U32 uint32 `toon:"u32"`
		U64 uint64 `toon:"u64"`
		U   uint   `toon:"u"`
	}

	testRowFloatWidths struct {
		F32 float32 `toon:"f32"`
		F64 float64 `toon:"f64"`
	}

	testRowIgnoredFields struct {
		A       int    `toon:"a"`
		Ignored string `toon:"-"`
		B       string `toon:"b"`
	}

	testRowUnexported struct {
		A          int `toon:"a"`
		unexported int
		B          string `toon:"b"`
	}

	testRowDuplicateTags struct {
		A1 int `toon:"x"`
		A2 int `toon:"x"`
	}

	testRowPointerField struct {
		A int  `toon:"a"`
		P *int `toon:"p"`
	}

	testRowOmitempty struct {
		A int `toon:"a,omitempty"`
		B int `toon:"b"`
	}

	testRowNestedStruct struct {
		A      int `toon:"a"`
		Nested struct {
			X int `toon:"x"`
		} `toon:"nested"`
	}

	testRowValueStringer struct {
		A int               `toon:"a"`
		S testValueStringer `toon:"s"`
	}

	testRowPtrStringer struct {
		A int             `toon:"a"`
		S testPtrStringer `toon:"s"`
	}

	testRowTime struct {
		A int       `toon:"a"`
		T time.Time `toon:"t"`
	}

	testRowEmpty struct{}
)

func TestTabularRowPlanCompilation(t *testing.T) {
	t.Run("benchmark_row_eligibility", func(t *testing.T) {
		plan := cachedTabularRowPlan(reflect.TypeOf(BenchmarkRow{}), DelimiterComma)
		if !plan.IsEligible() {
			t.Fatalf("expected BenchmarkRow to be eligible, got ineligible: %s", plan.Reason())
		}
		if len(plan.fields) != 11 {
			t.Fatalf("expected 11 fields, got %d", len(plan.fields))
		}

		expectedOrder := []string{"id", "name", "email", "active", "score", "role", "dept", "city", "age", "rating", "bio"}
		for i, exp := range expectedOrder {
			if plan.fields[i].name != exp {
				t.Errorf("field %d: expected name %q, got %q", i, exp, plan.fields[i].name)
			}
			if plan.fields[i].flatIndex != i {
				t.Errorf("field %d: expected flatIndex %d, got %d", i, i, plan.fields[i].flatIndex)
			}
		}

		if plan.fields[0].op != opInt {
			t.Errorf("ID op: got %v, want opInt", plan.fields[0].op)
		}
		if plan.fields[1].op != opString {
			t.Errorf("Name op: got %v, want opString", plan.fields[1].op)
		}
		if plan.fields[3].op != opBool {
			t.Errorf("Active op: got %v, want opBool", plan.fields[3].op)
		}
		if plan.fields[4].op != opFloat64 || plan.fields[4].bitWidth != 64 {
			t.Errorf("Score op: got %v (bitwidth %d), want opFloat64 64", plan.fields[4].op, plan.fields[4].bitWidth)
		}

		wantHeader := "{id,name,email,active,score,role,dept,city,age,rating,bio}:"
		if plan.headerLiteral != wantHeader {
			t.Errorf("headerLiteral: got %q, want %q", plan.headerLiteral, wantHeader)
		}

		// Check Tab delimiter precompilation
		planTab := cachedTabularRowPlan(reflect.TypeOf(BenchmarkRow{}), DelimiterTab)
		wantTabHeader := "{id\tname\temail\tactive\tscore\trole\tdept\tcity\tage\trating\tbio}:"
		if planTab.headerLiteral != wantTabHeader {
			t.Errorf("tab headerLiteral: got %q, want %q", planTab.headerLiteral, wantTabHeader)
		}

		// Check Pipe delimiter precompilation
		planPipe := cachedTabularRowPlan(reflect.TypeOf(BenchmarkRow{}), DelimiterPipe)
		wantPipeHeader := "{id|name|email|active|score|role|dept|city|age|rating|bio}:"
		if planPipe.headerLiteral != wantPipeHeader {
			t.Errorf("pipe headerLiteral: got %q, want %q", planPipe.headerLiteral, wantPipeHeader)
		}
	})

	t.Run("aliases", func(t *testing.T) {
		plan := cachedTabularRowPlan(reflect.TypeOf(testRowAliases{}), DelimiterComma)
		if !plan.IsEligible() {
			t.Fatalf("expected testRowAliases to be eligible, got ineligible: %s", plan.Reason())
		}
		if len(plan.fields) != 4 {
			t.Fatalf("expected 4 fields, got %d", len(plan.fields))
		}
		if plan.fields[0].op != opInt {
			t.Errorf("alias int op: got %v, want opInt", plan.fields[0].op)
		}
		if plan.fields[1].op != opString {
			t.Errorf("alias string op: got %v, want opString", plan.fields[1].op)
		}
		if plan.fields[2].op != opBool {
			t.Errorf("alias bool op: got %v, want opBool", plan.fields[2].op)
		}
		if plan.fields[3].op != opFloat32 || plan.fields[3].bitWidth != 32 {
			t.Errorf("alias float op: got %v (bitwidth %d), want opFloat32 32", plan.fields[3].op, plan.fields[3].bitWidth)
		}
	})

	t.Run("all_integer_widths", func(t *testing.T) {
		plan := cachedTabularRowPlan(reflect.TypeOf(testRowIntWidths{}), DelimiterComma)
		if !plan.IsEligible() {
			t.Fatalf("expected testRowIntWidths to be eligible: %s", plan.Reason())
		}
		widths := []int{8, 16, 32, 64, strconv.IntSize, 8, 16, 32, 64, strconv.IntSize}
		for i, w := range widths {
			if plan.fields[i].bitWidth != w {
				t.Errorf("field %s: expected bitWidth %d, got %d", plan.fields[i].name, w, plan.fields[i].bitWidth)
			}
			expectedOp := opInt
			if i >= 5 {
				expectedOp = opUint
			}
			if plan.fields[i].op != expectedOp {
				t.Errorf("field %s: expected op %v, got %v", plan.fields[i].name, expectedOp, plan.fields[i].op)
			}
		}
	})

	t.Run("float_widths", func(t *testing.T) {
		plan := cachedTabularRowPlan(reflect.TypeOf(testRowFloatWidths{}), DelimiterComma)
		if !plan.IsEligible() {
			t.Fatalf("expected testRowFloatWidths to be eligible: %s", plan.Reason())
		}
		if plan.fields[0].op != opFloat32 || plan.fields[0].bitWidth != 32 {
			t.Errorf("f32: got op %v, width %d", plan.fields[0].op, plan.fields[0].bitWidth)
		}
		if plan.fields[1].op != opFloat64 || plan.fields[1].bitWidth != 64 {
			t.Errorf("f64: got op %v, width %d", plan.fields[1].op, plan.fields[1].bitWidth)
		}
	})

	t.Run("ignored_and_unexported_fields", func(t *testing.T) {
		planIgnored := cachedTabularRowPlan(reflect.TypeOf(testRowIgnoredFields{}), DelimiterComma)
		if !planIgnored.IsEligible() {
			t.Fatalf("expected testRowIgnoredFields to be eligible: %s", planIgnored.Reason())
		}
		if len(planIgnored.fields) != 2 {
			t.Fatalf("expected 2 fields (skipping '-'), got %d", len(planIgnored.fields))
		}
		if planIgnored.fields[0].name != "a" || planIgnored.fields[1].name != "b" {
			t.Errorf("unexpected fields: %v", planIgnored.fields)
		}

		planUnexported := cachedTabularRowPlan(reflect.TypeOf(testRowUnexported{}), DelimiterComma)
		if !planUnexported.IsEligible() {
			t.Fatalf("expected testRowUnexported to be eligible: %s", planUnexported.Reason())
		}
		if len(planUnexported.fields) != 2 {
			t.Fatalf("expected 2 fields (skipping unexported), got %d", len(planUnexported.fields))
		}
	})

	t.Run("ineligible_cases", func(t *testing.T) {
		cases := []struct {
			name string
			typ  reflect.Type
			want string
		}{
			{"duplicate_tags", reflect.TypeOf(testRowDuplicateTags{}), "duplicate"},
			{"pointer_field", reflect.TypeOf(testRowPointerField{}), "unsupported type"},
			{"omitempty", reflect.TypeOf(testRowOmitempty{}), "omitempty"},
			{"nested_struct", reflect.TypeOf(testRowNestedStruct{}), "unsupported type"},
			{"value_stringer", reflect.TypeOf(testRowValueStringer{}), "fmt.Stringer"},
			{"ptr_stringer", reflect.TypeOf(testRowPtrStringer{}), "fmt.Stringer"},
			{"time_type", reflect.TypeOf(testRowTime{}), "time type"},
			{"empty_struct", reflect.TypeOf(testRowEmpty{}), "no exported fields"},
			{"pointer_to_struct", reflect.TypeOf(&BenchmarkRow{}), "unsupported row kind"},
			{"primitive_type", reflect.TypeOf(42), "unsupported row kind"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				plan := cachedTabularRowPlan(tc.typ, DelimiterComma)
				if plan.IsEligible() {
					t.Fatalf("expected type %v to be ineligible, but plan was eligible", tc.typ)
				}
				if !strings.Contains(plan.Reason(), tc.want) {
					t.Errorf("reason %q does not contain %q", plan.Reason(), tc.want)
				}
			})
		}
	})

	t.Run("plan_cache_identity", func(t *testing.T) {
		typ := reflect.TypeOf(BenchmarkRow{})
		p1 := cachedTabularRowPlan(typ, DelimiterComma)
		p2 := cachedTabularRowPlan(typ, DelimiterComma)
		if p1 != p2 {
			t.Errorf("expected plan pointer identity, got %p != %p", p1, p2)
		}

		ineligibleTyp := reflect.TypeOf(testRowPointerField{})
		ip1 := cachedTabularRowPlan(ineligibleTyp, DelimiterComma)
		ip2 := cachedTabularRowPlan(ineligibleTyp, DelimiterComma)
		if ip1 != ip2 {
			t.Errorf("expected ineligible plan pointer identity, got %p != %p", ip1, ip2)
		}
	})
}

func TestTabularRowPlanConcurrentLookups(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(BenchmarkRow{}),
		reflect.TypeOf(testRowAliases{}),
		reflect.TypeOf(testRowIntWidths{}),
		reflect.TypeOf(testRowFloatWidths{}),
		reflect.TypeOf(testRowIgnoredFields{}),
		reflect.TypeOf(testRowPointerField{}),
		reflect.TypeOf(testRowOmitempty{}),
		reflect.TypeOf(testRowNestedStruct{}),
		reflect.TypeOf(testRowValueStringer{}),
		reflect.TypeOf(testRowTime{}),
	}
	delims := []Delimiter{DelimiterComma, DelimiterTab, DelimiterPipe}

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				typ := types[(gid+i)%len(types)]
				delim := delims[(gid+i)%len(delims)]
				plan := cachedTabularRowPlan(typ, delim)
				if plan == nil {
					t.Errorf("nil plan returned for %v", typ)
				}
			}
		}(g)
	}

	wg.Wait()
}

func marshalGeneric(v any, opts ...EncoderOption) ([]byte, error) {
	cfg := defaultEncoderOptions()
	for _, opt := range opts {
		opt(&cfg)
	}
	normalized, err := normalize(v, cfg)
	if err != nil {
		return nil, err
	}
	state := &encodeState{
		cfg: cfg,
		buf: newEncBuffer(estimateBufferSize(normalized)),
	}
	if err := state.encodeRoot(normalized); err != nil {
		return nil, err
	}
	return state.buf.Bytes(), nil
}

type testScalarParityRow struct {
	Str   string  `toon:"str"`
	Bool  bool    `toon:"b"`
	I8    int8    `toon:"i8"`
	I16   int16   `toon:"i16"`
	I32   int32   `toon:"i32"`
	I64   int64   `toon:"i64"`
	Int   int     `toon:"i"`
	U8    uint8   `toon:"u8"`
	U16   uint16  `toon:"u16"`
	U32   uint32  `toon:"u32"`
	U64   uint64  `toon:"u64"`
	Uint  uint    `toon:"u"`
	F32   float32 `toon:"f32"`
	F64   float64 `toon:"f64"`
}

func TestDirectTabularRowParity(t *testing.T) {
	rows := []testScalarParityRow{
		{
			Str: "alpha", Bool: true,
			I8: -8, I16: -16, I32: -32, I64: -64, Int: -100,
			U8: 8, U16: 16, U32: 32, U64: 64, Uint: 100,
			F32: 1.5, F64: 3.14159,
		},
		{
			Str: "beta,with,commas", Bool: false,
			I8: 127, I16: 32767, I32: 2147483647, I64: 9007199254740990, Int: 42,
			U8: 255, U16: 65535, U32: 4294967295, U64: 9007199254740990, Uint: 4242,
			F32: -0.0, F64: 0.0,
		},
		{
			Str: "quotes: \"hello\" and \\slashes\\", Bool: true,
			I8: 0, I16: 0, I32: 0, I64: 0, Int: 0,
			U8: 0, U16: 0, U32: 0, U64: 0, Uint: 0,
			F32: float32(math.NaN()), F64: math.Inf(1),
		},
	}

	delims := []struct {
		name string
		opt  EncoderOption
	}{
		{"comma", WithDelimiter(DelimiterComma)},
		{"tab", WithDelimiter(DelimiterTab)},
		{"pipe", WithDelimiter(DelimiterPipe)},
	}

	for _, d := range delims {
		t.Run(d.name, func(t *testing.T) {
			got, err := Marshal(rows, d.opt)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			want, err := marshalGeneric(rows, d.opt)
			if err != nil {
				t.Fatalf("marshalGeneric failed: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("mismatch for delimiter %s:\ngot:\n%s\nwant:\n%s", d.name, string(got), string(want))
			}
		})
	}
}

func TestDirectTabularBoundaryParity(t *testing.T) {
	type boundaryRow struct {
		Name string  `toon:"name"`
		I64  int64   `toon:"i64"`
		U64  uint64  `toon:"u64"`
		F64  float64 `toon:"f64"`
		S    string  `toon:"s"`
	}

	rows := []boundaryRow{
		{Name: "maxSafeInteger", I64: 9007199254740991, U64: 9007199254740991, F64: 1.0, S: "safe"},
		{Name: "maxSafeInteger+1", I64: 9007199254740992, U64: 9007199254740992, F64: 1e20, S: "unsafe"},
		{Name: "maxSafeInteger+100", I64: 9007199254741091, U64: 9007199254741091, F64: 1e-6, S: "unsafe2"},
		{Name: "-maxSafeInteger", I64: -9007199254740991, U64: 0, F64: -1.0, S: "-safe"},
		{Name: "-maxSafeInteger-1", I64: -9007199254740992, U64: 0, F64: -0.0, S: "-unsafe"},
		{Name: "nan_and_inf", I64: 0, U64: 0, F64: math.NaN(), S: "true"},
		{Name: "pos_inf", I64: 0, U64: 0, F64: math.Inf(1), S: "false"},
		{Name: "neg_inf", I64: 0, U64: 0, F64: math.Inf(-1), S: "null"},
		{Name: "numeric_string", I64: 1, U64: 1, F64: 0.1, S: "12345"},
		{Name: "spaces", I64: 2, U64: 2, F64: 0.2, S: " with leading space"},
		{Name: "multibyte_utf8", I64: 3, U64: 3, F64: 0.3, S: "こんにちは世界 🚀🎉"},
		{Name: "control_chars", I64: 4, U64: 4, F64: 0.4, S: "line1\nline2\ttab\rcarriage"},
	}

	got, err := Marshal(rows)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	want, err := marshalGeneric(rows)
	if err != nil {
		t.Fatalf("marshalGeneric failed: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("boundary mismatch:\ngot:\n%s\nwant:\n%s", string(got), string(want))
	}

	// Also verify MarshalString parity
	gotStr, err := MarshalString(rows)
	if err != nil {
		t.Fatalf("MarshalString failed: %v", err)
	}
	if gotStr != string(want) {
		t.Errorf("MarshalString mismatch: got %q, want %q", gotStr, string(want))
	}
}

func TestDirectTabularInvalidUTF8Error(t *testing.T) {
	type invalidRow struct {
		Name string `toon:"name"`
		Val  int    `toon:"val"`
	}

	rows := []invalidRow{
		{Name: "valid", Val: 1},
		{Name: "bad\xffutf8", Val: 2},
	}

	_, err := Marshal(rows)
	if err == nil {
		t.Fatal("expected error for invalid UTF-8, got nil")
	}
	if !strings.Contains(err.Error(), "string is not valid UTF-8") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDirectTabularAllocationZero(t *testing.T) {
	payload := generateTabularPayload(1000)
	rows := payload.Users
	plan := cachedTabularRowPlan(reflect.TypeOf(BenchmarkRow{}), DelimiterComma)
	if !plan.IsEligible() {
		t.Fatalf("expected BenchmarkRow plan to be eligible: %s", plan.Reason())
	}

	val := reflect.ValueOf(rows)
	var buf encBuffer
	buf.Grow(1000 * 256)

	allocs := testing.AllocsPerRun(20, func() {
		buf.Reset()
		ok, err := plan.appendRows(&buf, val, "", 0, 2, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected appendRows to succeed")
		}
	})

	if allocs != 0 {
		t.Errorf("1000 rows appendRows allocated %f times, want 0", allocs)
	}
}

type NamedBenchmarkRows []BenchmarkRow

type (
	testContainerTyped struct {
		Title  string         `toon:"title"`
		Users  []BenchmarkRow `toon:"users"`
		Footer string         `toon:"footer"`
	}

	testContainerGeneric struct {
		Title  string `toon:"title"`
		Users  any    `toon:"users"`
		Footer string `toon:"footer"`
	}

	testContainerGenericOnlyUsers struct {
		Users any `toon:"users"`
	}

	testContainerNamedSlice struct {
		Users NamedBenchmarkRows `toon:"users"`
	}

	testContainerArray struct {
		Users [3]BenchmarkRow `toon:"users"`
	}

	testContainerPointerRows struct {
		Users []*BenchmarkRow `toon:"users"`
	}

	testContainerOmitempty struct {
		Title string         `toon:"title"`
		Users []BenchmarkRow `toon:"users,omitempty"`
	}

	testContainerEmptySlice struct {
		Title string         `toon:"title"`
		Users []BenchmarkRow `toon:"users"`
	}

	testComplexContainerTyped struct {
		ID     int             `toon:"id"`
		Users1 []BenchmarkRow  `toon:"users1"`
		Note   string          `toon:"note"`
		Users2 [2]BenchmarkRow `toon:"users2"`
		Done   bool            `toon:"done"`
	}

	testComplexContainerGeneric struct {
		ID     int    `toon:"id"`
		Users1 any    `toon:"users1"`
		Note   string `toon:"note"`
		Users2 any    `toon:"users2"`
		Done   bool   `toon:"done"`
	}
)

func TestContainingStructTypedTabularSlices(t *testing.T) {
	rows := generateTabularPayload(10).Users

	t.Run("surrounding_fields_parity", func(t *testing.T) {
		delims := []struct {
			name string
			opt  EncoderOption
		}{
			{"comma", WithDelimiter(DelimiterComma)},
			{"tab", WithDelimiter(DelimiterTab)},
			{"pipe", WithDelimiter(DelimiterPipe)},
		}

		for _, d := range delims {
			t.Run(d.name, func(t *testing.T) {
				typed := testContainerTyped{
					Title:  "Enterprise Directory",
					Users:  rows,
					Footer: "Confidential",
				}
				generic := testContainerGeneric{
					Title:  "Enterprise Directory",
					Users:  rows,
					Footer: "Confidential",
				}

				gotBytes, err := Marshal(typed, d.opt)
				if err != nil {
					t.Fatalf("Marshal(typed) failed: %v", err)
				}
				wantBytes, err := Marshal(generic, d.opt)
				if err != nil {
					t.Fatalf("Marshal(generic) failed: %v", err)
				}

				if !bytes.Equal(gotBytes, wantBytes) {
					t.Errorf("delimiter %s mismatch:\ngot:\n%s\nwant:\n%s", d.name, string(gotBytes), string(wantBytes))
				}
			})
		}
	})

	t.Run("named_slice_type", func(t *testing.T) {
		named := testContainerNamedSlice{Users: NamedBenchmarkRows(rows)}
		generic := testContainerGenericOnlyUsers{Users: rows}

		gotBytes, err := Marshal(named)
		if err != nil {
			t.Fatalf("Marshal(named) failed: %v", err)
		}
		wantBytes, err := Marshal(generic)
		if err != nil {
			t.Fatalf("Marshal(generic) failed: %v", err)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("named slice mismatch:\ngot:\n%s\nwant:\n%s", string(gotBytes), string(wantBytes))
		}
	})

	t.Run("array_type", func(t *testing.T) {
		var arr [3]BenchmarkRow
		copy(arr[:], rows[:3])
		arrContainer := testContainerArray{Users: arr}
		generic := testContainerGenericOnlyUsers{Users: arr[:]}

		gotBytes, err := Marshal(arrContainer)
		if err != nil {
			t.Fatalf("Marshal(arrContainer) failed: %v", err)
		}
		wantBytes, err := Marshal(generic)
		if err != nil {
			t.Fatalf("Marshal(generic) failed: %v", err)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("array mismatch:\ngot:\n%s\nwant:\n%s", string(gotBytes), string(wantBytes))
		}
	})

	t.Run("pointer_rows_fallback", func(t *testing.T) {
		ptrRows := make([]*BenchmarkRow, len(rows))
		for i := range rows {
			ptrRows[i] = &rows[i]
		}
		ptrContainer := testContainerPointerRows{Users: ptrRows}
		generic := testContainerGenericOnlyUsers{Users: ptrRows}

		gotBytes, err := Marshal(ptrContainer)
		if err != nil {
			t.Fatalf("Marshal(ptrContainer) failed: %v", err)
		}
		wantBytes, err := Marshal(generic)
		if err != nil {
			t.Fatalf("Marshal(generic) failed: %v", err)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("pointer rows fallback mismatch:\ngot:\n%s\nwant:\n%s", string(gotBytes), string(wantBytes))
		}
	})

	t.Run("omitempty_container", func(t *testing.T) {
		// Empty / nil slice should be omitted
		omitEmpty := testContainerOmitempty{Title: "Only Title", Users: nil}
		got, err := Marshal(omitEmpty)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(got), "users") {
			t.Errorf("expected users to be omitted, got %q", string(got))
		}

		// Non-empty should be emitted tabularly
		omitNonEmpty := testContainerOmitempty{Title: "With Users", Users: rows}
		gotNonEmpty, err := Marshal(omitNonEmpty)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(gotNonEmpty), "users[10]{id,name") {
			t.Errorf("expected users[10] header, got %q", string(gotNonEmpty))
		}
	})

	t.Run("empty_slice_fallback", func(t *testing.T) {
		emptyCont := testContainerEmptySlice{Title: "Empty", Users: nil}
		got, err := Marshal(emptyCont)
		if err != nil {
			t.Fatal(err)
		}
		// In TOON, empty array field is users: []
		if !strings.Contains(string(got), "users: []") {
			t.Errorf("expected users: [], got %q", string(got))
		}
	})

	t.Run("complex_mixed_surrounding_fields", func(t *testing.T) {
		var arr2 [2]BenchmarkRow
		copy(arr2[:], rows[5:7])

		typed := testComplexContainerTyped{
			ID:     42,
			Users1: rows[:3],
			Note:   "intermediate",
			Users2: arr2,
			Done:   true,
		}
		generic := testComplexContainerGeneric{
			ID:     42,
			Users1: rows[:3],
			Note:   "intermediate",
			Users2: arr2[:],
			Done:   true,
		}

		gotBytes, err := Marshal(typed)
		if err != nil {
			t.Fatal(err)
		}
		wantBytes, err := Marshal(generic)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("complex container mismatch:\ngot:\n%s\nwant:\n%s", string(gotBytes), string(wantBytes))
		}
	})

	t.Run("benchmark_payload_allocation_drop", func(t *testing.T) {
		payload := generateTabularPayload(1000)
		allocs := testing.AllocsPerRun(10, func() {
			_, err := Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
		})
		if allocs > 50 {
			t.Errorf("Marshal(TabularPayload 1000 rows) allocated %f times, expected <= 50", allocs)
		}
	})
}

func TestPreflightTabularValidationAndCapacity(t *testing.T) {
	t.Run("invalid_utf8_in_struct_field_preflight", func(t *testing.T) {
		type badRow struct {
			ID   int    `toon:"id"`
			Name string `toon:"name"`
		}
		type container struct {
			Header string   `toon:"header"`
			Rows   []badRow `toon:"rows"`
			Footer string   `toon:"footer"`
		}

		c := container{
			Header: "Valid Header",
			Rows: []badRow{
				{ID: 1, Name: "Valid Name"},
				{ID: 2, Name: "Bad\xffName"},
			},
			Footer: "Valid Footer",
		}

		_, err := Marshal(c)
		if err == nil {
			t.Fatal("expected error for invalid UTF-8 in struct field, got nil")
		}
		if !strings.Contains(err.Error(), "string is not valid UTF-8") {
			t.Errorf("expected 'string is not valid UTF-8', got %q", err.Error())
		}
	})

	t.Run("invalid_utf8_in_root_slice_preflight", func(t *testing.T) {
		type badRow struct {
			ID   int    `toon:"id"`
			Name string `toon:"name"`
		}
		rows := []badRow{
			{ID: 1, Name: "Valid Name"},
			{ID: 2, Name: "Invalid\xffUTF8"},
		}

		_, err := Marshal(rows)
		if err == nil {
			t.Fatal("expected error for invalid UTF-8 in root slice, got nil")
		}
		if !strings.Contains(err.Error(), "string is not valid UTF-8") {
			t.Errorf("expected 'string is not valid UTF-8', got %q", err.Error())
		}
	})

	t.Run("fallback_ineligible_conditions", func(t *testing.T) {
		type unsupportedRow struct {
			ID   int       `toon:"id"`
			Time time.Time `toon:"time"` // unsupported in fast path
		}
		type containerWithUnsupported struct {
			Title string           `toon:"title"`
			Rows  []unsupportedRow `toon:"rows"`
		}

		c := containerWithUnsupported{
			Title: "Fallback",
			Rows: []unsupportedRow{
				{ID: 1, Time: time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)},
			},
		}

		got, err := Marshal(c)
		if err != nil {
			t.Fatalf("expected clean fallback for unsupported field type, got error: %v", err)
		}
		// Generic normalization should emit rows as list items
		if !strings.Contains(string(got), "title: Fallback") {
			t.Errorf("expected title in fallback output, got: %s", string(got))
		}
	})

	t.Run("encbuffer_capacity_growth_limit_1000rows", func(t *testing.T) {
		payload := generateTabularPayload(1000)
		norm, err := normalize(payload, defaultEncoderOptions())
		if err != nil {
			t.Fatal(err)
		}

		// Initial buffer with default 64-byte capacity
		buf := newEncBuffer(64)
		if buf.Cap() != 64 {
			t.Fatalf("expected initial capacity 64, got %d", buf.Cap())
		}

		// Grow once with conservative capacity hint from preflight
		hint := estimateBufferSize(norm)
		buf.Grow(hint)
		capAfterGrow := buf.Cap()

		state := &encodeState{
			cfg: defaultEncoderOptions(),
			buf: buf,
		}

		// Encode the 1,000-row document
		if err := state.encodeRoot(norm); err != nil {
			t.Fatal(err)
		}

		finalCap := state.buf.Cap()
		if finalCap != capAfterGrow {
			t.Errorf("encBuffer reallocated during encoding: grew from %d to %d (more than once after initialization)", capAfterGrow, finalCap)
		}
		if state.buf.Grows() > 1 {
			t.Errorf("encBuffer.Grow was called %d times, expected <= 1", state.buf.Grows())
		}
	})

	t.Run("preflight_zero_allocations", func(t *testing.T) {
		payload := generateTabularPayload(1000)
		val := reflect.ValueOf(payload.Users)
		plan := cachedTabularRowPlan(reflect.TypeOf(BenchmarkRow{}), DelimiterComma)

		allocs := testing.AllocsPerRun(100, func() {
			ok, est, err := preflightTabularSlice(val, plan)
			if !ok || est <= 0 || err != nil {
				t.Fatalf("preflight failed: ok=%v, est=%d, err=%v", ok, est, err)
			}
		})
		if allocs != 0 {
			t.Errorf("preflightTabularSlice allocated %f times, expected 0", allocs)
		}
	})
}


