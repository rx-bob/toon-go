//go:build (!amd64 && !arm64) || purego

package simd

func ScanLinesAVX2(data []byte, dst []int) []int { return ScanLinesSWAR(data, dst) }
func LeadingSpacesAVX2(data []byte) int          { return LeadingSpacesSWAR(data) }
func ComputeIndentAVX2(data []byte) int          { return ComputeIndentSWAR(data) }
func ScanLinesNEON(data []byte, dst []int) []int { return ScanLinesSWAR(data, dst) }
func LeadingSpacesNEON(data []byte) int          { return LeadingSpacesSWAR(data) }
func ComputeIndentNEON(data []byte) int          { return ComputeIndentSWAR(data) }
