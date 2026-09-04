package simd

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestCPUFeatures_Queries(t *testing.T) {
	f := Features()
	if HasAVX2() != f.HasAVX2 {
		t.Errorf("HasAVX2() = %v, want %v", HasAVX2(), f.HasAVX2)
	}
	if HasBMI2() != f.HasBMI2 {
		t.Errorf("HasBMI2() = %v, want %v", HasBMI2(), f.HasBMI2)
	}
	if HasNEON() != f.HasNEON {
		t.Errorf("HasNEON() = %v, want %v", HasNEON(), f.HasNEON)
	}
}

func TestAlgorithm_FallbackMatrix(t *testing.T) {
	tests := []struct {
		name     string
		features CPUFeatures
		expected Algorithm
	}{
		{
			name:     "AllDisabled_FallsBackToSWAR",
			features: CPUFeatures{HasAVX2: false, HasBMI2: false, HasNEON: false},
			expected: AlgoSWAR,
		},
		{
			name:     "AVX2Only_NoBMI2_FallsBackToSWAR",
			features: CPUFeatures{HasAVX2: true, HasBMI2: false, HasNEON: false},
			expected: AlgoSWAR,
		},
		{
			name:     "BMI2Only_NoAVX2_FallsBackToSWAR",
			features: CPUFeatures{HasAVX2: false, HasBMI2: true, HasNEON: false},
			expected: AlgoSWAR,
		},
		{
			name:     "AVX2AndBMI2_SelectsAVX2",
			features: CPUFeatures{HasAVX2: true, HasBMI2: true, HasNEON: false},
			expected: AlgoAVX2,
		},
		{
			name:     "NEONOnly_SelectsNEON",
			features: CPUFeatures{HasAVX2: false, HasBMI2: false, HasNEON: true},
			expected: AlgoNEON,
		},
		{
			name:     "AVX2AndNEON_PrefersAVX2",
			features: CPUFeatures{HasAVX2: true, HasBMI2: true, HasNEON: true},
			expected: AlgoAVX2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restore := SetCPUFeaturesForTest(tc.features)
			defer restore()

			algo := SelectBestAlgorithm()
			if algo != tc.expected {
				t.Errorf("SelectBestAlgorithm() = %v (%s), want %v (%s)", algo, algo, tc.expected, tc.expected)
			}
		})
	}
}

func TestScanDelim_SimulatedFallbacks(t *testing.T) {
	testInputs := [][]byte{
		[]byte(""),
		[]byte(","),
		[]byte("a,b,c"),
		[]byte(`"a,b",c,d`),
		[]byte(`"escaped \" quote",foo,bar`),
		[]byte(`unquoted\slash,normal,item`),
		bytes.Repeat([]byte("id,name,role,active\n1,alice,admin,true\n2,bob,engineer,false\n"), 50),
	}

	simulations := []struct {
		name     string
		features CPUFeatures
	}{
		{
			name:     "SimulateGeneric_AllDisabled",
			features: CPUFeatures{HasAVX2: false, HasBMI2: false, HasNEON: false},
		},
		{
			name:     "SimulateAVX2",
			features: CPUFeatures{HasAVX2: true, HasBMI2: true, HasNEON: false},
		},
		{
			name:     "SimulateNEON",
			features: CPUFeatures{HasAVX2: false, HasBMI2: false, HasNEON: true},
		},
	}

	for _, sim := range simulations {
		t.Run(sim.name, func(t *testing.T) {
			restore := SetCPUFeaturesForTest(sim.features)
			defer restore()

			for i, input := range testInputs {
				expected := ScanDelimScalar(input, ',')

				// Auto dispatch
				auto := ScanDelimAuto(input, ',')
				if auto != expected {
					t.Errorf("input %d (%s): ScanDelimAuto = %d, want %d", i, sim.name, auto, expected)
				}

				// Direct AVX2 (must fall back cleanly if unsupported)
				avx2 := ScanDelimAVX2(input, ',')
				if avx2 != expected {
					t.Errorf("input %d (%s): ScanDelimAVX2 = %d, want %d", i, sim.name, avx2, expected)
				}

				// Direct NEON (must fall back cleanly if unsupported)
				neon := ScanDelimNEON(input, ',')
				if neon != expected {
					t.Errorf("input %d (%s): ScanDelimNEON = %d, want %d", i, sim.name, neon, expected)
				}
			}
		})
	}
}

func TestAlgorithm_String(t *testing.T) {
	cases := []struct {
		algo     Algorithm
		expected string
	}{
		{AlgoScalar, "Scalar"},
		{AlgoSWAR, "SWAR"},
		{AlgoAVX2, "AVX2"},
		{AlgoNEON, "NEON"},
		{Algorithm(99), "Unknown"},
	}

	for _, c := range cases {
		if c.algo.String() != c.expected {
			t.Errorf("%d.String() = %q, want %q", c.algo, c.algo.String(), c.expected)
		}
	}
}

func TestSWAR_QuoteBoundaryAlignments(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		delim        byte
		wantIndices  []int
		wantFirstIdx int
		wantInQuotes bool
	}{
		{
			name:         "QuoteAtByte0_SinglePair",
			input:        `"abc",def`,
			delim:        ',',
			wantIndices:  []int{5},
			wantFirstIdx: 5,
			wantInQuotes: false,
		},
		{
			name:         "QuoteAtByte0_DelimInside",
			input:        `"a,b",c,d`,
			delim:        ',',
			wantIndices:  []int{5, 7},
			wantFirstIdx: 5,
			wantInQuotes: false,
		},
		{
			name: "QuoteAtByte7",
			// bytes 0..6 are '0'..'6', byte 7 is '"', byte 11 is '"', byte 12 is ','
			input:        `0123456"abc",def`,
			delim:        ',',
			wantIndices:  []int{12},
			wantFirstIdx: 12,
			wantInQuotes: false,
		},
		{
			name: "QuoteClosingAtByte7",
			// byte 0 is '"', byte 7 is '"', byte 8 is ','
			input:        `"123456",foo`,
			delim:        ',',
			wantIndices:  []int{8},
			wantFirstIdx: 8,
			wantInQuotes: false,
		},
		{
			name: "QuoteAtByte8_NewWordBoundary",
			// bytes 0..7 are '0'..'7', byte 8 is '"', byte 12 is '"', byte 13 is ','
			input:        `01234567"abc",def`,
			delim:        ',',
			wantIndices:  []int{13},
			wantFirstIdx: 13,
			wantInQuotes: false,
		},
		{
			name: "QuoteSpanningByte7And8",
			// byte 7 is '"', byte 8 is 'x', byte 9 is '"', byte 10 is ','
			input:        `0123456"x",next`,
			delim:        ',',
			wantIndices:  []int{10},
			wantFirstIdx: 10,
			wantInQuotes: false,
		},
		{
			name: "QuoteAtByte15_SecondWordBoundary",
			// bytes 0..14 are 15 chars, byte 15 is '"', byte 19 is '"', byte 20 is ','
			input:        `012345678901234"abc",def`,
			delim:        ',',
			wantIndices:  []int{20},
			wantFirstIdx: 20,
			wantInQuotes: false,
		},
		{
			name: "MultipleQuotePairsInSingleWord",
			// 8-byte block contains: "a","b",
			input:        `"a","b",c`,
			delim:        ',',
			wantIndices:  []int{3, 7},
			wantFirstIdx: 3,
			wantInQuotes: false,
		},
		{
			name:         "PipeDelimiter",
			input:        `"alpha|beta"|gamma|"delta|epsilon"`,
			delim:        '|',
			wantIndices:  []int{12, 18},
			wantFirstIdx: 12,
			wantInQuotes: false,
		},
		{
			name:         "TabDelimiter",
			input:        "\"col\t1\"\tcol2\t\"col\t3\"",
			delim:        '\t',
			wantIndices:  []int{7, 12},
			wantFirstIdx: 7,
			wantInQuotes: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := []byte(tc.input)

			// 1. IndexUnquotedSWAR
			first := IndexUnquotedSWAR(b, tc.delim)
			if first != tc.wantFirstIdx {
				t.Errorf("IndexUnquotedSWAR(%q, %c) = %d, want %d", tc.input, tc.delim, first, tc.wantFirstIdx)
			}

			// 2. FindDelimsSWAR
			gotIndices, inQuotes := FindDelimsSWAR(b, tc.delim, nil)
			if inQuotes != tc.wantInQuotes {
				t.Errorf("FindDelimsSWAR(%q) inQuotes = %v, want %v", tc.input, inQuotes, tc.wantInQuotes)
			}
			if len(gotIndices) != len(tc.wantIndices) {
				t.Fatalf("FindDelimsSWAR(%q) indices = %v, want %v", tc.input, gotIndices, tc.wantIndices)
			}
			for i := range gotIndices {
				if gotIndices[i] != tc.wantIndices[i] {
					t.Errorf("index[%d] = %d, want %d", i, gotIndices[i], tc.wantIndices[i])
				}
			}

			// 3. CountDelimsSWAR
			count := CountDelimsSWAR(b, tc.delim)
			if count != len(tc.wantIndices) {
				t.Errorf("CountDelimsSWAR(%q) = %d, want %d", tc.input, count, len(tc.wantIndices))
			}
		})
	}
}

func TestSWAR_EscapedQuotesAcrossWordBoundaries(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		delim        byte
		wantIndices  []int
		wantFirstIdx int
		wantInQuotes bool
	}{
		{
			name: "BackslashAtByte7_QuoteAtByte8_InsideQuotes",
			// byte 0: ", bytes 1..6: 'a', byte 7: \, byte 8: ", bytes 9..10: 'b', byte 11: ", byte 12: ,
			input:        "\"aaaaaa\\\"bb\",end",
			delim:        ',',
			wantIndices:  []int{12},
			wantFirstIdx: 12,
			wantInQuotes: false,
		},
		{
			name: "DoubleBackslashAtBytes6And7_QuoteAtByte8_InsideQuotes",
			// \\ inside quotes escapes the slash, so " at byte 8 CLOSES the quote!
			// byte 0: ", bytes 1..5: 'a', byte 6: \, byte 7: \, byte 8: ", byte 9: ,
			input:        "\"aaaaa\\\\\",next",
			delim:        ',',
			wantIndices:  []int{9},
			wantFirstIdx: 9,
			wantInQuotes: false,
		},
		{
			name: "BackslashOutsideQuotesAtByte7",
			// outside quotes, \ is a literal, doesn't escape " at byte 8
			// bytes 0..6: 'a', byte 7: \, byte 8: ", byte 9: 'b', byte 10: ", byte 11: ,
			input:        "aaaaaaa\\\"b\",next",
			delim:        ',',
			wantIndices:  []int{11},
			wantFirstIdx: 11,
			wantInQuotes: false,
		},
		{
			name:         "UnterminatedQuotedString",
			input:        `"unterminated string without closing quote, continuing`,
			delim:        ',',
			wantIndices:  nil,
			wantFirstIdx: -1,
			wantInQuotes: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := []byte(tc.input)
			first := IndexUnquotedSWAR(b, tc.delim)
			if first != tc.wantFirstIdx {
				t.Errorf("IndexUnquotedSWAR = %d, want %d", first, tc.wantFirstIdx)
			}

			indices, inQuotes := FindDelimsSWAR(b, tc.delim, nil)
			if inQuotes != tc.wantInQuotes {
				t.Errorf("inQuotes = %v, want %v", inQuotes, tc.wantInQuotes)
			}
			if len(indices) != len(tc.wantIndices) {
				t.Fatalf("indices = %v, want %v", indices, tc.wantIndices)
			}
			for i := range indices {
				if indices[i] != tc.wantIndices[i] {
					t.Errorf("index[%d] = %d, want %d", i, indices[i], tc.wantIndices[i])
				}
			}
		})
	}
}

func TestSWAR_OddLengthsAndTails(t *testing.T) {
	// Test various buffer lengths from 0 to 65
	for length := 0; length <= 65; length++ {
		prefix := bytes.Repeat([]byte("x"), length)
		// Append ",tail"
		input := append(prefix, []byte(",tail")...)
		expectedIdx := length

		first := IndexUnquotedSWAR(input, ',')
		if first != expectedIdx {
			t.Errorf("length %d: IndexUnquotedSWAR = %d, want %d", length, first, expectedIdx)
		}

		indices, inQuotes := FindDelimsSWAR(input, ',', nil)
		if inQuotes {
			t.Errorf("length %d: unexpectedly inQuotes", length)
		}
		if len(indices) != 1 || indices[0] != expectedIdx {
			t.Errorf("length %d: FindDelimsSWAR = %v, want [%d]", length, indices, expectedIdx)
		}
	}
}

func TestSWAR_ParityWithParse(t *testing.T) {
	testSamples := []struct {
		input string
		delim byte
	}{
		{`1,2,3,4,5`, ','},
		{`"a,b",c,"d,e",f`, ','},
		{`"escaped \" quote",hello,world`, ','},
		{`"backslash \\",test`, ','},
		{`plain|pipe|separated|values`, '|'},
		{`"quoted | pipe"|unquoted|"another | pipe"`, '|'},
		{"col1\tcol2\tcol3", '\t'},
		{"\"col\t1\"\t\"col\t2\"\tcol3", '\t'},
		{`foo,bar,"baz"`, ','},
		{`"",123,""`, ','},
		{`x,"y",z`, ','},
		{`a,b\c,d`, ','},
	}

	indexUnquotedScalar := func(s string, target byte) int {
		inQuotes := false
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case '\\':
				if inQuotes {
					i++
				}
			case '"':
				inQuotes = !inQuotes
			default:
				if !inQuotes && s[i] == target {
					return i
				}
			}
		}
		return -1
	}

	splitDelimsScalar := func(s string, delim byte) ([]string, error) {
		var tokens []string
		inQuotes := false
		escaped := false
		start := 0
		for i := 0; i < len(s); i++ {
			b := s[i]
			switch {
			case escaped:
				escaped = false
			case b == '\\' && inQuotes:
				escaped = true
			case b == '"':
				inQuotes = !inQuotes
			case b == delim && !inQuotes:
				tokens = append(tokens, strings.Trim(s[start:i], " "))
				start = i + 1
			}
		}
		if inQuotes {
			return nil, fmt.Errorf("unterminated string")
		}
		tokens = append(tokens, strings.Trim(s[start:], " "))
		return tokens, nil
	}

	for i, sample := range testSamples {
		b := []byte(sample.input)

		// 1. Check IndexUnquoted parity
		expectedFirst := indexUnquotedScalar(sample.input, sample.delim)
		gotFirst := IndexUnquotedSWAR(b, sample.delim)
		if gotFirst != expectedFirst {
			t.Errorf("sample %d %q: IndexUnquotedSWAR = %d, scalar = %d", i, sample.input, gotFirst, expectedFirst)
		}

		// 2. Check SplitInlineValues parity
		expectedTokens, expErr := splitDelimsScalar(sample.input, sample.delim)
		indices, inQuotes := FindDelimsSWAR(b, sample.delim, nil)
		if expErr != nil {
			if !inQuotes {
				t.Errorf("sample %d %q: expected parse error %v, but inQuotes was false", i, sample.input, expErr)
			}
			continue
		}
		if inQuotes {
			t.Errorf("sample %d %q: inQuotes is true but parse succeeded", i, sample.input)
		}

		// Reconstruct tokens from indices
		var gotTokens []string
		start := 0
		for _, idx := range indices {
			tok := strings.Trim(sample.input[start:idx], " ")
			gotTokens = append(gotTokens, tok)
			start = idx + 1
		}
		gotTokens = append(gotTokens, strings.Trim(sample.input[start:], " "))

		if len(gotTokens) != len(expectedTokens) {
			t.Fatalf("sample %d %q: got %d tokens %v, want %d tokens %v", i, sample.input, len(gotTokens), gotTokens, len(expectedTokens), expectedTokens)
		}
		for j := range gotTokens {
			if gotTokens[j] != expectedTokens[j] {
				t.Errorf("sample %d %q: token[%d] = %q, want %q", i, sample.input, j, gotTokens[j], expectedTokens[j])
			}
		}
	}
}

func TestAVX2_DifferentialAgainstSWAR_10000Rows(t *testing.T) {
	if !HasAVX2() || !HasBMI2() {
		t.Skip("skipping AVX2 differential test: host CPU does not support AVX2+BMI2")
	}

	rng := rand.New(rand.NewSource(42))
	delims := []byte{',', '\t', '|', ';', ':'}

	for rowIdx := 0; rowIdx < 10000; rowIdx++ {
		delim := delims[rng.Intn(len(delims))]
		numCols := rng.Intn(20) + 1
		var rowParts [][]byte

		for col := 0; col < numCols; col++ {
			fieldMode := rng.Intn(6)
			var field []byte
			switch fieldMode {
			case 0: // Plain alphanumeric
				length := rng.Intn(40)
				field = make([]byte, length)
				for k := range field {
					field[k] = byte('a' + rng.Intn(26))
				}
			case 1: // Quoted with embedded delimiter
				field = []byte(fmt.Sprintf("\"field_%d_%c_val\"", col, delim))
			case 2: // Escaped quotes inside quoted string
				field = []byte(`"escaped \" quote"`)
			case 3: // Double backslashes
				field = []byte(`"escaped \\ backslash"`)
			case 4: // Empty field
				field = []byte("")
			case 5: // Variable length padding spanning 32-byte chunk boundary
				length := 31 + (rng.Intn(5) - 2) // 29..33 bytes
				field = bytes.Repeat([]byte{'x'}, length)
			}
			rowParts = append(rowParts, field)
		}

		row := bytes.Join(rowParts, []byte{delim})

		// SWAR Baseline Outputs
		wantIndices, wantInQuotes := FindDelimsSWAR(row, delim, nil)
		wantCount := ScanDelimSWAR(row, delim)

		// AVX2 Kernel Outputs
		gotIndices, gotInQuotes := FindDelimsAVX2(row, delim, nil)
		gotCount := ScanDelimAVX2(row, delim)

		if gotInQuotes != wantInQuotes {
			t.Fatalf("row %d: inQuotes mismatch: AVX2=%v, SWAR=%v, input=%q", rowIdx, gotInQuotes, wantInQuotes, row)
		}
		if gotCount != wantCount {
			t.Fatalf("row %d: count mismatch: AVX2=%d, SWAR=%d, input=%q", rowIdx, gotCount, wantCount, row)
		}
		if len(gotIndices) != len(wantIndices) {
			t.Fatalf("row %d: indices length mismatch: AVX2=%v (len %d), SWAR=%v (len %d), input=%q",
				rowIdx, gotIndices, len(gotIndices), wantIndices, len(wantIndices), row)
		}
		for k := range gotIndices {
			if gotIndices[k] != wantIndices[k] {
				t.Fatalf("row %d: index[%d] mismatch: AVX2=%d, SWAR=%d, input=%q",
					rowIdx, k, gotIndices[k], wantIndices[k], row)
			}
		}
	}
}

func TestAVX2_FallbackOnSimulatedDisabled(t *testing.T) {
	restore := SetCPUFeaturesForTest(CPUFeatures{HasAVX2: false, HasBMI2: false, HasNEON: false})
	defer restore()

	input := []byte("users[2]{id,name}:\n 1,alice\n 2,bob\n")
	indices, inQuotes := FindDelimsAVX2(input, ',', nil)
	swarIndices, swarInQuotes := FindDelimsSWAR(input, ',', nil)

	if inQuotes != swarInQuotes {
		t.Errorf("inQuotes mismatch: %v vs %v", inQuotes, swarInQuotes)
	}
	if len(indices) != len(swarIndices) {
		t.Fatalf("indices length mismatch: %v vs %v", indices, swarIndices)
	}
	for i := range indices {
		if indices[i] != swarIndices[i] {
			t.Errorf("index[%d] = %d, want %d", i, indices[i], swarIndices[i])
		}
	}

	count := CountDelimsAVX2(input, ',')
	swarCount := CountDelimsSWAR(input, ',')
	if count != swarCount {
		t.Errorf("count = %d, want %d", count, swarCount)
	}
}

func TestNEON_DifferentialAgainstSWAR_10000Rows(t *testing.T) {
	if !HasNEON() {
		t.Skip("skipping NEON differential test: host CPU does not support NEON")
	}

	rng := rand.New(rand.NewSource(12345))
	delims := []byte{',', '\t', '|', ';', ':'}

	for rowIdx := 0; rowIdx < 10000; rowIdx++ {
		delim := delims[rng.Intn(len(delims))]
		numCols := rng.Intn(20) + 1
		var rowParts [][]byte

		for col := 0; col < numCols; col++ {
			fieldMode := rng.Intn(6)
			var field []byte
			switch fieldMode {
			case 0: // Plain alphanumeric
				length := rng.Intn(40)
				field = make([]byte, length)
				for k := range field {
					field[k] = byte('a' + rng.Intn(26))
				}
			case 1: // Quoted with embedded delimiter
				field = []byte(fmt.Sprintf("\"field_%d_%c_val\"", col, delim))
			case 2: // Escaped quotes inside quoted string
				field = []byte(`"escaped \" quote"`)
			case 3: // Double backslashes
				field = []byte(`"escaped \\ backslash"`)
			case 4: // Empty field
				field = []byte("")
			case 5: // Variable length padding spanning 16-byte chunk boundary
				length := 15 + (rng.Intn(5) - 2) // 13..17 bytes
				field = bytes.Repeat([]byte{'x'}, length)
			}
			rowParts = append(rowParts, field)
		}

		row := bytes.Join(rowParts, []byte{delim})

		// SWAR Baseline Outputs
		wantIndices, wantInQuotes := FindDelimsSWAR(row, delim, nil)
		wantCount := ScanDelimSWAR(row, delim)

		// NEON Kernel Outputs
		gotIndices, gotInQuotes := FindDelimsNEON(row, delim, nil)
		gotCount := ScanDelimNEON(row, delim)

		if gotInQuotes != wantInQuotes {
			t.Fatalf("row %d: inQuotes mismatch: NEON=%v, SWAR=%v, input=%q", rowIdx, gotInQuotes, wantInQuotes, row)
		}
		if gotCount != wantCount {
			t.Fatalf("row %d: count mismatch: NEON=%d, SWAR=%d, input=%q", rowIdx, gotCount, wantCount, row)
		}
		if len(gotIndices) != len(wantIndices) {
			t.Fatalf("row %d: indices length mismatch: NEON=%v (len %d), SWAR=%v (len %d), input=%q",
				rowIdx, gotIndices, len(gotIndices), wantIndices, len(wantIndices), row)
		}
		for k := range gotIndices {
			if gotIndices[k] != wantIndices[k] {
				t.Fatalf("row %d: index[%d] mismatch: NEON=%d, SWAR=%d, input=%q",
					rowIdx, k, gotIndices[k], wantIndices[k], row)
			}
		}
	}
}

func TestNEON_AlignedAndUnalignedBuffers(t *testing.T) {
	if !HasNEON() {
		t.Skip("skipping NEON test: host CPU does not support NEON")
	}

	for size := 0; size <= 128; size++ {
		for delimPos := 0; delimPos < size; delimPos++ {
			buf := make([]byte, size)
			for i := range buf {
				buf[i] = 'a'
			}
			buf[delimPos] = ','

			wantIndices, wantQuotes := FindDelimsSWAR(buf, ',', nil)
			gotIndices, gotQuotes := FindDelimsNEON(buf, ',', nil)

			if wantQuotes != gotQuotes {
				t.Fatalf("size %d delimPos %d: inQuotes mismatch: NEON=%v, SWAR=%v", size, delimPos, gotQuotes, wantQuotes)
			}
			if len(wantIndices) != len(gotIndices) || (len(wantIndices) > 0 && wantIndices[0] != gotIndices[0]) {
				t.Fatalf("size %d delimPos %d: indices mismatch: NEON=%v, SWAR=%v", size, delimPos, gotIndices, wantIndices)
			}

			wantCount := ScanDelimSWAR(buf, ',')
			gotCount := ScanDelimNEON(buf, ',')
			if wantCount != gotCount {
				t.Fatalf("size %d delimPos %d: count mismatch: NEON=%d, SWAR=%d", size, delimPos, gotCount, wantCount)
			}
		}
	}
}

func TestSWAR_Classifier_AllASCII(t *testing.T) {
	delims := []byte{',', '\t', '|', ';', 0}

	for b := 0; b < 128; b++ {
		ch := byte(b)
		single := []byte{ch}

		// Test single byte
		wantEscape := HasEscapeOrControlScalar(single)
		gotEscape := HasEscapeOrControlSWAR(single)
		if wantEscape != gotEscape {
			t.Fatalf("ASCII 0x%02X (%q) HasEscapeOrControl: got %v, want %v", ch, ch, gotEscape, wantEscape)
		}

		wantEscapeIdx := IndexEscapeOrControlScalar(single)
		gotEscapeIdx := IndexEscapeOrControlSWAR(single)
		if wantEscapeIdx != gotEscapeIdx {
			t.Fatalf("ASCII 0x%02X (%q) IndexEscapeOrControl: got %d, want %d", ch, ch, gotEscapeIdx, wantEscapeIdx)
		}

		for _, delim := range delims {
			wantSpecial := HasSpecialOrControlScalar(single, delim)
			gotSpecial := HasSpecialOrControlSWAR(single, delim)
			if wantSpecial != gotSpecial {
				t.Fatalf("ASCII 0x%02X (%q) HasSpecialOrControl (delim %q): got %v, want %v", ch, ch, delim, gotSpecial, wantSpecial)
			}

			wantSpecialIdx := IndexSpecialOrControlScalar(single, delim)
			gotSpecialIdx := IndexSpecialOrControlSWAR(single, delim)
			if wantSpecialIdx != gotSpecialIdx {
				t.Fatalf("ASCII 0x%02X (%q) IndexSpecialOrControl (delim %q): got %d, want %d", ch, ch, delim, gotSpecialIdx, wantSpecialIdx)
			}
		}

		// Test embedded in various buffer lengths and positions (8, 9, 15, 16, 23, 32, 64)
		testSizes := []int{8, 9, 15, 16, 23, 32, 64}
		for _, size := range testSizes {
			positions := []int{0, 1, 7, size / 2, size - 1}
			for _, pos := range positions {
				buf := bytes.Repeat([]byte("a"), size)
				buf[pos] = ch

				wantEsc := HasEscapeOrControlScalar(buf)
				gotEsc := HasEscapeOrControlSWAR(buf)
				if wantEsc != gotEsc {
					t.Fatalf("size %d pos %d ASCII 0x%02X (%q) HasEscapeOrControl: got %v, want %v", size, pos, ch, ch, gotEsc, wantEsc)
				}
				wantEscIdx := IndexEscapeOrControlScalar(buf)
				gotEscIdx := IndexEscapeOrControlSWAR(buf)
				if wantEscIdx != gotEscIdx {
					t.Fatalf("size %d pos %d ASCII 0x%02X (%q) IndexEscapeOrControl: got %d, want %d", size, pos, ch, ch, gotEscIdx, wantEscIdx)
				}

				for _, delim := range delims {
					wantSpec := HasSpecialOrControlScalar(buf, delim)
					gotSpec := HasSpecialOrControlSWAR(buf, delim)
					if wantSpec != gotSpec {
						t.Fatalf("size %d pos %d ASCII 0x%02X (%q) HasSpecialOrControl (delim %q): got %v, want %v", size, pos, ch, ch, delim, gotSpec, wantSpec)
					}
					wantSpecIdx := IndexSpecialOrControlScalar(buf, delim)
					gotSpecIdx := IndexSpecialOrControlSWAR(buf, delim)
					if wantSpecIdx != gotSpecIdx {
						t.Fatalf("size %d pos %d ASCII 0x%02X (%q) IndexSpecialOrControl (delim %q): got %d, want %d", size, pos, ch, ch, delim, gotSpecIdx, wantSpecIdx)
					}
				}
			}
		}
	}
}

func TestSWAR_Classifier_UTF8Sequences(t *testing.T) {
	cleanUTF8Strings := []string{
		"Hello World",
		"café crème brûlée",
		"日本語のテスト文字列です",
		"你好世界，这是一个测试",
		"안녕하세요 반갑습니다",
		"Привет мир и вселенная",
		"Γειά σου κόσμε όμορφε",
		"🚀🌟🎉🔥🤖📦🔧💡",
		"Mixed english and 中文 and emoji 🚀 works seamlessly",
	}

	for _, s := range cleanUTF8Strings {
		data := []byte(s)

		if HasEscapeOrControlSWAR(data) {
			t.Fatalf("clean UTF-8 string %q falsely flagged by HasEscapeOrControlSWAR", s)
		}
		if HasSpecialOrControlSWAR(data, ',') {
			t.Fatalf("clean UTF-8 string %q falsely flagged by HasSpecialOrControlSWAR with comma delim", s)
		}
		if NeedsQuotingSWAR(data, 0) {
			t.Fatalf("clean UTF-8 string %q falsely flagged by NeedsQuotingSWAR", s)
		}

		// Inject special structural characters and verify detection
		specials := []byte{':', '\\', '"', '[', ']', '{', '}', '\n', '\t', '\x00'}
		for _, sp := range specials {
			for pos := 0; pos <= len(data); pos++ {
				modified := make([]byte, 0, len(data)+1)
				modified = append(modified, data[:pos]...)
				modified = append(modified, sp)
				modified = append(modified, data[pos:]...)

				if !HasSpecialOrControlSWAR(modified, 0) {
					t.Fatalf("failed to detect special 0x%02X in UTF-8 string at pos %d: %q", sp, pos, modified)
				}
				idx := IndexSpecialOrControlSWAR(modified, 0)
				wantIdx := IndexSpecialOrControlScalar(modified, 0)
				if idx != wantIdx {
					t.Fatalf("special 0x%02X pos mismatch: got %d, want %d", sp, idx, wantIdx)
				}
			}
		}
	}

	// Verify all byte values in 0x80..0xFF (continuation and lead bytes) never trigger false positives
	for b := 0x80; b <= 0xFF; b++ {
		ch := byte(b)
		buf := make([]byte, 64)
		for i := range buf {
			buf[i] = ch
		}

		if HasEscapeOrControlSWAR(buf) {
			t.Fatalf("byte 0x%02X falsely flagged by HasEscapeOrControlSWAR", ch)
		}
		if HasSpecialOrControlSWAR(buf, ',') {
			t.Fatalf("byte 0x%02X falsely flagged by HasSpecialOrControlSWAR", ch)
		}
	}
}

func TestSWAR_Classifier_DifferentialFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	charset := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-. :/\\\"[]{} \t\r\n\x00\x01\x1F\x7F")
	// Add multi-byte UTF-8 bytes
	utf8Bytes := []byte("世é🚀")
	charset = append(charset, utf8Bytes...)

	delims := []byte{',', '\t', '|', ';', ':', 0}

	for iter := 0; iter < 5000; iter++ {
		length := rng.Intn(130)
		buf := make([]byte, length)
		for i := range buf {
			buf[i] = charset[rng.Intn(len(charset))]
		}
		delim := delims[rng.Intn(len(delims))]

		wantEsc := HasEscapeOrControlScalar(buf)
		gotEsc := HasEscapeOrControlSWAR(buf)
		if wantEsc != gotEsc {
			t.Fatalf("iter %d: HasEscapeOrControl mismatch: got %v, want %v, buf=%q", iter, gotEsc, wantEsc, buf)
		}

		wantEscIdx := IndexEscapeOrControlScalar(buf)
		gotEscIdx := IndexEscapeOrControlSWAR(buf)
		if wantEscIdx != gotEscIdx {
			t.Fatalf("iter %d: IndexEscapeOrControl mismatch: got %d, want %d, buf=%q", iter, gotEscIdx, wantEscIdx, buf)
		}

		wantSpec := HasSpecialOrControlScalar(buf, delim)
		gotSpec := HasSpecialOrControlSWAR(buf, delim)
		if wantSpec != gotSpec {
			t.Fatalf("iter %d: HasSpecialOrControl mismatch (delim %q): got %v, want %v, buf=%q", iter, delim, gotSpec, wantSpec, buf)
		}

		wantSpecIdx := IndexSpecialOrControlScalar(buf, delim)
		gotSpecIdx := IndexSpecialOrControlSWAR(buf, delim)
		if wantSpecIdx != gotSpecIdx {
			t.Fatalf("iter %d: IndexSpecialOrControl mismatch (delim %q): got %d, want %d, buf=%q", iter, delim, gotSpecIdx, wantSpecIdx, buf)
		}
	}
}

func BenchmarkCharacterClassification(b *testing.B) {
	sizes := []int{16, 64, 512, 4096}
	for _, size := range sizes {
		clean := bytes.Repeat([]byte("AlphaNumericValue12345_Test"), (size/27)+1)[:size]

		b.Run(fmt.Sprintf("NeedsQuoting_Scalar_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = NeedsQuotingScalar(clean, ',')
			}
		})

		b.Run(fmt.Sprintf("NeedsQuoting_SWAR_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = NeedsQuotingSWAR(clean, ',')
			}
		})

		b.Run(fmt.Sprintf("HasEscapeOrControl_Scalar_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = HasEscapeOrControlScalar(clean)
			}
		})

		b.Run(fmt.Sprintf("HasEscapeOrControl_SWAR_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = HasEscapeOrControlSWAR(clean)
			}
		})

		b.Run(fmt.Sprintf("NeedsQuoting_AVX2_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = NeedsQuotingAVX2(clean, ',')
			}
		})

		b.Run(fmt.Sprintf("HasEscapeOrControl_AVX2_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = HasEscapeOrControlAVX2(clean)
			}
		})

		b.Run(fmt.Sprintf("NeedsQuoting_NEON_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = NeedsQuotingNEON(clean, ',')
			}
		})

		b.Run(fmt.Sprintf("HasEscapeOrControl_NEON_%dB", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = HasEscapeOrControlNEON(clean)
			}
		})
	}
}

func TestAVX2_Classifier_FallbackOnSimulatedDisabled(t *testing.T) {
	restore := SetCPUFeaturesForTest(CPUFeatures{HasAVX2: false, HasBMI2: false, HasNEON: false})
	defer restore()

	inputs := []string{
		"plain_simple_text",
		"users[2]{id,name}:",
		"line\nbreak",
		"tab\tseparated",
		`escaped\"quote`,
		"unicode: 你好世界 🚀",
	}

	for _, s := range inputs {
		data := []byte(s)

		gotEsc := HasEscapeOrControlAVX2(data)
		wantEsc := HasEscapeOrControlSWAR(data)
		if gotEsc != wantEsc {
			t.Fatalf("input %q: HasEscapeOrControlAVX2 = %v, want %v", s, gotEsc, wantEsc)
		}

		gotEscIdx := IndexEscapeOrControlAVX2(data)
		wantEscIdx := IndexEscapeOrControlSWAR(data)
		if gotEscIdx != wantEscIdx {
			t.Fatalf("input %q: IndexEscapeOrControlAVX2 = %d, want %d", s, gotEscIdx, wantEscIdx)
		}

		gotSpec := HasSpecialOrControlAVX2(data, ',')
		wantSpec := HasSpecialOrControlSWAR(data, ',')
		if gotSpec != wantSpec {
			t.Fatalf("input %q: HasSpecialOrControlAVX2 = %v, want %v", s, gotSpec, wantSpec)
		}

		gotSpecIdx := IndexSpecialOrControlAVX2(data, ',')
		wantSpecIdx := IndexSpecialOrControlSWAR(data, ',')
		if gotSpecIdx != wantSpecIdx {
			t.Fatalf("input %q: IndexSpecialOrControlAVX2 = %d, want %d", s, gotSpecIdx, wantSpecIdx)
		}
	}
}

func TestAVX2_Classifier_WideCharacterCorpora(t *testing.T) {
	corpora := []string{
		// English plain
		"alphaNumericValue12345_Test_Identifier_Here",
		// Latin extended
		"Les naïfs méritent des récompenses pour leur créativité",
		// Cyrillic
		"Быстрая бурая лисица прыгает через ленивую собаку",
		// CJK
		"春眠不觉晓，处处闻啼鸟。夜来风雨声，花落知多少。",
		// Japanese Kana & Kanji
		"いろはにほへと ちりぬるを わかよたれそ つねならむ",
		// Korean Hangul
		"다람쥐 헌 쳇바퀴에 타고파. 무궁화 꽃이 피었습니다.",
		// Greek
		"Ξεσκεπάζω την ψυχοφθόρα βδελυγμία του καθωσπρεπισμού.",
		// Arabic
		"نص حكيم له سر قاطع وذو شأن عظيم مكتوب على ثوب أخضر ومذهب",
		// Emojis & Symbols
		"🚀✨🎉🔥🤖💡📦🛠️⚡💻🎯🏆",
		// Long text spanning multiple 32-byte chunks (> 100 bytes)
		"The quick brown fox jumps over the lazy dog. A large continuous text block designed to span multiple vector chunks.",
	}

	delims := []byte{',', '\t', '|', ';', 0}

	for _, text := range corpora {
		data := []byte(text)

		// Base clean string should not require escaping or special quoting (no delim)
		if HasEscapeOrControlAVX2(data) != HasEscapeOrControlScalar(data) {
			t.Fatalf("corpora %q: HasEscapeOrControlAVX2 mismatch", text)
		}
		if HasSpecialOrControlAVX2(data, 0) != HasSpecialOrControlScalar(data, 0) {
			t.Fatalf("corpora %q: HasSpecialOrControlAVX2 mismatch with delim 0", text)
		}

		// Inject special characters into every position of the corpora
		specials := []byte{':', '\\', '"', '[', ']', '{', '}', '\n', '\r', '\t', '\x00'}
		for _, sp := range specials {
			for pos := 0; pos <= len(data); pos++ {
				modified := make([]byte, 0, len(data)+1)
				modified = append(modified, data[:pos]...)
				modified = append(modified, sp)
				modified = append(modified, data[pos:]...)

				for _, delim := range delims {
					wantSpecial := HasSpecialOrControlScalar(modified, delim)
					gotSpecial := HasSpecialOrControlAVX2(modified, delim)
					if wantSpecial != gotSpecial {
						t.Fatalf("modified corpora (%q at pos %d, delim %q): got %v, want %v", sp, pos, delim, gotSpecial, wantSpecial)
					}

					wantSpecialIdx := IndexSpecialOrControlScalar(modified, delim)
					gotSpecialIdx := IndexSpecialOrControlAVX2(modified, delim)
					if wantSpecialIdx != gotSpecialIdx {
						t.Fatalf("modified corpora (%q at pos %d, delim %q) index: got %d, want %d", sp, pos, delim, gotSpecialIdx, wantSpecialIdx)
					}
				}
			}
		}
	}
}

func TestNEON_Classifier_FallbackOnSimulatedDisabled(t *testing.T) {
	restore := SetCPUFeaturesForTest(CPUFeatures{HasAVX2: false, HasBMI2: false, HasNEON: false})
	defer restore()

	inputs := []string{
		"plain_simple_text",
		"users[2]{id,name}:",
		"line\nbreak",
		"tab\tseparated",
		`escaped\"quote`,
		"unicode: 你好世界 🚀",
	}

	for _, s := range inputs {
		data := []byte(s)

		gotEsc := HasEscapeOrControlNEON(data)
		wantEsc := HasEscapeOrControlSWAR(data)
		if gotEsc != wantEsc {
			t.Fatalf("input %q: HasEscapeOrControlNEON = %v, want %v", s, gotEsc, wantEsc)
		}

		gotEscIdx := IndexEscapeOrControlNEON(data)
		wantEscIdx := IndexEscapeOrControlSWAR(data)
		if gotEscIdx != wantEscIdx {
			t.Fatalf("input %q: IndexEscapeOrControlNEON = %d, want %d", s, gotEscIdx, wantEscIdx)
		}

		gotSpec := HasSpecialOrControlNEON(data, ',')
		wantSpec := HasSpecialOrControlSWAR(data, ',')
		if gotSpec != wantSpec {
			t.Fatalf("input %q: HasSpecialOrControlNEON = %v, want %v", s, gotSpec, wantSpec)
		}

		gotSpecIdx := IndexSpecialOrControlNEON(data, ',')
		wantSpecIdx := IndexSpecialOrControlSWAR(data, ',')
		if gotSpecIdx != wantSpecIdx {
			t.Fatalf("input %q: IndexSpecialOrControlNEON = %d, want %d", s, gotSpecIdx, wantSpecIdx)
		}
	}
}

func TestNEON_Classifier_WideCharacterCorpora(t *testing.T) {
	corpora := []string{
		// English plain
		"alphaNumericValue12345_Test_Identifier_Here",
		// Latin extended
		"Les naïfs méritent des récompenses pour leur créativité",
		// Cyrillic
		"Быстрая бурая лисица прыгает через ленивую собаку",
		// CJK
		"春眠不觉晓，处处闻啼鸟。夜来风雨声，花落知多少。",
		// Japanese Kana & Kanji
		"いろはにほへと ちりぬるを わかよたれそ つねならむ",
		// Korean Hangul
		"다람쥐 헌 쳇바퀴에 타고파. 무궁화 꽃이 피었습니다.",
		// Greek
		"Ξεσκεπάζω την ψυχοφθόρα βδελυγμία του καθωσπρεπισμού.",
		// Arabic
		"نص حكيم له سر قاطع وذو شأن عظيم مكتوب على ثوب أخضر ومذهب",
		// Emojis & Symbols
		"🚀✨🎉🔥🤖💡📦🛠️⚡💻🎯🏆",
		// Long text spanning multiple 16-byte chunks (> 100 bytes)
		"The quick brown fox jumps over the lazy dog. A large continuous text block designed to span multiple vector chunks.",
	}

	delims := []byte{',', '\t', '|', ';', 0}

	for _, text := range corpora {
		data := []byte(text)

		if HasEscapeOrControlNEON(data) != HasEscapeOrControlScalar(data) {
			t.Fatalf("corpora %q: HasEscapeOrControlNEON mismatch", text)
		}
		if HasSpecialOrControlNEON(data, 0) != HasSpecialOrControlScalar(data, 0) {
			t.Fatalf("corpora %q: HasSpecialOrControlNEON mismatch with delim 0", text)
		}

		// Inject special characters into every position of the corpora
		specials := []byte{':', '\\', '"', '[', ']', '{', '}', '\n', '\r', '\t', '\x00'}
		for _, sp := range specials {
			for pos := 0; pos <= len(data); pos++ {
				modified := make([]byte, 0, len(data)+1)
				modified = append(modified, data[:pos]...)
				modified = append(modified, sp)
				modified = append(modified, data[pos:]...)

				for _, delim := range delims {
					wantSpecial := HasSpecialOrControlScalar(modified, delim)
					gotSpecial := HasSpecialOrControlNEON(modified, delim)
					if wantSpecial != gotSpecial {
						t.Fatalf("modified corpora (%q at pos %d, delim %q): got %v, want %v", sp, pos, delim, gotSpecial, wantSpecial)
					}

					wantSpecialIdx := IndexSpecialOrControlScalar(modified, delim)
					gotSpecialIdx := IndexSpecialOrControlNEON(modified, delim)
					if wantSpecialIdx != gotSpecialIdx {
						t.Fatalf("modified corpora (%q at pos %d, delim %q) index: got %d, want %d", sp, pos, delim, gotSpecialIdx, wantSpecialIdx)
					}
				}
			}
		}
	}
}

func TestNEON_Classifier_DifferentialAgainstSWAR_10000Rows(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	delims := []byte{',', '\t', '|', ';', ':'}
	charset := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-. :/\\\"[]{} \t\r\n\x00\x01\x1F\x7F世é🚀")

	for rowIdx := 0; rowIdx < 10000; rowIdx++ {
		length := rng.Intn(100)
		buf := make([]byte, length)
		for i := range buf {
			buf[i] = charset[rng.Intn(len(charset))]
		}
		delim := delims[rng.Intn(len(delims))]

		gotEsc := HasEscapeOrControlNEON(buf)
		wantEsc := HasEscapeOrControlSWAR(buf)
		if gotEsc != wantEsc {
			t.Fatalf("row %d: HasEscapeOrControl mismatch: NEON=%v, SWAR=%v, buf=%q", rowIdx, gotEsc, wantEsc, buf)
		}

		gotEscIdx := IndexEscapeOrControlNEON(buf)
		wantEscIdx := IndexEscapeOrControlSWAR(buf)
		if gotEscIdx != wantEscIdx {
			t.Fatalf("row %d: IndexEscapeOrControl mismatch: NEON=%d, SWAR=%d, buf=%q", rowIdx, gotEscIdx, wantEscIdx, buf)
		}

		gotSpec := HasSpecialOrControlNEON(buf, delim)
		wantSpec := HasSpecialOrControlSWAR(buf, delim)
		if gotSpec != wantSpec {
			t.Fatalf("row %d: HasSpecialOrControl mismatch: NEON=%v, SWAR=%v, delim=%q, buf=%q", rowIdx, gotSpec, wantSpec, delim, buf)
		}

		gotSpecIdx := IndexSpecialOrControlNEON(buf, delim)
		wantSpecIdx := IndexSpecialOrControlSWAR(buf, delim)
		if gotSpecIdx != wantSpecIdx {
			t.Fatalf("row %d: IndexSpecialOrControl mismatch: NEON=%d, SWAR=%d, delim=%q, buf=%q", rowIdx, gotSpecIdx, wantSpecIdx, delim, buf)
		}
	}
}

func TestScanLinesSWAR(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []int
	}{
		{"empty", "", nil},
		{"no break", "field: value", nil},
		{"lf", "a\nb\nc", []int{1, 3}},
		{"cr", "a\rb\rc", []int{1, 3}},
		{"crlf", "a\r\nb\r\nc", []int{1, 4}},
		{"mixed", "a\r\nb\nc\rd", []int{1, 4, 6}},
		{"word boundaries", "1234567\n1234567\r1234567\r\nend", []int{7, 15, 23}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScanLinesSWAR([]byte(tt.data), nil)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ScanLinesSWAR(%q) = %v, want %v", tt.data, got, tt.want)
			}
			prefix := []int{99}
			got = ScanLinesSWAR([]byte(tt.data), prefix)
			want := append([]int{99}, tt.want...)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ScanLinesSWAR append = %v, want %v", got, want)
			}
		})
	}
}

func TestComputeIndentSWAR(t *testing.T) {
	tests := []struct {
		data string
		want int
	}{
		{"", 0}, {"value", 0}, {" value", 1}, {"  value", 2},
		{"       value", 7}, {"        value", 8}, {"                value", 16},
		{"    \tvalue", 4}, {"  \u00a0value", 2}, {"                    ", 20},
	}
	for _, tt := range tests {
		got := ComputeIndentSWAR([]byte(tt.data))
		if got != tt.want {
			t.Errorf("ComputeIndentSWAR(%q) = %d, want %d", tt.data, got, tt.want)
		}
		if leading := LeadingSpacesSWAR([]byte(tt.data)); leading != tt.want {
			t.Errorf("LeadingSpacesSWAR(%q) = %d, want %d", tt.data, leading, tt.want)
		}
	}
}

func TestAVX2LineAndIndentDifferential(t *testing.T) {
	tests := []string{
		"",
		strings.Repeat("a", 31) + "\n",
		strings.Repeat("a", 31) + "\r\nend",
		strings.Repeat("a", 32) + "\r\nend",
		strings.Repeat("a", 33) + "\nend",
		"first\rsecond\nthird\r\nfourth",
	}
	for _, input := range tests {
		want := ScanLinesSWAR([]byte(input), nil)
		got := ScanLinesAVX2([]byte(input), nil)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ScanLinesAVX2(%q) = %v, want %v", input, got, want)
		}
		if got := ScanLinesNEON([]byte(input), nil); !reflect.DeepEqual(got, want) {
			t.Errorf("ScanLinesNEON(%q) = %v, want %v", input, got, want)
		}
	}

	for _, n := range []int{0, 1, 7, 31, 32, 33, 63, 64, 65, 96} {
		input := strings.Repeat(" ", n) + "\tvalue"
		want := LeadingSpacesSWAR([]byte(input))
		if got := LeadingSpacesAVX2([]byte(input)); got != want {
			t.Errorf("LeadingSpacesAVX2(%d spaces) = %d, want %d", n, got, want)
		}
		if got := ComputeIndentAVX2([]byte(input)); got != want {
			t.Errorf("ComputeIndentAVX2(%d spaces) = %d, want %d", n, got, want)
		}
		if got := LeadingSpacesNEON([]byte(input)); got != want {
			t.Errorf("LeadingSpacesNEON(%d spaces) = %d, want %d", n, got, want)
		}
		if got := ComputeIndentNEON([]byte(input)); got != want {
			t.Errorf("ComputeIndentNEON(%d spaces) = %d, want %d", n, got, want)
		}
	}

	rng := rand.New(rand.NewSource(20260904))
	for iter := 0; iter < 10000; iter++ {
		data := make([]byte, rng.Intn(160))
		for i := range data {
			switch rng.Intn(16) {
			case 0:
				data[i] = '\n'
			case 1:
				data[i] = '\r'
			case 2:
				data[i] = ' '
			default:
				data[i] = byte('a' + rng.Intn(26))
			}
		}
		wantLines := ScanLinesSWAR(data, nil)
		if gotLines := ScanLinesAVX2(data, nil); !reflect.DeepEqual(gotLines, wantLines) {
			t.Fatalf("iteration %d: ScanLinesAVX2(%q) = %v, want %v", iter, data, gotLines, wantLines)
		}
		if gotLines := ScanLinesNEON(data, nil); !reflect.DeepEqual(gotLines, wantLines) {
			t.Fatalf("iteration %d: ScanLinesNEON(%q) = %v, want %v", iter, data, gotLines, wantLines)
		}
		wantIndent := LeadingSpacesSWAR(data)
		if gotIndent := LeadingSpacesAVX2(data); gotIndent != wantIndent {
			t.Fatalf("iteration %d: LeadingSpacesAVX2(%q) = %d, want %d", iter, data, gotIndent, wantIndent)
		}
		if gotIndent := LeadingSpacesNEON(data); gotIndent != wantIndent {
			t.Fatalf("iteration %d: LeadingSpacesNEON(%q) = %d, want %d", iter, data, gotIndent, wantIndent)
		}
	}
}
