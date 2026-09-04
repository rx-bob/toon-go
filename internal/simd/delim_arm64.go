//go:build arm64 && !purego

package simd

func findDelimsNEONAsm(data []byte, delim byte, dst []int, inQuotesIn bool, baseOffset int) (n int, inQuotesOut bool, processed int)
func countDelimsNEONAsm(data []byte, delim byte, inQuotesIn bool) (count int, inQuotesOut bool, processed int)

// FindDelimsNEON finds unquoted delimiter indices using 16-byte NEON vector strides with automatic SWAR fallback.
func FindDelimsNEON(data []byte, delim byte, dst []int) ([]int, bool) {
	if !HasNEON() {
		return FindDelimsSWAR(data, delim, dst)
	}

	n := len(data)
	if n < 16 {
		return FindDelimsSWAR(data, delim, dst)
	}

	// Ensure sufficient capacity so assembly kernel never truncates mid-chunk
	if cap(dst)-len(dst) < n {
		grow := make([]int, len(dst), len(dst)+n)
		copy(grow, dst)
		dst = grow
	}

	newLen, inQuotes, processed := findDelimsNEONAsm(data, delim, dst, false, 0)
	dst = dst[:newLen]

	if processed < n {
		var remIndices []int
		remIndices, inQuotes, _ = FindDelimsSWARWithState(data[processed:], delim, nil, inQuotes, false)
		for _, idx := range remIndices {
			dst = append(dst, processed+idx)
		}
	}

	return dst, inQuotes
}

// CountDelimsNEON counts unquoted occurrences of delim using NEON with automatic SWAR fallback.
func CountDelimsNEON(data []byte, delim byte) int {
	if !HasNEON() {
		return CountDelimsSWAR(data, delim)
	}

	count, inQuotes, processed := countDelimsNEONAsm(data, delim, false)
	if processed < len(data) {
		remCount, _, _ := CountDelimsSWARWithState(data[processed:], delim, inQuotes, false)
		count += remCount
	}
	return count
}

// FindDelimsAVX2 falls back to SWAR on arm64.
func FindDelimsAVX2(data []byte, delim byte, dst []int) ([]int, bool) {
	return FindDelimsSWAR(data, delim, dst)
}

// CountDelimsAVX2 falls back to SWAR on arm64.
func CountDelimsAVX2(data []byte, delim byte) int {
	return CountDelimsSWAR(data, delim)
}
