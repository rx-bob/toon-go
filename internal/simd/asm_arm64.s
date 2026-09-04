//go:build arm64 && !purego

#include "textflag.h"

// func findDelimsNEONAsm(data []byte, delim byte, dst []int, inQuotesIn bool, baseOffset int) (n int, inQuotesOut bool, processed int)
TEXT ·findDelimsNEONAsm(SB), NOSPLIT, $0-96
	MOVD    data_base+0(FP), R0
	MOVD    data_len+8(FP), R1

	MOVD    dst_base+32(FP), R5
	MOVD    dst_len+40(FP), R6
	MOVD    dst_cap+48(FP), R7

	MOVBU   inQuotesIn+56(FP), R8
	NEG     R8, R8                      // R8: -1 if inQuotesIn, 0 otherwise

	MOVD    baseOffset+64(FP), R16      // R16: baseOffset
	MOVD    $0, R9                      // R9: offset in data

	// If data_len < 16, exit immediately to SWAR/scalar
	CMP     $16, R1
	BLO     find_done

	MOVBU   delim+24(FP), R2
	VMOV    R2, V0.B16                  // V0: broadcast delim

	MOVD    $0x22, R3
	VMOV    R3, V1.B16                  // V1: broadcast quote '"'

	MOVD    $0x5c, R4
	VMOV    R4, V2.B16                  // V2: broadcast backslash '\\'

	MOVD    $0x0101010101010101, R19    // R19: byte mask
	MOVD    $0x0102040810204080, R20    // R20: condensation magic

	SUB     $16, R1, R10                // R10: limit = len - 16

find_loop_start:
	CMP     R10, R9
	BHI     find_done

	ADD     R0, R9, R11
	VLD1    (R11), [V3.B16]             // Load 16-byte chunk

	// Check backslash
	VCMEQ   V2.B16, V3.B16, V4.B16
	VMOV    V4.D[0], R12
	VMOV    V4.D[1], R13
	ORR     R13, R12, R12
	CBNZ    R12, find_done

	// Compare delims and quotes
	VCMEQ   V0.B16, V3.B16, V6.B16      // V6 = delim matches
	VCMEQ   V1.B16, V3.B16, V7.B16      // V7 = quote matches

	// Condense quote mask to 16 bits in R12:
	// Low 8 bytes
	VMOV    V7.D[0], R12
	LSR     $7, R12, R14
	AND     R19, R14, R14
	MUL     R20, R14, R14
	LSR     $56, R14, R12               // R12 = low 8 bits of quote mask
	// High 8 bytes
	VMOV    V7.D[1], R13
	LSR     $7, R13, R14
	AND     R19, R14, R14
	MUL     R20, R14, R14
	LSR     $56, R14, R14
	ORR     R14<<8, R12, R12            // R12 = full 16-bit quote mask

	// Prefix-XOR quote mask across 16 bits
	EOR     R12<<1, R12, R12
	EOR     R12<<2, R12, R12
	EOR     R12<<4, R12, R12
	EOR     R12<<8, R12, R12
	EOR     R8, R12, R12                // Apply incoming carry

	// Update carry for next stride from bit 15:
	LSL     $48, R12, R8
	ASR     $63, R8, R8                 // R8: -1 if bit 15 was 1, else 0

	// Condense delim mask to 16 bits in R13:
	// Low 8 bytes
	VMOV    V6.D[0], R13
	LSR     $7, R13, R14
	AND     R19, R14, R14
	MUL     R20, R14, R14
	LSR     $56, R14, R13
	// High 8 bytes
	VMOV    V6.D[1], R14
	LSR     $7, R14, R15
	AND     R19, R15, R15
	MUL     R20, R15, R15
	LSR     $56, R15, R15
	ORR     R15<<8, R13, R13            // R13 = full 16-bit delim mask

	// Unquoted delims: delimMask & ~quoteMask
	MVN     R12, R12
	AND     R13, R12, R12
	AND     $0xffff, R12, R12

find_extract:
	CBZ     R12, find_next_stride
	CMP     R7, R6
	BHS     find_done                   // Buffer full

	RBIT    R12, R14
	CLZ     R14, R14                    // R14 = trailing zeros
	ADD     R16, R9, R15
	ADD     R14, R15, R15               // R15 = baseOffset + offset + tz
	MOVD    R15, (R5)(R6<<3)
	ADD     $1, R6, R6

	SUB     $1, R12, R14
	AND     R14, R12, R12               // Clear lowest bit
	B       find_extract

find_next_stride:
	ADD     $16, R9, R9
	B       find_loop_start

find_done:
	MOVD    R6, n+72(FP)
	TST     $1, R8
	CSET    NE, R11
	MOVB    R11, inQuotesOut+80(FP)
	MOVD    R9, processed+88(FP)
	RET

// func countDelimsNEONAsm(data []byte, delim byte, inQuotesIn bool) (count int, inQuotesOut bool, processed int)
TEXT ·countDelimsNEONAsm(SB), NOSPLIT, $0-56
	MOVD    data_base+0(FP), R0
	MOVD    data_len+8(FP), R1

	MOVBU   inQuotesIn+25(FP), R8
	NEG     R8, R8

	MOVD    $0, R6                      // R6: total count
	MOVD    $0, R9                      // R9: offset

	// If data_len < 16, exit immediately
	CMP     $16, R1
	BLO     count_done

	MOVBU   delim+24(FP), R2
	VMOV    R2, V0.B16

	MOVD    $0x22, R3
	VMOV    R3, V1.B16

	MOVD    $0x5c, R4
	VMOV    R4, V2.B16

	MOVD    $0x0101010101010101, R19
	MOVD    $0x0102040810204080, R20

	SUB     $16, R1, R10                // R10: limit

count_loop_start:
	CMP     R10, R9
	BHI     count_done

	ADD     R0, R9, R11
	VLD1    (R11), [V3.B16]

	// Check backslash
	VCMEQ   V2.B16, V3.B16, V4.B16
	VMOV    V4.D[0], R12
	VMOV    V4.D[1], R13
	ORR     R13, R12, R12
	CBNZ    R12, count_done

	// Compare delims and quotes
	VCMEQ   V0.B16, V3.B16, V6.B16
	VCMEQ   V1.B16, V3.B16, V7.B16

	// Condense quote mask
	VMOV    V7.D[0], R12
	LSR     $7, R12, R14
	AND     R19, R14, R14
	MUL     R20, R14, R14
	LSR     $56, R14, R12

	VMOV    V7.D[1], R13
	LSR     $7, R13, R14
	AND     R19, R14, R14
	MUL     R20, R14, R14
	LSR     $56, R14, R14
	ORR     R14<<8, R12, R12

	// Prefix-XOR quote mask
	EOR     R12<<1, R12, R12
	EOR     R12<<2, R12, R12
	EOR     R12<<4, R12, R12
	EOR     R12<<8, R12, R12
	EOR     R8, R12, R12

	LSL     $48, R12, R8
	ASR     $63, R8, R8

	// Condense delim mask
	VMOV    V6.D[0], R13
	LSR     $7, R13, R14
	AND     R19, R14, R14
	MUL     R20, R14, R14
	LSR     $56, R14, R13

	VMOV    V6.D[1], R14
	LSR     $7, R14, R15
	AND     R19, R15, R15
	MUL     R20, R15, R15
	LSR     $56, R15, R15
	ORR     R15<<8, R13, R13

	// Unquoted delims
	MVN     R12, R12
	AND     R13, R12, R12
	AND     $0xffff, R12, R12

count_extract:
	CBZ     R12, count_next_stride
	ADD     $1, R6, R6
	SUB     $1, R12, R14
	AND     R14, R12, R12
	B       count_extract

count_next_stride:
	ADD     $16, R9, R9
	B       count_loop_start

count_done:
	MOVD    R6, count+32(FP)
	TST     $1, R8
	CSET    NE, R11
	MOVB    R11, inQuotesOut+40(FP)
	MOVD    R9, processed+48(FP)
	RET

// func indexEscapeOrControlNEONAsm(data []byte) (index int, processed int)
TEXT ·indexEscapeOrControlNEONAsm(SB), NOSPLIT, $0-40
	MOVD    data_base+0(FP), R0
	MOVD    data_len+8(FP), R1

	MOVD    $0, R9                      // offset in data

	CMP     $16, R1
	BLO     escape_done

	MOVD    $0x20, R2
	VMOV    R2, V0.B16                  // V0: 0x20 limit for control bytes (< 0x20)

	MOVD    $0x5c, R3
	VMOV    R3, V1.B16                  // V1: '\\' escape character

	SUB     $16, R1, R10                // limit = len - 16

escape_loop_start:
	CMP     R10, R9
	BHI     escape_done

	ADD     R0, R9, R11
	VLD1    (R11), [V3.B16]             // Load 16-byte chunk

	VCMHI   V3.B16, V0.B16, V4.B16      // V4 = (0x20 > chunk) ? 0xFF : 0x00
	VCMEQ   V1.B16, V3.B16, V5.B16      // V5 = (chunk == '\\') ? 0xFF : 0x00
	VORR    V4.B16, V5.B16, V6.B16      // V6 = any match

	VMOV    V6.D[0], R12
	VMOV    V6.D[1], R13
	ORR     R13, R12, R14
	CBNZ    R14, escape_found

	ADD     $16, R9, R9
	B       escape_loop_start

escape_found:
	CBNZ    R12, escape_found_low
	RBIT    R13, R14
	CLZ     R14, R14
	LSR     $3, R14, R14
	ADD     $8, R14, R14
	ADD     R9, R14, R6
	B       escape_return

escape_found_low:
	RBIT    R12, R14
	CLZ     R14, R14
	LSR     $3, R14, R14
	ADD     R9, R14, R6

escape_return:
	MOVD    R6, index+24(FP)
	ADD     $16, R9, R9
	MOVD    R9, processed+32(FP)
	RET

escape_done:
	MOVD    $-1, R6
	MOVD    R6, index+24(FP)
	MOVD    R9, processed+32(FP)
	RET

// func indexSpecialOrControlNEONAsm(data []byte, delim byte) (index int, processed int)
TEXT ·indexSpecialOrControlNEONAsm(SB), NOSPLIT, $0-48
	MOVD    data_base+0(FP), R0
	MOVD    data_len+8(FP), R1
	MOVBU   delim+24(FP), R2

	MOVD    $0, R9                      // offset in data

	CMP     $16, R1
	BLO     special_done

	MOVD    $0x0F, R5
	VMOV    R5, V0.B16                  // V0 = 0x0F mask for nibbles

	// Load LUT_HIGH into V1
	MOVD    $0x1000080004020101, R3
	VMOV    R3, V1.D[0]
	MOVD    $0, R4
	VMOV    R4, V1.D[1]

	// Load LUT_LOW into V2
	MOVD    $0x0101010101030101, R3
	VMOV    R3, V2.D[0]
	MOVD    $0x0101190919050101, R4
	VMOV    R4, V2.D[1]

	// Broadcast delim into V9
	VMOV    R2, V9.B16

	SUB     $16, R1, R10                // limit = len - 16

	CBZ     R2, special_loop_nodelim

special_loop_delim:
	CMP     R10, R9
	BHI     special_done

	ADD     R0, R9, R11
	VLD1    (R11), [V3.B16]             // Load 16-byte chunk

	VAND    V0.B16, V3.B16, V4.B16      // low nibble
	VUSHR   $4, V3.B16, V5.B16
	VAND    V0.B16, V5.B16, V5.B16      // high nibble

	VTBL    V4.B16, [V2.B16], V6.B16    // LUT_LOW[lo]
	VTBL    V5.B16, [V1.B16], V7.B16    // LUT_HIGH[hi]
	VAND    V6.B16, V7.B16, V8.B16      // match bits

	VCMEQ   V9.B16, V3.B16, V10.B16     // delim match
	VORR    V10.B16, V8.B16, V8.B16

	VMOV    V8.D[0], R12
	VMOV    V8.D[1], R13
	ORR     R13, R12, R14
	CBNZ    R14, special_found

	ADD     $16, R9, R9
	B       special_loop_delim

special_loop_nodelim:
	CMP     R10, R9
	BHI     special_done

	ADD     R0, R9, R11
	VLD1    (R11), [V3.B16]

	VAND    V0.B16, V3.B16, V4.B16
	VUSHR   $4, V3.B16, V5.B16
	VAND    V0.B16, V5.B16, V5.B16

	VTBL    V4.B16, [V2.B16], V6.B16
	VTBL    V5.B16, [V1.B16], V7.B16
	VAND    V6.B16, V7.B16, V8.B16

	VMOV    V8.D[0], R12
	VMOV    V8.D[1], R13
	ORR     R13, R12, R14
	CBNZ    R14, special_found

	ADD     $16, R9, R9
	B       special_loop_nodelim

special_found:
	MOVD    $0, R15
	VMOV    R15, V11.B16
	VCMHI   V11.B16, V8.B16, V12.B16    // Turn non-zero match bytes to 0xFF
	VMOV    V12.D[0], R12
	VMOV    V12.D[1], R13
	CBNZ    R12, special_found_low
	RBIT    R13, R14
	CLZ     R14, R14
	LSR     $3, R14, R14
	ADD     $8, R14, R14
	ADD     R9, R14, R6
	B       special_return

special_found_low:
	RBIT    R12, R14
	CLZ     R14, R14
	LSR     $3, R14, R14
	ADD     R9, R14, R6

special_return:
	MOVD    R6, index+32(FP)
	ADD     $16, R9, R9
	MOVD    R9, processed+40(FP)
	RET

special_done:
	MOVD    $-1, R6
	MOVD    R6, index+32(FP)
	MOVD    R9, processed+40(FP)
	RET


