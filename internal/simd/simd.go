package simd

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

// DelimScanner abstracts delimiter scanning implementations.
type DelimScanner interface {
	ScanDelim(data []byte, delim byte) int
}

// SelectBestAlgorithm selects the optimal scanning algorithm based on runtime CPU capabilities.
func SelectBestAlgorithm() Algorithm {
	if HasAVX2() && HasBMI2() {
		return AlgoAVX2
	}
	if HasNEON() {
		return AlgoNEON
	}
	return AlgoSWAR
}

// ScanDelimAuto scans data for delim using the optimal algorithm for the host CPU.
func ScanDelimAuto(data []byte, delim byte) int {
	return ScanDelim(data, delim, SelectBestAlgorithm())
}

// FindDelimsAuto finds all unquoted occurrences of delim in data using the optimal algorithm for the host CPU.
func FindDelimsAuto(data []byte, delim byte, dst []int) ([]int, bool) {
	if HasAVX2() && HasBMI2() {
		return FindDelimsAVX2(data, delim, dst)
	}
	if HasNEON() {
		return FindDelimsNEON(data, delim, dst)
	}
	return FindDelimsSWAR(data, delim, dst)
}

// IndexUnquotedAuto returns the index of the first unquoted occurrence of delim in data.
func IndexUnquotedAuto(data []byte, delim byte) int {
	return IndexUnquotedSWAR(data, delim)
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
	return ^(((v & 0x7f7f7f7f7f7f7f7f) + 0x7f7f7f7f7f7f7f7f) | v) & 0x8080808080808080
}

// ScanDelimSWAR counts unquoted occurrences of delim using 64-bit SWAR word scanning.
func ScanDelimSWAR(data []byte, delim byte) int {
	return CountDelimsSWAR(data, delim)
}

// ScanDelimAVX2 counts unquoted occurrences of delim using a 32-byte vector stride.
// On non-amd64 systems, it falls back to SWAR.
// ScanDelimAVX2 counts unquoted occurrences of delim using AVX2.
// On non-amd64 systems or when AVX2 is unavailable, it falls back to SWAR.
func ScanDelimAVX2(data []byte, delim byte) int {
	return CountDelimsAVX2(data, delim)
}

// ScanDelimNEON counts unquoted occurrences of delim using a 16-byte vector stride.
// On non-arm64 systems, it falls back to SWAR.
// ScanDelimNEON counts unquoted occurrences of delim using NEON.
// On non-arm64 systems or when NEON is unavailable, it falls back to SWAR.
func ScanDelimNEON(data []byte, delim byte) int {
	return CountDelimsNEON(data, delim)
}
