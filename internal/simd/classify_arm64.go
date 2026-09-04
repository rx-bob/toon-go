//go:build arm64 && !purego

package simd

func indexEscapeOrControlNEONAsm(data []byte) (index int, processed int)
func indexSpecialOrControlNEONAsm(data []byte, delim byte) (index int, processed int)

// HasEscapeOrControlNEON reports whether data contains an escape character ('\\')
// or control character (< 0x20) using ARM64 NEON with automatic SWAR fallback.
func HasEscapeOrControlNEON(data []byte) bool {
	return IndexEscapeOrControlNEON(data) >= 0
}

// IndexEscapeOrControlNEON returns the byte index of the first '\\' or < 0x20 character
// using ARM64 NEON with automatic SWAR fallback for tails and systems without NEON.
func IndexEscapeOrControlNEON(data []byte) int {
	if !HasNEON() {
		return IndexEscapeOrControlSWAR(data)
	}
	idx, processed := indexEscapeOrControlNEONAsm(data)
	if idx >= 0 {
		return idx
	}
	if processed < len(data) {
		remIdx := IndexEscapeOrControlSWAR(data[processed:])
		if remIdx >= 0 {
			return processed + remIdx
		}
	}
	return -1
}

// HasSpecialOrControlNEON reports whether data contains a special/control character
// or delim using ARM64 NEON with automatic SWAR fallback.
func HasSpecialOrControlNEON(data []byte, delim byte) bool {
	return IndexSpecialOrControlNEON(data, delim) >= 0
}

// IndexSpecialOrControlNEON returns the byte index of the first special, control,
// or delim byte using ARM64 NEON with automatic SWAR fallback.
func IndexSpecialOrControlNEON(data []byte, delim byte) int {
	if !HasNEON() {
		return IndexSpecialOrControlSWAR(data, delim)
	}
	idx, processed := indexSpecialOrControlNEONAsm(data, delim)
	if idx >= 0 {
		return idx
	}
	if processed < len(data) {
		remIdx := IndexSpecialOrControlSWAR(data[processed:], delim)
		if remIdx >= 0 {
			return processed + remIdx
		}
	}
	return -1
}

// NeedsQuotingNEON reports whether data contains characters that require quoting in TOON using ARM64 NEON.
func NeedsQuotingNEON(data []byte, delim byte) bool {
	return HasSpecialOrControlNEON(data, delim)
}

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
