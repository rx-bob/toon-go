//go:build amd64 && !purego

#include "textflag.h"

// func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuid(SB), NOSPLIT, $0-24
	MOVL eaxArg+0(FP), AX
	MOVL ecxArg+4(FP), CX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET

// func xgetbv(ecxArg uint32) (eax, edx uint32)
TEXT ·xgetbv(SB), NOSPLIT, $0-12
	MOVL ecxArg+0(FP), CX
	XGETBV
	MOVL AX, eax+4(FP)
	MOVL DX, edx+8(FP)
	RET
