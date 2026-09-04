//go:build amd64 && !purego

package simd

import "math/bits"

//go:generate go run ./gen/lines.go -out asm_lines_amd64.s

func lineMaskAVX2Asm(data []byte) (mask uint32, processed int)
func leadingSpacesAVX2Asm(data []byte) (count int, processed int)

// ScanLinesAVX2 appends logical CR/LF line-break offsets using AVX2 32-byte
// strides, with SWAR used for tails and CPUs without AVX2 and BMI2.
func ScanLinesAVX2(data []byte, dst []int) []int {
	if !HasAVX2() || !HasBMI2() {
		return ScanLinesSWAR(data, dst)
	}

	offset := 0
	for offset+32 <= len(data) {
		mask, processed := lineMaskAVX2Asm(data[offset:])
		if processed != 32 {
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
	// ScanLinesSWAR starts a fresh buffer and therefore cannot see a CR in the
	// final AVX2 stride immediately before this tail's first LF.
	if offset > 0 && data[offset-1] == '\r' && data[offset] == '\n' && len(dst) > start && dst[start] == 0 {
		copy(dst[start:], dst[start+1:])
		dst = dst[:len(dst)-1]
	}
	for i := start; i < len(dst); i++ {
		dst[i] += offset
	}
	return dst
}

// LeadingSpacesAVX2 returns number of initial ASCII spaces using AVX2 32-byte
// strides, with SWAR used for tails and CPUs without AVX2 and BMI2.
func LeadingSpacesAVX2(data []byte) int {
	if !HasAVX2() || !HasBMI2() {
		return LeadingSpacesSWAR(data)
	}
	count, processed := leadingSpacesAVX2Asm(data)
	if count != processed {
		return count
	}
	return count + LeadingSpacesSWAR(data[processed:])
}

// ComputeIndentAVX2 returns the initial ASCII-space indentation width.
func ComputeIndentAVX2(data []byte) int {
	return LeadingSpacesAVX2(data)
}

func ScanLinesNEON(data []byte, dst []int) []int { return ScanLinesSWAR(data, dst) }
func LeadingSpacesNEON(data []byte) int          { return LeadingSpacesSWAR(data) }
func ComputeIndentNEON(data []byte) int          { return ComputeIndentSWAR(data) }
