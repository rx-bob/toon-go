package parse

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/toon-format/toon-go/internal/simd"
)

// UnquoteString removes surrounding quotes and unescapes TOON strings.
func UnquoteString(token string) (string, error) {
	if len(token) < 2 || token[0] != '"' || token[len(token)-1] != '"' {
		return "", errors.New("invalid quoted string")
	}
	body := token[1 : len(token)-1]
	if !simd.HasEscape(unsafe.Slice(unsafe.StringData(body), len(body))) {
		if !utf8.ValidString(body) {
			return "", errors.New("invalid UTF-8 in quoted string")
		}
		return body, nil
	}
	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if ch == '\\' {
			i++
			if i >= len(body) {
				return "", errors.New("unterminated escape sequence")
			}
			switch body[i] {
			case '\\', '"':
				b.WriteByte(body[i])
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				if i+4 >= len(body) {
					return "", errors.New(`\u escape requires four hex digits`)
				}
				hex := body[i+1 : i+5]
				code, err := strconv.ParseUint(hex, 16, 16)
				if err != nil {
					return "", fmt.Errorf(`invalid \u escape %q`, hex)
				}
				if code >= 0xd800 && code <= 0xdfff {
					return "", fmt.Errorf(`surrogate escape \u%s is not allowed`, hex)
				}
				b.WriteRune(rune(code))
				i += 4
			default:
				return "", fmt.Errorf("invalid escape sequence \\%c", body[i])
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(body[i:])
		if r == utf8.RuneError && size == 1 {
			return "", errors.New("invalid UTF-8 in quoted string")
		}
		b.WriteString(body[i : i+size])
		i += size - 1
	}
	return b.String(), nil
}

// IndexUnquoted returns the byte index of target outside quoted regions.
func IndexUnquoted(s string, target rune) int {
	if target < utf8.RuneSelf {
		b := unsafe.Slice(unsafe.StringData(s), len(s))
		return simd.IndexUnquotedAuto(b, byte(target))
	}
	inQuotes := false
	escaped := false
	for idx, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inQuotes:
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case !inQuotes && r == target:
			return idx
		}
	}
	return -1
}

func trimSpaces(s string) string {
	start := 0
	for start < len(s) && s[start] == ' ' {
		start++
	}
	end := len(s)
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}

// SplitInlineValues tokenizes a delimiter-separated list, respecting quoted segments.
func SplitInlineValues(segment string, delimiter rune) ([]string, error) {
	tokens, _, err := SplitInlineValuesAppend(segment, delimiter, nil, nil)
	return tokens, err
}

// SplitInlineValuesAppend tokenizes a delimiter-separated list, respecting quoted segments,
// appending tokens to dst and using delimsBuf to hold delimiter positions.
func SplitInlineValuesAppend(segment string, delimiter rune, dst []string, delimsBuf []int) ([]string, []int, error) {
	if trimSpaces(segment) == "" {
		return dst, delimsBuf, nil
	}

	if delimiter < utf8.RuneSelf {
		b := unsafe.Slice(unsafe.StringData(segment), len(segment))
		delims, inQuotes := simd.FindDelimsAuto(b, byte(delimiter), delimsBuf)
		if inQuotes {
			return nil, delims, errors.New("unterminated string in delimited values")
		}

		needed := len(delims) + 1
		if cap(dst)-len(dst) < needed {
			grow := make([]string, len(dst), len(dst)+needed)
			copy(grow, dst)
			dst = grow
		}

		start := 0
		for _, idx := range delims {
			dst = append(dst, trimSpaces(segment[start:idx]))
			start = idx + 1
		}
		dst = append(dst, trimSpaces(segment[start:]))
		return dst, delims, nil
	}

	// Fallback for non-ASCII rune delimiters
	inQuotes := false
	escaped := false
	start := 0

	for i, r := range segment {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inQuotes:
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case r == delimiter && !inQuotes:
			dst = append(dst, trimSpaces(segment[start:i]))
			start = i + utf8.RuneLen(r)
		}
	}
	if inQuotes {
		return nil, delimsBuf, errors.New("unterminated string in delimited values")
	}
	dst = append(dst, trimSpaces(segment[start:]))
	return dst, delimsBuf, nil
}
