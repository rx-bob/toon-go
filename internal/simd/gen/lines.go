//go:build ignore

package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
)

func main() {
	Package("github.com/toon-format/toon-go/internal/simd")
	ConstraintExpr("amd64,!purego")

	// lineMaskAVX2Asm compares one 32-byte stride against LF and CR, returning
	// one bit per matching byte. The Go wrapper extracts logical boundaries.
	TEXT("lineMaskAVX2Asm", NOSPLIT, "func(data []byte) (mask uint32, processed int)")
	Doc("lineMaskAVX2Asm returns the LF/CR byte mask for one 32-byte AVX2 stride.")

	dataPtr := Load(Param("data").Base(), GP64())
	dataLen := Load(Param("data").Len(), GP64())
	CMPQ(dataLen, U32(32))
	JB(LabelRef("line_mask_done"))

	lfGP := GP32()
	MOVL(U32(0x0A), lfGP)
	lfX := XMM()
	MOVD(lfGP, lfX)
	lfY := YMM()
	VPBROADCASTB(lfX, lfY)

	crGP := GP32()
	MOVL(U32(0x0D), crGP)
	crX := XMM()
	MOVD(crGP, crX)
	crY := YMM()
	VPBROADCASTB(crX, crY)

	chunk := YMM()
	VMOVDQU(Mem{Base: dataPtr}, chunk)
	lfMask := YMM()
	VPCMPEQB(lfY, chunk, lfMask)
	crMask := YMM()
	VPCMPEQB(crY, chunk, crMask)
	VPOR(crMask, lfMask, lfMask)

	mask := GP32()
	VPMOVMSKB(lfMask, mask)
	Store(mask, ReturnIndex(0))
	processed := GP64()
	MOVQ(U32(32), processed)
	Store(processed, ReturnIndex(1))
	VZEROUPPER()
	RET()

	Label("line_mask_done")
	zero := GP32()
	XORL(zero, zero)
	Store(zero, ReturnIndex(0))
	zero64 := GP64()
	XORQ(zero64, zero64)
	Store(zero64, ReturnIndex(1))
	VZEROUPPER()
	RET()

	// leadingSpacesAVX2Asm consumes complete 32-byte strides. It stops at the
	// first non-space byte and returns both its position and bytes inspected.
	TEXT("leadingSpacesAVX2Asm", NOSPLIT, "func(data []byte) (count int, processed int)")
	Doc("leadingSpacesAVX2Asm counts leading spaces using 32-byte AVX2 strides.")

	spacePtr := Load(Param("data").Base(), GP64())
	spaceLen := Load(Param("data").Len(), GP64())
	CMPQ(spaceLen, U32(32))
	JB(LabelRef("spaces_done"))

	spaceGP := GP32()
	MOVL(U32(0x20), spaceGP)
	spaceX := XMM()
	MOVD(spaceGP, spaceX)
	spaceY := YMM()
	VPBROADCASTB(spaceX, spaceY)

	offset := GP64()
	XORQ(offset, offset)
	limit := GP64()
	MOVQ(spaceLen, limit)
	SUBQ(U32(32), limit)

	Label("spaces_loop")
	CMPQ(offset, limit)
	JA(LabelRef("spaces_done"))
	spaceChunk := YMM()
	VMOVDQU(Mem{Base: spacePtr, Index: offset, Scale: 1}, spaceChunk)
	spaceMask := YMM()
	VPCMPEQB(spaceY, spaceChunk, spaceMask)
	match := GP32()
	VPMOVMSKB(spaceMask, match)
	NOTL(match)
	TESTL(match, match)
	JNZ(LabelRef("spaces_found"))
	ADDQ(U32(32), offset)
	JMP(LabelRef("spaces_loop"))

	Label("spaces_found")
	tz := GP32()
	TZCNTL(match, tz)
	count := GP64()
	MOVQ(offset, count)
	ADDQ(tz.As64(), count)
	Store(count, ReturnIndex(0))
	ADDQ(U32(32), offset)
	Store(offset, ReturnIndex(1))
	VZEROUPPER()
	RET()

	Label("spaces_done")
	Store(offset, ReturnIndex(0))
	Store(offset, ReturnIndex(1))
	VZEROUPPER()
	RET()

	Generate()
}
