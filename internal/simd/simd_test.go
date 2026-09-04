package simd

import (
	"bytes"
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
