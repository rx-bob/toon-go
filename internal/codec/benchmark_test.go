package codec

import (
	"fmt"
	"testing"
)

// BenchmarkRow represents a single row in tabular synthetic benchmarks with 11 mixed fields.
type BenchmarkRow struct {
	ID     int     `toon:"id"`
	Name   string  `toon:"name"`
	Email  string  `toon:"email"`
	Active bool    `toon:"active"`
	Score  float64 `toon:"score"`
	Role   string  `toon:"role"`
	Dept   string  `toon:"dept"`
	City   string  `toon:"city"`
	Age    int     `toon:"age"`
	Rating float64 `toon:"rating"`
	Bio    string  `toon:"bio"`
}

// TabularPayload encapsulates a tabular list of benchmark rows.
type TabularPayload struct {
	Users []BenchmarkRow `toon:"users"`
}

func generateTabularPayload(rows int) TabularPayload {
	users := make([]BenchmarkRow, rows)
	roles := []string{"Engineer", "Manager", "Designer", "Director", "Architect"}
	depts := []string{"Platform", "Infrastructure", "Security", "Analytics", "Product"}
	cities := []string{"San Francisco", "New York", "London", "Tokyo", "Berlin"}

	for i := 0; i < rows; i++ {
		users[i] = BenchmarkRow{
			ID:     i + 1,
			Name:   fmt.Sprintf("User %d Firstname Lastname", i+1),
			Email:  fmt.Sprintf("user.%d@enterprise-platform.internal.org", i+1),
			Active: i%2 == 0,
			Score:  float64(i)*1.25 + 50.0,
			Role:   roles[i%len(roles)],
			Dept:   depts[i%len(depts)],
			City:   cities[i%len(cities)],
			Age:    22 + (i % 45),
			Rating: 3.5 + float64(i%15)*0.1,
			Bio:    fmt.Sprintf("Team member %d focusing on high-throughput backend services and data pipelines", i+1),
		}
	}
	return TabularPayload{Users: users}
}

// NestedTree represents a deeply nested binary tree node.
type NestedTree struct {
	Level  int         `toon:"level"`
	Name   string      `toon:"name"`
	Tag    string      `toon:"tag"`
	Active bool        `toon:"active"`
	Value  float64     `toon:"value"`
	Left   *NestedTree `toon:"left,omitempty"`
	Right  *NestedTree `toon:"right,omitempty"`
}

// NestedPayload encapsulates a nested tree payload.
type NestedPayload struct {
	Root *NestedTree `toon:"root"`
}

func generateNestedTree(currentDepth, maxDepth int, id int) *NestedTree {
	if currentDepth > maxDepth {
		return nil
	}
	node := &NestedTree{
		Level:  currentDepth,
		Name:   fmt.Sprintf("node_level_%d_id_%d", currentDepth, id),
		Tag:    fmt.Sprintf("tag-%d", id%7),
		Active: id%2 == 0,
		Value:  float64(id)*3.14159 + 0.5,
	}
	if currentDepth < maxDepth {
		node.Left = generateNestedTree(currentDepth+1, maxDepth, id*2)
		node.Right = generateNestedTree(currentDepth+1, maxDepth, id*2+1)
	}
	return node
}

// QuotedStringItem represents a record containing strings with various quoting conditions.
type QuotedStringItem struct {
	ID        int    `toon:"id"`
	Clean     string `toon:"clean"`
	Space     string `toon:"space"`
	Numeric   string `toon:"numeric"`
	Special   string `toon:"special"`
	Delimited string `toon:"delimited"`
	Escaped   string `toon:"escaped"`
	LongText  string `toon:"long_text"`
}

// StringPayload encapsulates string records with mixed quoting needs.
type StringPayload struct {
	Records []QuotedStringItem `toon:"records"`
}

func generateStringPayload(count int) StringPayload {
	records := make([]QuotedStringItem, count)
	for i := 0; i < count; i++ {
		records[i] = QuotedStringItem{
			ID:        i + 1,
			Clean:     fmt.Sprintf("clean_token_identifier_%d", i+1),
			Space:     fmt.Sprintf("  field with spaces and padding %d  ", i+1),
			Numeric:   fmt.Sprintf("-0%d.450e+02", (i%9)+1),
			Special:   fmt.Sprintf(":colon[bracket_%d]{brace_value}:key", i+1),
			Delimited: fmt.Sprintf("alpha,beta|gamma\tdelta,%d", i+1),
			Escaped:   "line 1\nline 2\rline 3\twith \"escaped quotes\" and \\backslashes\\",
			LongText:  "TOON is a compact, human-readable serialization format: targeting LLM workflows where 'predictable structure' and [reduced tokens] are \"essential\" - feature #42 (v4.1.1).",
		}
	}
	return StringPayload{Records: records}
}

func BenchmarkMarshal(b *testing.B) {
	cases := []struct {
		name string
		data any
	}{
		// Tabular array benchmarks covering row counts and size categories:
		// Small (<1KB), 100 rows (~21KB), Medium (~52KB), 1,000 rows (~210KB), 10,000 rows (>1MB).
		{name: "Tabular/Small_4Rows", data: generateTabularPayload(4)},
		{name: "Tabular/100Rows", data: generateTabularPayload(100)},
		{name: "Tabular/Medium_250Rows", data: generateTabularPayload(250)},
		{name: "Tabular/1000Rows", data: generateTabularPayload(1000)},
		{name: "Tabular/10000Rows", data: generateTabularPayload(10000)},

		// Deeply nested object benchmarks:
		// Small (<1KB), Medium (~50KB), Large (>1MB).
		{name: "NestedObject/Small_Depth3", data: NestedPayload{Root: generateNestedTree(1, 3, 1)}},
		{name: "NestedObject/Medium_Depth8", data: NestedPayload{Root: generateNestedTree(1, 8, 1)}},
		{name: "NestedObject/Large_Depth14", data: NestedPayload{Root: generateNestedTree(1, 14, 1)}},

		// Long string payloads with mixed quoting needs:
		// Small (<1KB), Medium (~50KB), Large (>1MB).
		{name: "LongStrings/Small_2Records", data: generateStringPayload(2)},
		{name: "LongStrings/Medium_110Records", data: generateStringPayload(110)},
		{name: "LongStrings/Large_2600Records", data: generateStringPayload(2600)},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			sample, err := Marshal(tc.data)
			if err != nil {
				b.Fatalf("Marshal setup failed: %v", err)
			}
			b.SetBytes(int64(len(sample)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				out, err := Marshal(tc.data)
				if err != nil {
					b.Fatalf("Marshal failed: %v", err)
				}
				_ = out
			}
		})
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	type unmarshalCase struct {
		name      string
		data      []byte
		newTarget func() any
	}

	cases := []unmarshalCase{
		// Tabular array benchmarks:
		{
			name: "Tabular/Small_4Rows",
			data: func() []byte {
				d, _ := Marshal(generateTabularPayload(4))
				return d
			}(),
			newTarget: func() any { return new(TabularPayload) },
		},
		{
			name: "Tabular/100Rows",
			data: func() []byte {
				d, _ := Marshal(generateTabularPayload(100))
				return d
			}(),
			newTarget: func() any { return new(TabularPayload) },
		},
		{
			name: "Tabular/Medium_250Rows",
			data: func() []byte {
				d, _ := Marshal(generateTabularPayload(250))
				return d
			}(),
			newTarget: func() any { return new(TabularPayload) },
		},
		{
			name: "Tabular/1000Rows",
			data: func() []byte {
				d, _ := Marshal(generateTabularPayload(1000))
				return d
			}(),
			newTarget: func() any { return new(TabularPayload) },
		},
		{
			name: "Tabular/10000Rows",
			data: func() []byte {
				d, _ := Marshal(generateTabularPayload(10000))
				return d
			}(),
			newTarget: func() any { return new(TabularPayload) },
		},

		// Deeply nested object benchmarks:
		{
			name: "NestedObject/Small_Depth3",
			data: func() []byte {
				d, _ := Marshal(NestedPayload{Root: generateNestedTree(1, 3, 1)})
				return d
			}(),
			newTarget: func() any { return new(NestedPayload) },
		},
		{
			name: "NestedObject/Medium_Depth8",
			data: func() []byte {
				d, _ := Marshal(NestedPayload{Root: generateNestedTree(1, 8, 1)})
				return d
			}(),
			newTarget: func() any { return new(NestedPayload) },
		},
		{
			name: "NestedObject/Large_Depth14",
			data: func() []byte {
				d, _ := Marshal(NestedPayload{Root: generateNestedTree(1, 14, 1)})
				return d
			}(),
			newTarget: func() any { return new(NestedPayload) },
		},

		// Long string payloads with mixed quoting needs:
		{
			name: "LongStrings/Small_2Records",
			data: func() []byte {
				d, _ := Marshal(generateStringPayload(2))
				return d
			}(),
			newTarget: func() any { return new(StringPayload) },
		},
		{
			name: "LongStrings/Medium_110Records",
			data: func() []byte {
				d, _ := Marshal(generateStringPayload(110))
				return d
			}(),
			newTarget: func() any { return new(StringPayload) },
		},
		{
			name: "LongStrings/Large_2600Records",
			data: func() []byte {
				d, _ := Marshal(generateStringPayload(2600))
				return d
			}(),
			newTarget: func() any { return new(StringPayload) },
		},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.data)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				target := tc.newTarget()
				if err := Unmarshal(tc.data, target); err != nil {
					b.Fatalf("Unmarshal failed: %v", err)
				}
			}
		})
	}
}

func TestBenchmarkDatasetsCoverSizeRanges(t *testing.T) {
	tabularSmall, err := Marshal(generateTabularPayload(4))
	if err != nil {
		t.Fatalf("tabular small marshal failed: %v", err)
	}
	if len(tabularSmall) >= 1024 {
		t.Errorf("tabular small size %d >= 1024", len(tabularSmall))
	}

	tabularMedium, err := Marshal(generateTabularPayload(250))
	if err != nil {
		t.Fatalf("tabular medium marshal failed: %v", err)
	}
	if len(tabularMedium) < 40*1024 || len(tabularMedium) > 70*1024 {
		t.Errorf("tabular medium size %d not ~50KB", len(tabularMedium))
	}

	tabularLarge, err := Marshal(generateTabularPayload(10000))
	if err != nil {
		t.Fatalf("tabular large marshal failed: %v", err)
	}
	if len(tabularLarge) <= 1024*1024 {
		t.Errorf("tabular large size %d <= 1MB", len(tabularLarge))
	}

	nestedSmall, err := Marshal(NestedPayload{Root: generateNestedTree(1, 3, 1)})
	if err != nil {
		t.Fatalf("nested small marshal failed: %v", err)
	}
	if len(nestedSmall) >= 1024 {
		t.Errorf("nested small size %d >= 1024", len(nestedSmall))
	}

	nestedMedium, err := Marshal(NestedPayload{Root: generateNestedTree(1, 8, 1)})
	if err != nil {
		t.Fatalf("nested medium marshal failed: %v", err)
	}
	if len(nestedMedium) < 40*1024 || len(nestedMedium) > 70*1024 {
		t.Errorf("nested medium size %d not ~50KB", len(nestedMedium))
	}

	nestedLarge, err := Marshal(NestedPayload{Root: generateNestedTree(1, 14, 1)})
	if err != nil {
		t.Fatalf("nested large marshal failed: %v", err)
	}
	if len(nestedLarge) <= 1024*1024 {
		t.Errorf("nested large size %d <= 1MB", len(nestedLarge))
	}

	stringsSmall, err := Marshal(generateStringPayload(2))
	if err != nil {
		t.Fatalf("strings small marshal failed: %v", err)
	}
	if len(stringsSmall) >= 1024 {
		t.Errorf("strings small size %d >= 1024", len(stringsSmall))
	}

	stringsMedium, err := Marshal(generateStringPayload(110))
	if err != nil {
		t.Fatalf("strings medium marshal failed: %v", err)
	}
	if len(stringsMedium) < 40*1024 || len(stringsMedium) > 70*1024 {
		t.Errorf("strings medium size %d not ~50KB", len(stringsMedium))
	}

	stringsLarge, err := Marshal(generateStringPayload(2600))
	if err != nil {
		t.Fatalf("strings large marshal failed: %v", err)
	}
	if len(stringsLarge) <= 1024*1024 {
		t.Errorf("strings large size %d <= 1MB", len(stringsLarge))
	}
}

