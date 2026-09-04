//go:build (!amd64 && !arm64) || purego

package simd

// FindDelimsAVX2 falls back to SWAR on non-amd64 architectures.
func FindDelimsAVX2(data []byte, delim byte, dst []int) ([]int, bool) {
	return FindDelimsSWAR(data, delim, dst)
}

// CountDelimsAVX2 falls back to SWAR on non-amd64 architectures.
func CountDelimsAVX2(data []byte, delim byte) int {
	return CountDelimsSWAR(data, delim)
}

// FindDelimsNEON falls back to SWAR on non-arm64 architectures.
func FindDelimsNEON(data []byte, delim byte, dst []int) ([]int, bool) {
	return FindDelimsSWAR(data, delim, dst)
}

// CountDelimsNEON falls back to SWAR on non-arm64 architectures.
func CountDelimsNEON(data []byte, delim byte) int {
	return CountDelimsSWAR(data, delim)
}
