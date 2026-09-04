package codec

import "unicode/utf8"

// encBuffer is an append-only contiguous byte buffer designed for low-allocation encoding.
type encBuffer struct {
	buf []byte
}

func newEncBuffer(hint int) encBuffer {
	if hint <= 0 {
		hint = 64
	} else if hint > 64*1024*1024 {
		hint = 64 * 1024 * 1024
	}
	return encBuffer{buf: make([]byte, 0, hint)}
}

func (b *encBuffer) Len() int {
	return len(b.buf)
}

func (b *encBuffer) Cap() int {
	return cap(b.buf)
}

func (b *encBuffer) Reset() {
	b.buf = b.buf[:0]
}

func (b *encBuffer) Bytes() []byte {
	return b.buf
}

func (b *encBuffer) String() string {
	return string(b.buf)
}

func (b *encBuffer) Grow(n int) {
	if cap(b.buf)-len(b.buf) < n {
		newCap := 2*cap(b.buf) + n
		if newCap < 64 {
			newCap = 64
		}
		newBuf := make([]byte, len(b.buf), newCap)
		copy(newBuf, b.buf)
		b.buf = newBuf
	}
}

func (b *encBuffer) WriteByte(c byte) {
	b.buf = append(b.buf, c)
}

func (b *encBuffer) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

func (b *encBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *encBuffer) WriteRune(r rune) {
	if r < utf8.RuneSelf {
		b.buf = append(b.buf, byte(r))
		return
	}
	var tmp [utf8.UTFMax]byte
	n := utf8.EncodeRune(tmp[:], r)
	b.buf = append(b.buf, tmp[:n]...)
}

func (b *encBuffer) WriteSpaces(n int) {
	if n <= 0 {
		return
	}
	const spaces = "                                " // 32 spaces
	for n > len(spaces) {
		b.buf = append(b.buf, spaces...)
		n -= len(spaces)
	}
	b.buf = append(b.buf, spaces[:n]...)
}

// estimateBufferSize computes a preallocation capacity hint based on the normalized structure.
func estimateBufferSize(v normalizedValue) int {
	switch val := v.(type) {
	case nil:
		return 8
	case bool:
		return 8
	case numberValue:
		return len(val.literal) + 4
	case string:
		return len(val) + 4
	case Object:
		if val.IsEmpty() {
			return 16
		}
		total := 0
		for _, f := range val.Fields {
			total += len(f.Key) + 4 + estimateBufferSize(f.Value)
		}
		return total
	case []normalizedValue:
		n := len(val)
		if n == 0 {
			return 16
		}
		if isPrimitiveArray(val) {
			return n*12 + 32
		}
		if fields, ok := detectTabular(val); ok {
			rowWidth := len(fields) * 16
			return 64 + n*(rowWidth+2)
		}
		sample := estimateBufferSize(val[0])
		return n * (sample + 8)
	default:
		return 64
	}
}
