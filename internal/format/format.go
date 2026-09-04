package format

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/toon-format/toon-go/internal/simd"
)

// Context captures delimiter information for quoting decisions.
type Context struct {
	Active   rune
	Document rune
	InArray  bool
}

// FormatString applies TOON quoting rules to the provided string.
func FormatString(s string, ctx Context) (string, error) {
	if err := ValidateCharacters(s); err != nil {
		return "", err
	}
	if NeedsQuoting(s, ctx) {
		return QuoteString(s)
	}
	return s, nil
}

// NeedsQuoting reports whether the string must be quoted in the supplied context.
func NeedsQuoting(s string, ctx Context) bool {
	if len(s) == 0 {
		return true
	}
	if s[0] == ' ' || s[0] == '\t' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t' {
		return true
	}
	switch s {
	case "true", "false", "null":
		return true
	}
	if LooksNumeric(s) {
		return true
	}
	if HasLeadingZeroDecimal(s) {
		return true
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "#") {
		return true
	}

	delim := ctx.Document
	if ctx.InArray {
		delim = ctx.Active
	}
	if delim < utf8.RuneSelf && simd.NeedsQuotingAuto(stringBytes(s), byte(delim)) {
		return true
	}
	if delim >= utf8.RuneSelf && strings.ContainsRune(s, delim) {
		return true
	}
	return false
}

// QuoteString escapes and wraps the string in double quotes.
func QuoteString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return quoteStringScalar(s), nil
	}

	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')

	data := stringBytes(s)
	start := 0
	for start < len(s) {
		idx := simd.IndexSpecialOrControlAuto(data[start:], 0)
		if idx < 0 {
			b.WriteString(s[start:])
			break
		}
		idx += start
		b.WriteString(s[start:idx])
		switch s[idx] {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if s[idx] < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, s[idx])
			} else {
				b.WriteByte(s[idx])
			}
		}
		start = idx + 1
	}
	b.WriteByte('"')
	return b.String(), nil
}

// quoteStringScalar preserves QuoteString's historical range-based behavior
// for invalid UTF-8 input, where invalid byte sequences become RuneError.
func quoteStringScalar(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func stringBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// ValidateCharacters ensures the string does not contain unsupported control characters.
func ValidateCharacters(s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("toon: string is not valid UTF-8")
	}
	return nil
}

// LooksNumeric reports whether the string resembles a numeric literal.
func LooksNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	i := 0
	if s[0] == '-' || s[0] == '+' {
		i++
		if i == len(s) {
			return false
		}
	}
	digits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		digits++
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if i == len(s) || !isDigit(s[i]) {
			return false
		}
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i == len(s) || !isDigit(s[i]) {
			return false
		}
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	return i == len(s)
}

// HasLeadingZeroDecimal reports whether the string is a decimal with forbidden leading zeros.
func HasLeadingZeroDecimal(s string) bool {
	if len(s) < 2 {
		return false
	}
	if s[0] != '0' {
		return false
	}
	return s[1] >= '0' && s[1] <= '9'
}

// EncodeKey applies TOON key quoting rules.
func EncodeKey(key string) (string, error) {
	if key == "" {
		return QuoteString(key)
	}
	if IsValidUnquotedKey(key) {
		return key, nil
	}
	return QuoteString(key)
}

// IsValidUnquotedKey reports whether the key satisfies the identifier pattern.
func IsValidUnquotedKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if i == 0 {
			if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
				return false
			}
		} else if c != '_' && c != '.' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

var decoderNumber = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// FormatNumber emits the deterministic v4.1 canonical representation.
func FormatNumber(f float64) string {
	if f == 0 {
		return "0"
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	i := strings.IndexByte(s, 'e')
	exp := s[i+1:]
	sign := "+"
	if exp[0] == '+' || exp[0] == '-' {
		sign, exp = exp[:1], exp[1:]
	}
	exp = strings.TrimLeft(exp, "0")
	if exp == "" {
		exp = "0"
	}
	return s[:i] + "e" + sign + exp
}

// ParseNumber recognizes exactly the v4.1 decoder number grammar.
func ParseNumber(token string) (float64, bool) {
	if !decoderNumber.MatchString(token) {
		return 0, false
	}
	digits := strings.TrimPrefix(token, "-")
	if len(digits) > 1 && digits[0] == '0' && digits[1] >= '0' && digits[1] <= '9' {
		return 0, false
	}
	f, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0, false
	}
	if f == 0 {
		return 0, true
	}
	return f, true
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
