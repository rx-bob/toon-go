//go:build amd64 && !purego

package simd

func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)
func xgetbv(ecxArg uint32) (eax, edx uint32)

func detectFeatures() CPUFeatures {
	var f CPUFeatures
	maxID, _, _, _ := cpuid(0, 0)
	if maxID < 7 {
		return f
	}

	_, _, ecx1, _ := cpuid(1, 0)
	// Check OSXSAVE (bit 27)
	hasOSXSAVE := (ecx1 & (1 << 27)) != 0
	if !hasOSXSAVE {
		return f
	}

	// Verify XMM (bit 1) and YMM (bit 2) are enabled by OS
	xcr0, _ := xgetbv(0)
	if (xcr0 & 0x6) != 0x6 {
		return f
	}

	_, ebx7, _, _ := cpuid(7, 0)
	// AVX2 is bit 5 of EBX
	f.HasAVX2 = (ebx7 & (1 << 5)) != 0
	// BMI2 is bit 8 of EBX
	f.HasBMI2 = (ebx7 & (1 << 8)) != 0

	return f
}
