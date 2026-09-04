package codec

import (
	"fmt"
	"math"
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

