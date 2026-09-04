package simd

import (
	"bytes"
	"testing"
)

// CPU Profiling Documentation and Scriptable Commands:
//
// To capture CPU profiles during benchmark execution and inspect hot instruction paths:
//
// 1. Benchmark and capture profile for SWAR algorithm on 64KB buffer:
//    go test -run=^$ -bench=BenchmarkDelimScan/SWAR/64KB -cpuprofile=cpu_swar_64k.pprof -benchtime=3s ./internal/simd
//
// 2. Benchmark and capture profile for Scalar algorithm on 64KB buffer:
//    go test -run=^$ -bench=BenchmarkDelimScan/Scalar/64KB -cpuprofile=cpu_scalar_64k.pprof -benchtime=3s ./internal/simd
//
// 3. Inspect top CPU consumers in terminal:
//    go tool pprof -top cpu_swar_64k.pprof
//
// 4. Inspect interactive web visualization (flamegraph / source assembly view):
//    go tool pprof -http=:8080 cpu_swar_64k.pprof
//
// 5. Compare profiles side-by-side:
//    go tool pprof -base cpu_scalar_64k.pprof cpu_swar_64k.pprof
//

func generateTabularBuffer(targetSize int) []byte {
	seed := []byte("101,Alice Smith,\"Engineering, Infrastructure\",98.5,Active,admin\n")
	var buf bytes.Buffer
	for buf.Len() < targetSize {
		buf.Write(seed)
	}
	return buf.Bytes()[:targetSize]
}

func BenchmarkDelimScan(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{name: "64B", size: 64},
		{name: "256B", size: 256},
		{name: "1KB", size: 1024},
		{name: "16KB", size: 16 * 1024},
		{name: "64KB", size: 64 * 1024},
	}

	// Pre-generate identical test buffers for all algorithm variants.
	buffers := make(map[string][]byte, len(sizes))
	for _, sz := range sizes {
		buffers[sz.name] = generateTabularBuffer(sz.size)
	}

	algorithms := []struct {
		name         string
		fn           func([]byte, byte) int
		checkSupport func() bool
		skipReason   string
	}{
		{
			name: "Scalar",
			fn:   ScanDelimScalar,
		},
		{
			name: "SWAR",
			fn:   ScanDelimSWAR,
		},
		{
			name:         "AVX2",
			fn:           ScanDelimAVX2,
			checkSupport: HasAVX2,
			skipReason:   "AVX2 requires amd64 architecture",
		},
		{
			name:         "NEON",
			fn:           ScanDelimNEON,
			checkSupport: HasNEON,
			skipReason:   "NEON requires arm64 architecture",
		},
	}

	for _, algo := range algorithms {
		algo := algo
		b.Run(algo.name, func(b *testing.B) {
			if algo.checkSupport != nil && !algo.checkSupport() {
				b.Skip(algo.skipReason)
			}

			for _, sz := range sizes {
				sz := sz
				buf := buffers[sz.name]

				b.Run(sz.name, func(b *testing.B) {
					b.SetBytes(int64(len(buf)))
					b.ReportAllocs()
					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						count := algo.fn(buf, ',')
						_ = count
					}
				})
			}
		})
	}
}

func TestDelimScanCorrectness(t *testing.T) {
	testSizes := []int{10, 64, 127, 256, 513, 1024, 4096, 16384, 65536}

	for _, sz := range testSizes {
		buf := generateTabularBuffer(sz)
		scalarCount := ScanDelimScalar(buf, ',')
		swarCount := ScanDelimSWAR(buf, ',')

		if scalarCount != swarCount {
			t.Fatalf("size %d: SWAR count %d != Scalar count %d", sz, swarCount, scalarCount)
		}

		if HasAVX2() {
			avx2Count := ScanDelimAVX2(buf, ',')
			if avx2Count != scalarCount {
				t.Fatalf("size %d: AVX2 count %d != Scalar count %d", sz, avx2Count, scalarCount)
			}
		}

		if HasNEON() {
			neonCount := ScanDelimNEON(buf, ',')
			if neonCount != scalarCount {
				t.Fatalf("size %d: NEON count %d != Scalar count %d", sz, neonCount, scalarCount)
			}
		}
	}

	// Test special quote and escape boundary cases
	specialCases := []string{
		`a,b,"c,d",e`,
		`"quoted",unquoted,"quoted,with,comma"`,
		`"escaped \" quote",next,field`,
		`""`,
		`,,,,`,
		`a\,b,c`,
	}

	for _, sc := range specialCases {
		data := []byte(sc)
		expected := ScanDelimScalar(data, ',')
		gotSWAR := ScanDelimSWAR(data, ',')
		if gotSWAR != expected {
			t.Errorf("input %q: got SWAR %d, want %d", sc, gotSWAR, expected)
		}
		if HasNEON() {
			gotNEON := ScanDelimNEON(data, ',')
			if gotNEON != expected {
				t.Errorf("input %q: got NEON %d, want %d", sc, gotNEON, expected)
			}
		}
		if HasAVX2() {
			gotAVX2 := ScanDelimAVX2(data, ',')
			if gotAVX2 != expected {
				t.Errorf("input %q: got AVX2 %d, want %d", sc, gotAVX2, expected)
			}
		}
	}
}
