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
	// findDelimsAVX2Asm
	// Finds unquoted delimiter indices into dst slice using 32-byte AVX2 strides.
	// -------------------------------------------------------------------------
	TEXT("findDelimsAVX2Asm", NOSPLIT, "func(data []byte, delim byte, dst []int, inQuotesIn bool) (n int, inQuotesOut bool, processed int)")
	Doc("findDelimsAVX2Asm scans data in 32-byte strides using AVX2 and BMI2 to extract unquoted delimiter offsets.")

	dataPtr := Load(Param("data").Base(), GP64())
	dataLen := Load(Param("data").Len(), GP64())

	delimByte := Load(Param("delim"), GP8())
	delimGP := GP32()
	MOVBQZX(delimByte, delimGP.As64())
	delimX := XMM()
	MOVD(delimGP, delimX)
	delimY := YMM()
	VPBROADCASTB(delimX, delimY)

	quoteGP := GP32()
	MOVL(U32(0x22), quoteGP)
	quoteX := XMM()
	MOVD(quoteGP, quoteX)
	quoteY := YMM()
	VPBROADCASTB(quoteX, quoteY)

	slashGP := GP32()
	MOVL(U32(0x5C), slashGP)
	slashX := XMM()
	MOVD(slashGP, slashX)
	slashY := YMM()
	VPBROADCASTB(slashX, slashY)

	dstPtr := Load(Param("dst").Base(), GP64())
	dstLen := Load(Param("dst").Len(), GP64())
	dstCap := Load(Param("dst").Cap(), GP64())

	inQuotesIn := Load(Param("inQuotesIn"), GP8())
	carry := GP32()
	MOVBQZX(inQuotesIn, carry.As64())
	NEGL(carry)

	offset := GP64()
	XORQ(offset, offset)

	limit := GP64()
	MOVQ(dataLen, limit)
	SUBQ(U32(32), limit)

	Label("find_loop_start")
	CMPQ(offset, limit)
	JA(LabelRef("find_done"))

	chunk := YMM()
	VMOVDQU(Mem{Base: dataPtr, Index: offset, Scale: 1}, chunk)

	slashMask := YMM()
	VPCMPEQB(slashY, chunk, slashMask)
	slashBits := GP32()
	VPMOVMSKB(slashMask, slashBits)
	TESTL(slashBits, slashBits)
	JNZ(LabelRef("find_done"))

	delimMask := YMM()
	VPCMPEQB(delimY, chunk, delimMask)
	quoteMask := YMM()
	VPCMPEQB(quoteY, chunk, quoteMask)

	quoteBits := GP32()
	VPMOVMSKB(quoteMask, quoteBits)

	q := GP32()
	MOVL(quoteBits, q)
	tmp := GP32()

	MOVL(q, tmp)
	SHLL(U8(1), tmp)
	XORL(tmp, q)

	MOVL(q, tmp)
	SHLL(U8(2), tmp)
	XORL(tmp, q)

	MOVL(q, tmp)
	SHLL(U8(4), tmp)
	XORL(tmp, q)

	MOVL(q, tmp)
	SHLL(U8(8), tmp)
	XORL(tmp, q)

	MOVL(q, tmp)
	SHLL(U8(16), tmp)
	XORL(tmp, q)

	XORL(carry, q)
	MOVL(q, carry)
	SARL(U8(31), carry)

	delimBits := GP32()
	VPMOVMSKB(delimMask, delimBits)

	NOTL(q)
	ANDL(delimBits, q)

	Label("find_extract")
	TESTL(q, q)
	JZ(LabelRef("find_next_stride"))

	CMPQ(dstLen, dstCap)
	JAE(LabelRef("find_done"))

	tz := GP32()
	TZCNTL(q, tz)

	absIdx := GP64()
	LEAQ(Mem{Base: offset, Index: tz.As64(), Scale: 1}, absIdx)
	MOVQ(absIdx, Mem{Base: dstPtr, Index: dstLen, Scale: 8})
	INCQ(dstLen)

	BLSRL(q, q)
	JMP(LabelRef("find_extract"))

	Label("find_next_stride")
	ADDQ(U32(32), offset)
	JMP(LabelRef("find_loop_start"))

	Label("find_done")
	VZEROUPPER()

	Store(dstLen, ReturnIndex(0))

	outInQuotes := GP8()
	TESTL(carry, carry)
	SETNE(outInQuotes)
	Store(outInQuotes, ReturnIndex(1))

	Store(offset, ReturnIndex(2))
	RET()

	// -------------------------------------------------------------------------
	// countDelimsAVX2Asm
	// High-throughput counter using POPCNTL across 32-byte strides.
	// -------------------------------------------------------------------------
	TEXT("countDelimsAVX2Asm", NOSPLIT, "func(data []byte, delim byte, inQuotesIn bool) (count int, inQuotesOut bool, processed int)")
	Doc("countDelimsAVX2Asm counts unquoted delimiters across 32-byte strides using AVX2 and BMI2.")

	dataPtr2 := Load(Param("data").Base(), GP64())
	dataLen2 := Load(Param("data").Len(), GP64())

	delimByte2 := Load(Param("delim"), GP8())
	delimGP2 := GP32()
	MOVBQZX(delimByte2, delimGP2.As64())
	delimX2 := XMM()
	MOVD(delimGP2, delimX2)
	delimY2 := YMM()
	VPBROADCASTB(delimX2, delimY2)

	quoteGP2 := GP32()
	MOVL(U32(0x22), quoteGP2)
	quoteX2 := XMM()
	MOVD(quoteGP2, quoteX2)
	quoteY2 := YMM()
	VPBROADCASTB(quoteX2, quoteY2)

	slashGP2 := GP32()
	MOVL(U32(0x5C), slashGP2)
	slashX2 := XMM()
	MOVD(slashGP2, slashX2)
	slashY2 := YMM()
	VPBROADCASTB(slashX2, slashY2)

	inQuotesIn2 := Load(Param("inQuotesIn"), GP8())
	carry2 := GP32()
	MOVBQZX(inQuotesIn2, carry2.As64())
	NEGL(carry2)

	totalCount := GP64()
	XORQ(totalCount, totalCount)
	offset2 := GP64()
	XORQ(offset2, offset2)

	limit2 := GP64()
	MOVQ(dataLen2, limit2)
	SUBQ(U32(32), limit2)

	Label("count_loop_start")
	CMPQ(offset2, limit2)
	JA(LabelRef("count_done"))

	chunk2 := YMM()
	VMOVDQU(Mem{Base: dataPtr2, Index: offset2, Scale: 1}, chunk2)

	slashMask2 := YMM()
	VPCMPEQB(slashY2, chunk2, slashMask2)
	slashBits2 := GP32()
	VPMOVMSKB(slashMask2, slashBits2)
	TESTL(slashBits2, slashBits2)
	JNZ(LabelRef("count_done"))

	delimMask2 := YMM()
	VPCMPEQB(delimY2, chunk2, delimMask2)
	quoteMask2 := YMM()
	VPCMPEQB(quoteY2, chunk2, quoteMask2)

	quoteBits2 := GP32()
	VPMOVMSKB(quoteMask2, quoteBits2)

	q2 := GP32()
	MOVL(quoteBits2, q2)
	tmp2 := GP32()

	MOVL(q2, tmp2)
	SHLL(U8(1), tmp2)
	XORL(tmp2, q2)

	MOVL(q2, tmp2)
	SHLL(U8(2), tmp2)
	XORL(tmp2, q2)

	MOVL(q2, tmp2)
	SHLL(U8(4), tmp2)
	XORL(tmp2, q2)

	MOVL(q2, tmp2)
	SHLL(U8(8), tmp2)
	XORL(tmp2, q2)

	MOVL(q2, tmp2)
	SHLL(U8(16), tmp2)
	XORL(tmp2, q2)

	XORL(carry2, q2)
	MOVL(q2, carry2)
	SARL(U8(31), carry2)

	delimBits2 := GP32()
	VPMOVMSKB(delimMask2, delimBits2)

	NOTL(q2)
	ANDL(delimBits2, q2)

	popCnt := GP32()
	POPCNTL(q2, popCnt)
	ADDQ(popCnt.As64(), totalCount)

	ADDQ(U32(32), offset2)
	JMP(LabelRef("count_loop_start"))

	Label("count_done")
	VZEROUPPER()

	Store(totalCount, ReturnIndex(0))

	outInQuotes2 := GP8()
	TESTL(carry2, carry2)
	SETNE(outInQuotes2)
	Store(outInQuotes2, ReturnIndex(1))

	Store(offset2, ReturnIndex(2))
	RET()

	Generate()
}
