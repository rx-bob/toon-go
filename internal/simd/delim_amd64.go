//go:build amd64 && !purego

package simd

//go:generate go run ./gen/delim_quote.go -out asm_amd64.s

func findDelimsAVX2Asm(data []byte, delim byte, dst []int, inQuotesIn bool) (n int, inQuotesOut bool, processed int)
func countDelimsAVX2Asm(data []byte, delim byte, inQuotesIn bool) (count int, inQuotesOut bool, processed int)

// FindDelimsAVX2 finds unquoted delimiter indices using 32-byte AVX2 vector strides with automatic SWAR fallback.
func FindDelimsAVX2(data []byte, delim byte, dst []int) ([]int, bool) {
	if !HasAVX2() || !HasBMI2() {
		return FindDelimsSWAR(data, delim, dst)
	}

	n := len(data)
	inQuotes := false
	processed := 0

	for processed+32 <= n {
		if cap(dst)-len(dst) < 32 {
			grow := make([]int, len(dst), len(dst)*2+32)
			copy(grow, dst)
			dst = grow
		}

		newLen, inQuotesOut, bytesConsumed := findDelimsAVX2Asm(data[processed:], delim, dst, inQuotes)
		dst = dst[:newLen]
		inQuotes = inQuotesOut
		processed += bytesConsumed

		// If AVX2 stopped early (e.g. backslash encountered or buffer full), process with SWAR
		if bytesConsumed < 32 && processed < n {
			remData := data[processed:]
			var swarDst []int
			swarDst, inQuotes, _ = FindDelimsSWARWithState(remData, delim, nil, inQuotes, false)
			for _, idx := range swarDst {
				dst = append(dst, processed+idx)
			}
			return dst, inQuotes
		}
	}

	if processed < n {
		var remIndices []int
		remIndices, inQuotes, _ = FindDelimsSWARWithState(data[processed:], delim, nil, inQuotes, false)
		for _, idx := range remIndices {
			dst = append(dst, processed+idx)
		}
		return dst, inQuotes
	}

	return dst, inQuotes
}

// CountDelimsAVX2 counts unquoted occurrences of delim using AVX2 with automatic SWAR fallback.
func CountDelimsAVX2(data []byte, delim byte) int {
	if !HasAVX2() || !HasBMI2() {
		return CountDelimsSWAR(data, delim)
	}

	count, inQuotes, processed := countDelimsAVX2Asm(data, delim, false)
	if processed < len(data) {
		remCount, _, _ := CountDelimsSWARWithState(data[processed:], delim, inQuotes, false)
		count += remCount
	}
	return count
}

// FindDelimsNEON falls back to SWAR on amd64.
func FindDelimsNEON(data []byte, delim byte, dst []int) ([]int, bool) {
	return FindDelimsSWAR(data, delim, dst)
}

// CountDelimsNEON falls back to SWAR on amd64.
func CountDelimsNEON(data []byte, delim byte) int {
	return CountDelimsSWAR(data, delim)
}
