//go:build arm64 && !purego

package simd

// HasEscapeOrControlAVX2 falls back to SWAR on ARM64 architectures.
func HasEscapeOrControlAVX2(data []byte) bool {
	return HasEscapeOrControlSWAR(data)
}

// IndexEscapeOrControlAVX2 falls back to SWAR on ARM64 architectures.
func IndexEscapeOrControlAVX2(data []byte) int {
	return IndexEscapeOrControlSWAR(data)
}

// HasSpecialOrControlAVX2 falls back to SWAR on ARM64 architectures.
func HasSpecialOrControlAVX2(data []byte, delim byte) bool {
	return HasSpecialOrControlSWAR(data, delim)
}

// IndexSpecialOrControlAVX2 falls back to SWAR on ARM64 architectures.
func IndexSpecialOrControlAVX2(data []byte, delim byte) int {
	return IndexSpecialOrControlSWAR(data, delim)
}

// NeedsQuotingAVX2 falls back to SWAR on ARM64 architectures.
func NeedsQuotingAVX2(data []byte, delim byte) bool {
	return HasSpecialOrControlSWAR(data, delim)
}
