package simd

import (
	"encoding/binary"
	"math/bits"
	"runtime"
)

// Algorithm specifies which delimiter scanning implementation to execute.
type Algorithm int

const (
	// AlgoScalar selects the byte-by-byte scalar scanning loop.
	AlgoScalar Algorithm = iota
	// AlgoSWAR selects the 64-bit pure Go SWAR scanning routine.
	AlgoSWAR
	// AlgoAVX2 selects the 256-bit AVX2 vector routine.
	AlgoAVX2
	// AlgoNEON selects the 128-bit ARM64 NEON vector routine.
	AlgoNEON
)

func (a Algorithm) String() string {
	switch a {
	case AlgoScalar:
		return "Scalar"
	case AlgoSWAR:
		return "SWAR"
	case AlgoAVX2:
		return "AVX2"
	case AlgoNEON:
		return "NEON"
	default:
		return "Unknown"
	}
}

// HasAVX2 reports whether AVX2 instructions are supported by the current CPU.
func HasAVX2() bool {
	return runtime.GOARCH == "amd64"
}

// HasNEON reports whether ARM64 NEON vector instructions are supported by the current CPU.
func HasNEON() bool {
	return runtime.GOARCH == "arm64"
}

// ScanDelim dispatches delimiter scanning to the requested algorithm.
func ScanDelim(data []byte, delim byte, algo Algorithm) int {
	switch algo {
	case AlgoScalar:
		return ScanDelimScalar(data, delim)
	case AlgoSWAR:
		return ScanDelimSWAR(data, delim)
	case AlgoAVX2:
		return ScanDelimAVX2(data, delim)
	case AlgoNEON:
		return ScanDelimNEON(data, delim)
	default:
		return ScanDelimScalar(data, delim)
	}
}

// ScanDelimScalar counts unquoted occurrences of delim using a scalar loop.
func ScanDelimScalar(data []byte, delim byte) int {
	count := 0
	inQuotes := false
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b == '\\' && inQuotes {
			i++
			continue
		}
		if b == '"' {
			inQuotes = !inQuotes
			continue
		}
		if b == delim && !inQuotes {
			count++
		}
	}
	return count
}

func hasByte64(w uint64, b byte) uint64 {
	mask := uint64(b) * 0x0101010101010101
	v := w ^ mask
	return (v - 0x0101010101010101) & ^v & 0x8080808080808080
}

// ScanDelimSWAR counts unquoted occurrences of delim using 64-bit SWAR word scanning.
func ScanDelimSWAR(data []byte, delim byte) int {
	count := 0
	inQuotes := false
	i := 0
	n := len(data)

	for i+8 <= n {
		if inQuotes {
			b := data[i]
			if b == '\\' {
				i += 2
				continue
			}
			if b == '"' {
				inQuotes = false
			}
			i++
			continue
		}

		w := binary.LittleEndian.Uint64(data[i:])
		hasQuote := hasByte64(w, '"')
		hasBackslash := hasByte64(w, '\\')

		if (hasQuote | hasBackslash) == 0 {
			delims := hasByte64(w, delim)
			if delims != 0 {
				count += bits.OnesCount64(delims)
			}
			i += 8
			continue
		}

		// Process scalar within word boundary when quotes/escapes are present.
		limit := i + 8
		for i < limit {
			b := data[i]
			if b == '\\' && inQuotes {
				i += 2
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

	// Remainder tail
	for i < n {
		b := data[i]
		if b == '\\' && inQuotes {
			i += 2
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

	return count
}

// ScanDelimAVX2 counts unquoted occurrences of delim using a 32-byte vector stride.
// On non-amd64 systems, it falls back to SWAR.
func ScanDelimAVX2(data []byte, delim byte) int {
	if !HasAVX2() {
		return ScanDelimSWAR(data, delim)
	}

	count := 0
	inQuotes := false
	i := 0
	n := len(data)

	// 32-byte vector stride pass
	for i+32 <= n && !inQuotes {
		w0 := binary.LittleEndian.Uint64(data[i:])
		w1 := binary.LittleEndian.Uint64(data[i+8:])
		w2 := binary.LittleEndian.Uint64(data[i+16:])
		w3 := binary.LittleEndian.Uint64(data[i+24:])

		quotes := hasByte64(w0, '"') | hasByte64(w1, '"') | hasByte64(w2, '"') | hasByte64(w3, '"')
		slashes := hasByte64(w0, '\\') | hasByte64(w1, '\\') | hasByte64(w2, '\\') | hasByte64(w3, '\\')

		if (quotes | slashes) == 0 {
			delims := hasByte64(w0, delim) | hasByte64(w1, delim) | hasByte64(w2, delim) | hasByte64(w3, delim)
			if delims != 0 {
				count += bits.OnesCount64(hasByte64(w0, delim)) +
					bits.OnesCount64(hasByte64(w1, delim)) +
					bits.OnesCount64(hasByte64(w2, delim)) +
					bits.OnesCount64(hasByte64(w3, delim))
			}
			i += 32
			continue
		}
		break
	}

	// Remainder processed with SWAR
	if i < n {
		count += ScanDelimSWAR(data[i:], delim)
	}

	return count
}

// ScanDelimNEON counts unquoted occurrences of delim using a 16-byte vector stride.
// On non-arm64 systems, it falls back to SWAR.
func ScanDelimNEON(data []byte, delim byte) int {
	if !HasNEON() {
		return ScanDelimSWAR(data, delim)
	}

	count := 0
	inQuotes := false
	i := 0
	n := len(data)

	// 16-byte vector stride pass
	for i+16 <= n && !inQuotes {
		w0 := binary.LittleEndian.Uint64(data[i:])
		w1 := binary.LittleEndian.Uint64(data[i+8:])

		quotes := hasByte64(w0, '"') | hasByte64(w1, '"')
		slashes := hasByte64(w0, '\\') | hasByte64(w1, '\\')

		if (quotes | slashes) == 0 {
			delims := hasByte64(w0, delim) | hasByte64(w1, delim)
			if delims != 0 {
				count += bits.OnesCount64(hasByte64(w0, delim)) + bits.OnesCount64(hasByte64(w1, delim))
			}
			i += 16
			continue
		}
		break
	}

	// Remainder processed with SWAR
	if i < n {
		count += ScanDelimSWAR(data[i:], delim)
	}

	return count
}
