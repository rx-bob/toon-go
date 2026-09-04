//go:build (!amd64 && !arm64) || purego

package simd

func detectFeatures() CPUFeatures {
	return CPUFeatures{
		HasAVX2: false,
		HasBMI2: false,
		HasNEON: false,
	}
}
