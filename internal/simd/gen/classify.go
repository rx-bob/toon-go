//go:build ignore

package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
)

func main() {
	Package("github.com/toon-format/toon-go/internal/simd")
	ConstraintExpr("amd64,!purego")

	// -------------------------------------------------------------------------
	// indexEscapeOrControlAVX2Asm
	// Scans data in 32-byte chunks using AVX2 and BMI2 to find the first escape
	// ('\') or control character (< 0x20).
	// -------------------------------------------------------------------------
	TEXT("indexEscapeOrControlAVX2Asm", NOSPLIT, "func(data []byte) (index int, processed int)")
	Doc("indexEscapeOrControlAVX2Asm scans data in 32-byte chunks using AVX2 to find the first escape ('\\\\') or control character (< 0x20).")

	dataPtr := Load(Param("data").Base(), GP64())
	dataLen := Load(Param("data").Len(), GP64())

	biasGP := GP32()
	MOVL(U32(0x80), biasGP)
	biasX := XMM()
	MOVD(biasGP, biasX)
	biasY := YMM()
	VPBROADCASTB(biasX, biasY)

	limitGP := GP32()
	MOVL(U32(0xA0), limitGP)
	limitX := XMM()
	MOVD(limitGP, limitX)
	limitY := YMM()
	VPBROADCASTB(limitX, limitY)

	slashGP := GP32()
	MOVL(U32(0x5C), slashGP)
	slashX := XMM()
	MOVD(slashGP, slashX)
	slashY := YMM()
	VPBROADCASTB(slashX, slashY)

	offset := GP64()
	XORQ(offset, offset)

	maxOffset := GP64()
	MOVQ(dataLen, maxOffset)
	SUBQ(U32(32), maxOffset)

	Label("escape_loop_start")
	CMPQ(offset, maxOffset)
	JA(LabelRef("escape_done"))

	chunk := YMM()
	VMOVDQU(Mem{Base: dataPtr, Index: offset, Scale: 1}, chunk)

	biasedChunk := YMM()
	VPXOR(biasY, chunk, biasedChunk)

	ctrlMask := YMM()
	VPCMPGTB(biasedChunk, limitY, ctrlMask)

	slashMask := YMM()
	VPCMPEQB(slashY, chunk, slashMask)

	match := YMM()
	VPOR(ctrlMask, slashMask, match)

	VPTEST(match, match)
	JNZ(LabelRef("escape_found"))

	ADDQ(U32(32), offset)
	JMP(LabelRef("escape_loop_start"))

	Label("escape_found")
	maskGP := GP32()
	VPMOVMSKB(match, maskGP)
	tzGP := GP32()
	TZCNTL(maskGP, tzGP)
	matchIdx := GP64()
	MOVQ(offset, matchIdx)
	ADDQ(tzGP.As64(), matchIdx)
	Store(matchIdx, ReturnIndex(0))
	ADDQ(U32(32), offset)
	Store(offset, ReturnIndex(1))
	VZEROUPPER()
	RET()

	Label("escape_done")
	noMatch := GP64()
	MOVQ(I64(-1), noMatch)
	Store(noMatch, ReturnIndex(0))
	Store(offset, ReturnIndex(1))
	VZEROUPPER()
	RET()

	// -------------------------------------------------------------------------
	// indexSpecialOrControlAVX2Asm
	// Scans data in 32-byte chunks using AVX2 (VPSHUFB lookup tables & comparisons)
	// to find the first special or control character.
	// -------------------------------------------------------------------------
	TEXT("indexSpecialOrControlAVX2Asm", NOSPLIT, "func(data []byte, delim byte) (index int, processed int)")
	Doc("indexSpecialOrControlAVX2Asm scans data in 32-byte chunks using AVX2 to find the first special or control character.")

	specDataPtr := Load(Param("data").Base(), GP64())
	specDataLen := Load(Param("data").Len(), GP64())
	specDelimByte := Load(Param("delim"), GP8())

	// Load mask0F (0x0F broadcast)
	mask0FGP := GP32()
	MOVL(U32(0x0F), mask0FGP)
	mask0FX := XMM()
	MOVD(mask0FGP, mask0FX)
	mask0FY := YMM()
	VPBROADCASTB(mask0FX, mask0FY)

	// Load LUT_HIGH: 0x1000080004020101 (low 64-bit), 0 (high 64-bit)
	lutHighGP := GP64()
	MOVQ(U64(0x1000080004020101), lutHighGP)
	lutHighY := YMM()
	MOVQ(lutHighGP, lutHighY.AsX())
	VINSERTI128(U8(1), lutHighY.AsX(), lutHighY, lutHighY)

	// Load LUT_LOW: 0x0101010101030101 (low 64-bit), 0x0101190919050101 (high 64-bit)
	lutLowLoGP := GP64()
	MOVQ(U64(0x0101010101030101), lutLowLoGP)
	lutLowHiGP := GP64()
	MOVQ(U64(0x0101190919050101), lutLowHiGP)

	lutLowLoX := XMM()
	MOVQ(lutLowLoGP, lutLowLoX)
	lutLowHiX := XMM()
	MOVQ(lutLowHiGP, lutLowHiX)

	lutLowY := YMM()
	VPUNPCKLQDQ(lutLowHiX, lutLowLoX, lutLowY.AsX())
	VINSERTI128(U8(1), lutLowY.AsX(), lutLowY, lutLowY)

	// Delim broadcast
	delimGP := GP32()
	MOVBQZX(specDelimByte, delimGP.As64())
	delimX := XMM()
	MOVD(delimGP, delimX)
	delimY := YMM()
	VPBROADCASTB(delimX, delimY)

	specOffset := GP64()
	XORQ(specOffset, specOffset)

	specMaxOffset := GP64()
	MOVQ(specDataLen, specMaxOffset)
	SUBQ(U32(32), specMaxOffset)

	CMPB(specDelimByte, U8(0))
	JE(LabelRef("special_loop_nodelim"))

	// Loop with active delimiter
	Label("special_loop_delim")
	CMPQ(specOffset, specMaxOffset)
	JA(LabelRef("special_done"))

	chunkD := YMM()
	VMOVDQU(Mem{Base: specDataPtr, Index: specOffset, Scale: 1}, chunkD)

	loD := YMM()
	VPAND(mask0FY, chunkD, loD)

	hiD := YMM()
	VPSRLW(U8(4), chunkD, hiD)
	VPAND(mask0FY, hiD, hiD)

	matchLoD := YMM()
	VPSHUFB(loD, lutLowY, matchLoD)

	matchHiD := YMM()
	VPSHUFB(hiD, lutHighY, matchHiD)

	matchD := YMM()
	VPAND(matchLoD, matchHiD, matchD)

	delimMatch := YMM()
	VPCMPEQB(delimY, chunkD, delimMatch)
	VPOR(delimMatch, matchD, matchD)

	VPTEST(matchD, matchD)
	JNZ(LabelRef("special_found_delim"))

	ADDQ(U32(32), specOffset)
	JMP(LabelRef("special_loop_delim"))

	Label("special_found_delim")
	zeroYD := YMM()
	VPXOR(zeroYD, zeroYD, zeroYD)
	cmpYD := YMM()
	VPCMPEQB(zeroYD, matchD, cmpYD)
	maskGPD := GP32()
	VPMOVMSKB(cmpYD, maskGPD)
	NOTL(maskGPD)
	tzGPD := GP32()
	TZCNTL(maskGPD, tzGPD)
	matchIdxD := GP64()
	MOVQ(specOffset, matchIdxD)
	ADDQ(tzGPD.As64(), matchIdxD)
	Store(matchIdxD, ReturnIndex(0))
	ADDQ(U32(32), specOffset)
	Store(specOffset, ReturnIndex(1))
	VZEROUPPER()
	RET()

	// Loop without active delimiter
	Label("special_loop_nodelim")
	CMPQ(specOffset, specMaxOffset)
	JA(LabelRef("special_done"))

	chunkNoD := YMM()
	VMOVDQU(Mem{Base: specDataPtr, Index: specOffset, Scale: 1}, chunkNoD)

	loNoD := YMM()
	VPAND(mask0FY, chunkNoD, loNoD)

	hiNoD := YMM()
	VPSRLW(U8(4), chunkNoD, hiNoD)
	VPAND(mask0FY, hiNoD, hiNoD)

	matchLoNoD := YMM()
	VPSHUFB(loNoD, lutLowY, matchLoNoD)

	matchHiNoD := YMM()
	VPSHUFB(hiNoD, lutHighY, matchHiNoD)

	matchNoD := YMM()
	VPAND(matchLoNoD, matchHiNoD, matchNoD)

	VPTEST(matchNoD, matchNoD)
	JNZ(LabelRef("special_found_nodelim"))

	ADDQ(U32(32), specOffset)
	JMP(LabelRef("special_loop_nodelim"))

	Label("special_found_nodelim")
	zeroYNoD := YMM()
	VPXOR(zeroYNoD, zeroYNoD, zeroYNoD)
	cmpYNoD := YMM()
	VPCMPEQB(zeroYNoD, matchNoD, cmpYNoD)
	maskGPNoD := GP32()
	VPMOVMSKB(cmpYNoD, maskGPNoD)
	NOTL(maskGPNoD)
	tzGPNoD := GP32()
	TZCNTL(maskGPNoD, tzGPNoD)
	matchIdxNoD := GP64()
	MOVQ(specOffset, matchIdxNoD)
	ADDQ(tzGPNoD.As64(), matchIdxNoD)
	Store(matchIdxNoD, ReturnIndex(0))
	ADDQ(U32(32), specOffset)
	Store(specOffset, ReturnIndex(1))
	VZEROUPPER()
	RET()

	Label("special_done")
	noMatchSpec := GP64()
	MOVQ(I64(-1), noMatchSpec)
	Store(noMatchSpec, ReturnIndex(0))
	Store(specOffset, ReturnIndex(1))
	VZEROUPPER()
	RET()

	Generate()
}
