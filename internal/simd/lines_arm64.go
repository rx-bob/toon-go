//go:build arm64 && !purego

package simd

import "math/bits"

func lineMaskNEONAsm(data []byte) (mask uint32, processed int)
func leadingSpacesNEONAsm(data []byte) (count int, processed int)

// ScanLinesNEON appends logical CR/LF line-break offsets using NEON 16-byte
// strides, with SWAR used for tails and hosts without ASIMD.
func ScanLinesNEON(data []byte, dst []int) []int {
	if !HasNEON() {
		return ScanLinesSWAR(data, dst)
	}

	offset := 0
	for offset+16 <= len(data) {
		mask, processed := lineMaskNEONAsm(data[offset:])
		if processed != 16 {
			break
		}
		for mask != 0 {
			idx := offset + bits.TrailingZeros32(mask)
			if data[idx] != '\n' || idx == 0 || data[idx-1] != '\r' {
				dst = append(dst, idx)
			}
			mask &= mask - 1
		}
		offset += processed
	}
	if offset == len(data) {
		return dst
	}
	start := len(dst)
	dst = ScanLinesSWAR(data[offset:], dst)
	if offset > 0 && data[offset-1] == '\r' && data[offset] == '\n' && len(dst) > start && dst[start] == 0 {
		copy(dst[start:], dst[start+1:])
		dst = dst[:len(dst)-1]
	}
	for i := start; i < len(dst); i++ {
		dst[i] += offset
	}
	return dst
}

// LeadingSpacesNEON returns number of initial ASCII spaces using NEON 16-byte
// strides, with SWAR used for tails and hosts without ASIMD.
func LeadingSpacesNEON(data []byte) int {
	if !HasNEON() {
		return LeadingSpacesSWAR(data)
	}
	count, processed := leadingSpacesNEONAsm(data)
	if count != processed {
		return count
	}
	return count + LeadingSpacesSWAR(data[processed:])
}

// ComputeIndentNEON returns the initial ASCII-space indentation width.
func ComputeIndentNEON(data []byte) int { return LeadingSpacesNEON(data) }

func ScanLinesAVX2(data []byte, dst []int) []int { return ScanLinesSWAR(data, dst) }
func LeadingSpacesAVX2(data []byte) int          { return LeadingSpacesSWAR(data) }
func ComputeIndentAVX2(data []byte) int          { return ComputeIndentSWAR(data) }
