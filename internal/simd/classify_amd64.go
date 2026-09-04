//go:build amd64 && !purego

package simd

//go:generate go run ./gen/classify.go -out asm_classify_amd64.s

func indexEscapeOrControlAVX2Asm(data []byte) (index int, processed int)
func indexSpecialOrControlAVX2Asm(data []byte, delim byte) (index int, processed int)

// HasEscapeOrControlAVX2 reports whether data contains an escape character ('\\')
// or control character (< 0x20) using AVX2 with automatic SWAR fallback.
func HasEscapeOrControlAVX2(data []byte) bool {
	return IndexEscapeOrControlAVX2(data) >= 0
}

// IndexEscapeOrControlAVX2 returns the byte index of the first '\\' or < 0x20 character
// using AVX2 with automatic SWAR fallback for tails and systems without AVX2+BMI2.
func IndexEscapeOrControlAVX2(data []byte) int {
	if !HasAVX2() || !HasBMI2() {
		return IndexEscapeOrControlSWAR(data)
	}
	idx, processed := indexEscapeOrControlAVX2Asm(data)
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

// HasSpecialOrControlAVX2 reports whether data contains a special/control character
// or delim using AVX2 with automatic SWAR fallback.
func HasSpecialOrControlAVX2(data []byte, delim byte) bool {
	return IndexSpecialOrControlAVX2(data, delim) >= 0
}

// IndexSpecialOrControlAVX2 returns the byte index of the first special, control,
// or delim byte using AVX2 with automatic SWAR fallback.
func IndexSpecialOrControlAVX2(data []byte, delim byte) int {
	if !HasAVX2() || !HasBMI2() {
		return IndexSpecialOrControlSWAR(data, delim)
	}
	idx, processed := indexSpecialOrControlAVX2Asm(data, delim)
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

// NeedsQuotingAVX2 reports whether data contains characters that require quoting in TOON using AVX2.
func NeedsQuotingAVX2(data []byte, delim byte) bool {
	return HasSpecialOrControlAVX2(data, delim)
}

// HasEscapeOrControlNEON falls back to SWAR on AMD64 architectures.
func HasEscapeOrControlNEON(data []byte) bool {
	return HasEscapeOrControlSWAR(data)
}

// IndexEscapeOrControlNEON falls back to SWAR on AMD64 architectures.
func IndexEscapeOrControlNEON(data []byte) int {
	return IndexEscapeOrControlSWAR(data)
}

// HasSpecialOrControlNEON falls back to SWAR on AMD64 architectures.
func HasSpecialOrControlNEON(data []byte, delim byte) bool {
	return HasSpecialOrControlSWAR(data, delim)
}

// IndexSpecialOrControlNEON falls back to SWAR on AMD64 architectures.
func IndexSpecialOrControlNEON(data []byte, delim byte) int {
	return IndexSpecialOrControlSWAR(data, delim)
}

// NeedsQuotingNEON falls back to SWAR on AMD64 architectures.
func NeedsQuotingNEON(data []byte, delim byte) bool {
	return HasSpecialOrControlSWAR(data, delim)
}
