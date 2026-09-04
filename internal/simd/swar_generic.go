package simd

import (
	"encoding/binary"
	"math/bits"
)

// IndexUnquotedSWAR returns the index of the first unquoted occurrence of delim in data,
// or -1 if not found. Matches parse.IndexUnquoted semantics.
func IndexUnquotedSWAR(data []byte, delim byte) int {
	n := len(data)
	inQuotes := false
	escaped := false
	i := 0

	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		hasSlash := hasByte64(w, '\\')

		// When no escapes are pending and no backslashes in this block, use parallel prefix-XOR
		if hasSlash == 0 && !escaped {
			hasQuote := hasByte64(w, '"')
			hasDelim := hasByte64(w, delim)

			// Fast prefix-XOR quote carry
			p := hasQuote
			p ^= p << 8
			p ^= p << 16
			p ^= p << 32

			if inQuotes {
				p ^= 0x8080808080808080
			}

			// Update inQuotes state exiting this 8-byte block
			inQuotes = (p & 0x8000000000000000) != 0

			unquoted := hasDelim & ^p
			if unquoted != 0 {
				tz := bits.TrailingZeros64(unquoted)
				return i + (tz >> 3)
			}
			i += 8
			continue
		}

		// When backslash or pending escape is present, process with stateful 8-byte stride
		limit := i + 8
		for i < limit {
			b := data[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if b == '\\' && inQuotes {
				escaped = true
				i++
				continue
			}
			if b == '"' {
				inQuotes = !inQuotes
				i++
				continue
			}
			if b == delim && !inQuotes {
				return i
			}
			i++
		}
	}

	// Tail remainder (< 8 bytes)
	for i < n {
		b := data[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if b == '\\' && inQuotes {
			escaped = true
			i++
			continue
		}
		if b == '"' {
			inQuotes = !inQuotes
			i++
			continue
		}
		if b == delim && !inQuotes {
			return i
		}
		i++
	}

	return -1
}

// FindDelimsSWAR finds all unquoted occurrences of delim in data and appends their indices to dst.
// It returns the resulting slice and a boolean indicating whether an unclosed quote was encountered.
func FindDelimsSWAR(data []byte, delim byte, dst []int) ([]int, bool) {
	indices, inQuotes, _ := FindDelimsSWARWithState(data, delim, dst, false, false)
	return indices, inQuotes
}

// FindDelimsSWARWithState finds all unquoted occurrences of delim in data with given initial inQuotes and escaped states.
func FindDelimsSWARWithState(data []byte, delim byte, dst []int, inQuotesIn bool, escapedIn bool) ([]int, bool, bool) {
	n := len(data)
	inQuotes := inQuotesIn
	escaped := escapedIn
	i := 0

	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		hasSlash := hasByte64(w, '\\')

		if hasSlash == 0 && !escaped {
			hasQuote := hasByte64(w, '"')
			hasDelim := hasByte64(w, delim)

			p := hasQuote
			p ^= p << 8
			p ^= p << 16
			p ^= p << 32

			if inQuotes {
				p ^= 0x8080808080808080
			}

			inQuotes = (p & 0x8000000000000000) != 0

			unquoted := hasDelim & ^p
			for unquoted != 0 {
				tz := bits.TrailingZeros64(unquoted)
				dst = append(dst, i+(tz>>3))
				unquoted &= unquoted - 1
			}
			i += 8
			continue
		}

		limit := i + 8
		for i < limit {
			b := data[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if b == '\\' && inQuotes {
				escaped = true
				i++
				continue
			}
			if b == '"' {
				inQuotes = !inQuotes
				i++
				continue
			}
			if b == delim && !inQuotes {
				dst = append(dst, i)
			}
			i++
		}
	}

	for i < n {
		b := data[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if b == '\\' && inQuotes {
			escaped = true
			i++
			continue
		}
		if b == '"' {
			inQuotes = !inQuotes
			i++
			continue
		}
		if b == delim && !inQuotes {
			dst = append(dst, i)
		}
		i++
	}

	return dst, inQuotes, escaped
}

// CountDelimsSWAR returns the total number of unquoted occurrences of delim in data.
func CountDelimsSWAR(data []byte, delim byte) int {
	count, _, _ := CountDelimsSWARWithState(data, delim, false, false)
	return count
}

// CountDelimsSWARWithState counts all unquoted occurrences of delim in data with given initial inQuotes and escaped states.
func CountDelimsSWARWithState(data []byte, delim byte, inQuotesIn bool, escapedIn bool) (int, bool, bool) {
	n := len(data)
	count := 0
	inQuotes := inQuotesIn
	escaped := escapedIn
	i := 0

	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		hasSlash := hasByte64(w, '\\')

		if hasSlash == 0 && !escaped {
			hasQuote := hasByte64(w, '"')
			hasDelim := hasByte64(w, delim)

			p := hasQuote
			p ^= p << 8
			p ^= p << 16
			p ^= p << 32

			if inQuotes {
				p ^= 0x8080808080808080
			}

			inQuotes = (p & 0x8000000000000000) != 0

			unquoted := hasDelim & ^p
			if unquoted != 0 {
				count += bits.OnesCount64(unquoted)
			}
			i += 8
			continue
		}

		limit := i + 8
		for i < limit {
			b := data[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if b == '\\' && inQuotes {
				escaped = true
				i++
				continue
			}
			if b == '"' {
				inQuotes = !inQuotes
				i++
				continue
			}
			if b == delim && !inQuotes {
				count++
			}
			i++
		}
	}

	for i < n {
		b := data[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if b == '\\' && inQuotes {
			escaped = true
			i++
			continue
		}
		if b == '"' {
			inQuotes = !inQuotes
			i++
			continue
		}
		if b == delim && !inQuotes {
			count++
		}
		i++
	}

	return count, inQuotes, escaped
}
