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

const (
	maskSlash  = uint64('\\') * 0x0101010101010101
	maskColon  = uint64(':') * 0x0101010101010101
	maskQuote  = uint64('"') * 0x0101010101010101
	maskLBrack = uint64('[') * 0x0101010101010101
	maskLBrace = uint64('{') * 0x0101010101010101
	maskRBrace = uint64('}') * 0x0101010101010101
)

// hasControl64 returns a bitmask with bit 7 set for each byte in w that is < 0x20.
// For any byte b, b < 0x20 iff bits 5, 6, 7 are all 0.
func hasControl64(w uint64) uint64 {
	t := ((w & 0x6060606060606060) + 0x7f7f7f7f7f7f7f7f) | w
	return ^t & 0x8080808080808080
}

// hasEscapeOrControl64 returns a bitmask with bit 7 set for each byte in w
// that is either a control character (< 0x20) or a backslash ('\\').
func hasEscapeOrControl64(w uint64) uint64 {
	return hasControl64(w) | hasByte64Mask(w, maskSlash)
}

// hasSpecialOrControl64 returns a bitmask with bit 7 set for each byte in w
// that is a control character (< 0x20), a TOON structural character
// (':', '\\', '"', '[', ']', '{', '}'), or matches delim (if delim != 0).
func hasSpecialOrControl64(w uint64, delim byte) uint64 {
	m := hasControl64(w)
	m |= hasByte64Mask(w, maskColon)
	m |= hasByte64Mask(w, maskQuote)
	m |= hasByte64Mask(w, maskLBrack)
	m |= hasByte64Mask(w&0xfefefefefefefefe, maskSlash) // matches '\\' (0x5C) and ']' (0x5D)
	m |= hasByte64Mask(w, maskLBrace)
	m |= hasByte64Mask(w, maskRBrace)
	if delim != 0 {
		m |= hasByte64(w, delim)
	}
	return m
}

// HasEscapeOrControlSWAR reports whether data contains any escape character ('\\')
// or control character (< 0x20) using 64-bit SWAR word scanning.
func HasEscapeOrControlSWAR(data []byte) bool {
	n := len(data)
	i := 0
	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		if hasEscapeOrControl64(w) != 0 {
			return true
		}
		i += 8
	}
	for i < n {
		b := data[i]
		if b < 0x20 || b == '\\' {
			return true
		}
		i++
	}
	return false
}

// IndexEscapeOrControlSWAR returns the index of the first byte in data that is
// an escape character ('\\') or a control character (< 0x20), or -1 if none found.
func IndexEscapeOrControlSWAR(data []byte) int {
	n := len(data)
	i := 0
	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		m := hasEscapeOrControl64(w)
		if m != 0 {
			tz := bits.TrailingZeros64(m)
			return i + (tz >> 3)
		}
		i += 8
	}
	for i < n {
		b := data[i]
		if b < 0x20 || b == '\\' {
			return i
		}
		i++
	}
	return -1
}

// HasSpecialOrControlSWAR reports whether data contains any control character (< 0x20),
// TOON structural character (':', '\\', '"', '[', ']', '{', '}'), or delim (if delim != 0)
// using 64-bit SWAR word scanning.
func HasSpecialOrControlSWAR(data []byte, delim byte) bool {
	n := len(data)
	i := 0
	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		if hasSpecialOrControl64(w, delim) != 0 {
			return true
		}
		i += 8
	}
	for i < n {
		b := data[i]
		if b < 0x20 {
			return true
		}
		switch b {
		case ':', '\\', '"', '[', ']', '{', '}':
			return true
		}
		if delim != 0 && b == delim {
			return true
		}
		i++
	}
	return false
}

// IndexSpecialOrControlSWAR returns the index of the first special, control, or delim byte,
// or -1 if none found.
func IndexSpecialOrControlSWAR(data []byte, delim byte) int {
	n := len(data)
	i := 0
	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		m := hasSpecialOrControl64(w, delim)
		if m != 0 {
			tz := bits.TrailingZeros64(m)
			return i + (tz >> 3)
		}
		i += 8
	}
	for i < n {
		b := data[i]
		if b < 0x20 {
			return i
		}
		switch b {
		case ':', '\\', '"', '[', ']', '{', '}':
			return i
		}
		if delim != 0 && b == delim {
			return i
		}
		i++
	}
	return -1
}

// NeedsQuotingSWAR reports whether data contains characters that require quoting in TOON.
func NeedsQuotingSWAR(data []byte, delim byte) bool {
	return HasSpecialOrControlSWAR(data, delim)
}

const highBits64 = 0x8080808080808080

// ScanLinesSWAR appends offsets of logical line breaks in data to dst. Both
// LF and CR terminate a line; a CRLF pair contributes one offset at its CR.
// The caller may reuse dst to avoid allocations.
func ScanLinesSWAR(data []byte, dst []int) []int {
	n := len(data)
	i := 0
	for i+8 <= n {
		w := binary.LittleEndian.Uint64(data[i:])
		breaks := hasByte64(w, '\n') | hasByte64(w, '\r')
		for breaks != 0 {
			offset := i + (bits.TrailingZeros64(breaks) >> 3)
			if data[offset] != '\n' || offset == 0 || data[offset-1] != '\r' {
				dst = append(dst, offset)
			}
			breaks &= breaks - 1
		}
		i += 8
	}
	for ; i < n; i++ {
		if data[i] != '\n' && data[i] != '\r' {
			continue
		}
		if data[i] != '\n' || i == 0 || data[i-1] != '\r' {
			dst = append(dst, i)
		}
	}
	return dst
}

// LeadingSpacesSWAR returns number of consecutive ASCII space bytes at data's
// beginning using 64-bit word comparisons and a scalar tail.
func LeadingSpacesSWAR(data []byte) int {
	i := 0
	for i+8 <= len(data) {
		w := binary.LittleEndian.Uint64(data[i:])
		spaces := hasByte64(w, ' ')
		if spaces != highBits64 {
			nonSpaces := (^spaces) & highBits64
			return i + (bits.TrailingZeros64(nonSpaces) >> 3)
		}
		i += 8
	}
	for i < len(data) && data[i] == ' ' {
		i++
	}
	return i
}

// ComputeIndentSWAR returns the initial ASCII-space indentation width.
func ComputeIndentSWAR(data []byte) int {
	return LeadingSpacesSWAR(data)
}
