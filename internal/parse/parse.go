package parse

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// UnquoteString removes surrounding quotes and unescapes TOON strings.
func UnquoteString(token string) (string, error) {
	if len(token) < 2 || token[0] != '"' || token[len(token)-1] != '"' {
		return "", errors.New("invalid quoted string")
	}
	body := token[1 : len(token)-1]
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
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if inQuotes {
				i++
			}
		case '"':
			inQuotes = !inQuotes
		default:
			if !inQuotes && s[i] < utf8.RuneSelf && rune(s[i]) == target {
				return i
			}
		}
	}
	return -1
}

// SplitInlineValues tokenizes a delimiter-separated list, respecting quoted segments.
func SplitInlineValues(segment string, delimiter rune) ([]string, error) {
	if strings.Trim(segment, " ") == "" {
		return nil, nil
	}
	var tokens []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, r := range segment {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && inQuotes:
			current.WriteRune(r)
			escaped = true
		case r == '"':
			current.WriteRune(r)
			inQuotes = !inQuotes
		case r == delimiter && !inQuotes:
			tokens = append(tokens, strings.Trim(current.String(), " "))
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if inQuotes {
		return nil, errors.New("unterminated string in delimited values")
	}
	tokens = append(tokens, strings.Trim(current.String(), " "))
	return tokens, nil
}
