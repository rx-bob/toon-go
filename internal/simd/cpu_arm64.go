//go:build arm64 && !purego

package simd

func detectFeatures() CPUFeatures {
	// NEON (ASIMD) is mandatory on all 64-bit ARMv8-A platforms.
	return CPUFeatures{
		HasNEON: true,
	}
}
