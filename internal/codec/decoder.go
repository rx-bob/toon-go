package codec

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	formatpkg "github.com/toon-format/toon-go/internal/format"
	parsepkg "github.com/toon-format/toon-go/internal/parse"
)

// Decoder parses TOON documents into Go values that match the data model from
// Section 2. Numbers are returned as float64, objects as map[string]any, and
// arrays as []any. Strings are unescaped per Section 7.1.
type Decoder struct {
	cfg decoderOptions
}

// NewDecoder constructs a Decoder with the given options.
func NewDecoder(opts ...DecoderOption) *Decoder {
	cfg := defaultDecoderOptions()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Decoder{cfg: cfg}
}

// Decode parses the provided TOON document.
func (d *Decoder) Decode(data []byte) (any, error) {
	if d.cfg.strict && !utf8.Valid(data) {
		return nil, errors.New("toon: input is not valid UTF-8")
	}
	parser, err := newParser(string(data), d.cfg)
	if err != nil {
		return nil, err
	}
	value, err := parser.parseDocument()
	if err != nil {
		return nil, err
	}
	return value, nil
}

// DecodeString is a convenience wrapper around Decode.
func (d *Decoder) DecodeString(doc string) (any, error) {
	parser, err := newParser(doc, d.cfg)
	if err != nil {
		return nil, err
	}
	return parser.parseDocument()
}

// Decode uses a temporary decoder configured with opts.
func Decode(data []byte, opts ...DecoderOption) (any, error) {
	return NewDecoder(opts...).Decode(data)
}

// DecodeString decodes s using a temporary decoder.
func DecodeString(s string, opts ...DecoderOption) (any, error) {
	return NewDecoder(opts...).DecodeString(s)
}

type parser struct {
	lines []parsedLine
	pos   int
	cfg   decoderOptions
}

type parsedLine struct {
	number  int
	indent  int
	content string
	raw     string
	blank   bool
}

func newParser(input string, cfg decoderOptions) (*parser, error) {
	rawLines := scanLines(input)
	lines := make([]parsedLine, 0, len(rawLines))
	for _, scanned := range rawLines {
		if scanned.comment {
			continue
		}
		indent, content, err := computeIndent(scanned.text, cfg)
		if err != nil {
			return nil, errorWrap(scanned.number, err)
		}
		lines = append(lines, parsedLine{
			number:  scanned.number,
			indent:  indent,
			content: content,
			raw:     scanned.text,
			blank:   content == "",
		})
	}
	return &parser{
		lines: lines,
		cfg:   cfg,
	}, nil
}

type scannedLine struct {
	number  int
	text    string
	comment bool
}

// scanLines is the document pre-pass. It preserves original line numbers,
// removes comments before indentation validation, and performs only the
// whitespace normalization allowed by the TOON grammar.
func scanLines(input string) []scannedLine {
	if strings.HasPrefix(input, "\ufeff") {
		input = input[len("\ufeff"):]
	}
	lines := make([]scannedLine, 0)
	start, number := 0, 1
	for i := 0; i < len(input); i++ {
		if input[i] != '\n' && input[i] != '\r' {
			continue
		}
		lines = append(lines, makeScannedLine(number, input[start:i]))
		number++
		if input[i] == '\r' && i+1 < len(input) && input[i+1] == '\n' {
			i++
		}
		start = i + 1
	}
	if start < len(input) {
		lines = append(lines, makeScannedLine(number, input[start:]))
	}
	return lines
}

func makeScannedLine(number int, line string) scannedLine {
	line = strings.TrimRight(line, " ")
	comment := false
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			continue
		}
		comment = line[i] == '#'
		break
	}
	return scannedLine{number: number, text: line, comment: comment}
}

func computeIndent(line string, cfg decoderOptions) (int, string, error) {
	indent := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			indent++
		case '\t':
			if cfg.strict {
				return 0, "", errors.New("tabs are not allowed in indentation (strict mode)")
			}
			// Non-strict mode treats a tab as one indentation level. This keeps
			// tab-indented data usable without allowing it to collapse to depth 0
			// when indentSize is greater than one.
			indent += cfg.indentSize
		default:
			content := line[i:]
			if cfg.strict && indent%cfg.indentSize != 0 {
				return 0, "", fmt.Errorf("indentation must be a multiple of %d spaces", cfg.indentSize)
			}
			return indent / cfg.indentSize, content, nil
		}
	}
	// Entire line whitespace.
	return 0, "", nil
}

func (p *parser) parseDocument() (any, error) {
	p.skipBlankLinesOutsideArrays()
	if p.pos >= len(p.lines) {
		return map[string]any{}, nil
	}

	nonBlank := p.countRemainingNonBlank()
	first := p.current()

	header, ok, err := p.parseHeader(first)
	if err != nil {
		return nil, errorWrap(first.number, err)
	}

	if first.indent == 0 && first.content == "[]" {
		p.pos++
		return p.finishRoot([]any{})
	}

	if nonBlank == 1 && first.indent == 0 && !ok && !isKeyValue(first.content) {
		token := trimSpaces(first.content)
		value, err := decodePrimitiveToken(token)
		if err != nil {
			return nil, errorWrap(first.number, err)
		}
		p.pos++
		return value, nil
	}

	if ok && first.indent == 0 && !header.keyPresent {
		p.pos++
		value, err := p.parseArray(header, 0)
		if err != nil {
			return nil, err
		}
		return p.finishRoot(value)
	}

	return p.parseObject(0)
}

// finishRoot enforces the root-form boundary after a root array has been
// parsed. Non-strict mode permits trailing content, but it is never consumed
// as part of the completed root value.
func (p *parser) finishRoot(value any) (any, error) {
	p.skipBlankLinesOutsideArrays()
	if p.pos >= len(p.lines) {
		return value, nil
	}
	if p.cfg.strict {
		return nil, errorAt(p.current().number, "trailing content after root value")
	}
	// Non-strict mode may ignore trailing structural content, but a scalar
	// line is never valid outside root-primitive position.
	for _, line := range p.lines[p.pos:] {
		if !line.blank && !isKeyValue(line.content) {
			return nil, errorAt(line.number, "scalar line outside root primitive position")
		}
	}
	return value, nil
}

func (p *parser) parseObject(depth int) (map[string]any, error) {
	result := make(map[string]any)
	for p.pos < len(p.lines) {
		line := p.current()
		if line.blank {
			p.pos++
			continue
		}
		if line.indent < depth {
			break
		}
		if line.indent > depth {
			return nil, errorAt(line.number, "unexpected indentation")
		}
		header, isHeader, err := p.parseHeader(line)
		if err != nil {
			return nil, errorWrap(line.number, err)
		}
		if isHeader {
			if !header.keyPresent {
				return nil, errorAt(line.number, "arrays within objects must have a key")
			}
			p.pos++
			value, err := p.parseArray(header, depth)
			if err != nil {
				return nil, err
			}
			result[header.key] = value
			continue
		}

		key, rest, err := splitKeyValue(line.content)
		if err != nil {
			return nil, errorWrap(line.number, err)
		}
		p.pos++
		if rest == "" {
			nextValue, err := p.parseObject(depth + 1)
			if err != nil {
				return nil, err
			}
			result[key] = nextValue
			continue
		}

		value, err := decodePrimitiveToken(rest)
		if err != nil {
			return nil, errorWrap(line.number, err)
		}
		result[key] = value
	}
	return result, nil
}

func (p *parser) parseArray(header parsedHeader, depth int) (any, error) {
	delimiter := header.delimiter.rune()
	var values []any
	ctx := p.cfg

	if len(header.inlineValues) > 0 {
		raw, err := parsepkg.SplitInlineValues(header.inlineValues, delimiter)
		if err != nil {
			return nil, errorWrap(p.lines[p.pos-1].number, err)
		}
		for _, token := range raw {
			value, err := decodePrimitiveToken(token)
			if err != nil {
				return nil, errorWrap(p.lines[p.pos-1].number, err)
			}
			values = append(values, value)
		}
		if ctx.strict && len(values) != header.length {
			return nil, errorAtf(p.lines[p.pos-1].number, "inline array length mismatch; expected %d, got %d", header.length, len(values))
		}
		return values, nil
	}

	if len(header.leafFields) > 0 {
		rows := make([]any, 0, header.length)
		for p.pos < len(p.lines) {
			line := p.current()
			if line.blank {
				if ctx.strict {
					if nextIndent, ok := p.nextNonBlankIndent(p.pos); !ok || nextIndent <= depth {
						break
					}
					return nil, errorAt(line.number, "blank line inside tabular array")
				}
				p.pos++
				continue
			}
			if line.indent <= depth {
				break
			}
			if line.indent != depth+1 {
				return nil, errorAt(line.number, "invalid indentation for tabular row")
			}
			trimmed := trimSpaces(line.content)
			if indexOutsideQuotes(trimmed, ':') != -1 {
				break
			}
			p.pos++
			raw, err := parsepkg.SplitInlineValues(trimmed, delimiter)
			if err != nil {
				return nil, errorWrap(line.number, err)
			}
			if ctx.strict && len(raw) != len(header.leafFields) {
				return nil, errorAt(line.number, "tabular row width mismatch")
			}
			row := make(map[string]any, len(header.leafFields))
			for idx, field := range header.leafFields {
				if idx >= len(raw) {
					break
				}
				value, err := decodePrimitiveToken(raw[idx])
				if err != nil {
					return nil, errorWrap(line.number, err)
				}
				row[field] = value
			}
			rows = append(rows, row)
			if ctx.strict && len(rows) > header.length {
				return nil, errorAtf(line.number, "too many tabular rows (expected %d)", header.length)
			}
		}
		if ctx.strict && len(rows) != header.length {
			return nil, errorAtf(p.lines[p.pos-1].number, "tabular length mismatch; expected %d rows", header.length)
		}
		return rows, nil
	}

	values = make([]any, 0, header.length)
	for p.pos < len(p.lines) {
		line := p.current()
		if line.blank {
			if ctx.strict {
				if nextIndent, ok := p.nextNonBlankIndent(p.pos); !ok || nextIndent <= depth {
					break
				}
				return nil, errorAt(line.number, "blank line inside list array")
			}
			p.pos++
			continue
		}
		if line.indent <= depth {
			break
		}
		if line.indent != depth+1 {
			return nil, errorAt(line.number, "invalid indentation for list item")
		}
		if !strings.HasPrefix(line.content, "-") {
			break
		}
		itemContent := trimSpaces(line.content[1:])
		p.pos++
		if itemContent == "" {
			values = append(values, map[string]any{})
			continue
		}

		if strings.HasPrefix(itemContent, "[") {
			itemHeader, ok, err := tryParseHeader(itemContent)
			itemHeader.sourceLine = line.number
			if err != nil {
				return nil, errorWrap(line.number, err)
			}
			if !ok {
				return nil, errorAt(line.number, "invalid array header in list item")
			}
			itemValue, err := p.parseArray(itemHeader, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, itemValue)
			continue
		}

		if header, isHeader, err := tryParseHeader(itemContent); err != nil {
			return nil, errorWrap(line.number, err)
		} else if isHeader {
			if !header.keyPresent {
				return nil, errorAt(line.number, "arrays within objects must have a key")
			}
			arrayValue, err := p.parseArray(header, depth+1)
			if err != nil {
				return nil, err
			}
			obj := map[string]any{header.key: arrayValue}
			if err := p.collectObjectListSiblings(obj, depth); err != nil {
				return nil, err
			}
			values = append(values, obj)
			continue
		}

		if isKeyValue(itemContent) {
			key, rest, err := splitKeyValue(itemContent)
			if err != nil {
				return nil, errorWrap(line.number, err)
			}
			if rest == "" {
				obj, err := p.parseObject(depth + 3)
				if err != nil {
					return nil, err
				}
				values = append(values, map[string]any{key: obj})
				continue
			}
			val, err := decodePrimitiveToken(rest)
			if err != nil {
				return nil, errorWrap(line.number, err)
			}
			obj := map[string]any{key: val}
			if err := p.collectObjectListSiblings(obj, depth); err != nil {
				return nil, err
			}
			values = append(values, obj)
			continue
		}

		value, err := decodePrimitiveToken(itemContent)
		if err != nil {
			return nil, errorWrap(line.number, err)
		}
		values = append(values, value)
	}

	if ctx.strict && len(values) != header.length {
		return nil, errorAtf(p.lines[p.pos-1].number, "list length mismatch; expected %d items", header.length)
	}
	return values, nil
}

func (p *parser) current() parsedLine {
	return p.lines[p.pos]
}

func (p *parser) skipBlankLinesOutsideArrays() {
	for p.pos < len(p.lines) {
		if !p.lines[p.pos].blank {
			break
		}
		p.pos++
	}
}

func (p *parser) countRemainingNonBlank() int {
	count := 0
	for _, line := range p.lines[p.pos:] {
		if !line.blank {
			count++
		}
	}
	return count
}

func (p *parser) nextNonBlankIndent(from int) (int, bool) {
	for i := from + 1; i < len(p.lines); i++ {
		if !p.lines[i].blank {
			return p.lines[i].indent, true
		}
	}
	return 0, false
}

func (p *parser) collectObjectListSiblings(obj map[string]any, depth int) error {
	for p.pos < len(p.lines) {
		next := p.current()
		if next.blank {
			if p.cfg.strict {
				if nextIndent, ok := p.nextNonBlankIndent(p.pos); !ok || nextIndent <= depth+1 {
					break
				}
				return errorAt(next.number, "blank line inside object list item")
			}
			p.pos++
			continue
		}
		if next.indent <= depth+1 {
			break
		}
		if next.indent != depth+2 {
			return errorAt(next.number, "invalid indentation for object list sibling")
		}
		if header, isHeader, err := p.parseHeader(next); err != nil {
			return errorWrap(next.number, err)
		} else if isHeader {
			p.pos++
			value, err := p.parseArray(header, depth+1)
			if err != nil {
				return err
			}
			if !header.keyPresent {
				return errorAt(next.number, "arrays within objects must have a key")
			}
			obj[header.key] = value
			continue
		}
		key, rest, err := splitKeyValue(next.content)
		if err != nil {
			return errorWrap(next.number, err)
		}
		p.pos++
		if rest == "" {
			nested, err := p.parseObject(depth + 3)
			if err != nil {
				return err
			}
			obj[key] = nested
		} else {
			value, err := decodePrimitiveToken(rest)
			if err != nil {
				return errorWrap(next.number, err)
			}
			obj[key] = value
		}
	}
	return nil
}

func (p *parser) parseHeader(line parsedLine) (parsedHeader, bool, error) {
	header, ok, err := tryParseHeader(line.content)
	if ok {
		header.sourceLine = line.number
	}
	return header, ok, err
}

type parsedHeader struct {
	key          string
	keyPresent   bool
	keyed        bool
	length       int
	delimiter    Delimiter
	fieldTree    []fieldNode
	leafFields   []string
	inlineValues string
	sourceLine   int
}

func tryParseHeader(content string) (parsedHeader, bool, error) {
	bracketStart := indexOutsideQuotes(content, '[')
	if bracketStart == -1 {
		return parsedHeader{}, false, nil
	}
	bracketEnd := matchingBracket(content, bracketStart)
	if bracketEnd == -1 {
		return parsedHeader{}, false, errors.New("missing closing bracket in array header")
	}
	colon := indexOutsideQuotesFrom(content, ':', bracketEnd+1)
	if colon == -1 {
		return parsedHeader{}, false, nil
	}
	left := trimSpaces(content[:colon])
	right := trimSpaces(content[colon+1:])
	keyPart := trimSpaces(left[:bracketStart])
	bracketSegment := content[bracketStart+1 : bracketEnd]
	fieldSegment := trimSpaces(content[bracketEnd+1 : colon])

	header := parsedHeader{
		key:        "",
		delimiter:  DelimiterComma,
		keyPresent: keyPart != "",
	}

	if keyPart != "" {
		key, err := decodeKeyToken(keyPart)
		if err != nil {
			return parsedHeader{}, false, err
		}
		header.key = key
	}

	length, delim, keyed, err := parseBracketSegment(bracketSegment)
	if err != nil {
		return parsedHeader{}, false, err
	}
	header.length = length
	header.delimiter = delim
	header.keyed = keyed

	if fieldSegment != "" {
		if !strings.HasPrefix(fieldSegment, "{") || !strings.HasSuffix(fieldSegment, "}") {
			return parsedHeader{}, false, errors.New("invalid field segment in array header")
		}
		inner := fieldSegment[1 : len(fieldSegment)-1]
		fields, err := parseFieldNodes(inner, delim.rune())
		if err != nil {
			return parsedHeader{}, false, err
		}
		header.fieldTree = fields
		header.leafFields = flattenFields(fields)
	}

	header.inlineValues = right
	return header, true, nil
}

func matchingBracket(s string, start int) int {
	inQuotes, escaped := false, false
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inQuotes {
			escaped = true
			continue
		}
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && c == ']' {
			return i
		}
	}
	return -1
}

func indexOutsideQuotesFrom(s string, target rune, start int) int {
	if start < 0 {
		start = 0
	}
	part := s[start:]
	i := indexOutsideQuotes(part, target)
	if i == -1 {
		return -1
	}
	return start + i
}

func parseFieldNodes(segment string, delimiter rune) ([]fieldNode, error) {
	parts, err := splitFieldEntries(segment, delimiter)
	if err != nil {
		return nil, err
	}
	fields := make([]fieldNode, 0, len(parts))
	for _, part := range parts {
		part = trimSpaces(part)
		if part == "" {
			return nil, errors.New("empty field name")
		}
		open := indexOutsideQuotes(part, '{')
		if open == -1 {
			name, err := decodeKeyToken(part)
			if err != nil {
				return nil, err
			}
			fields = append(fields, fieldNode{name: name})
			continue
		}
		if !strings.HasSuffix(trimSpaces(part), "}") {
			return nil, errors.New("invalid nested field group")
		}
		close := matchingBrace(part, open)
		if close != len(strings.TrimSpace(part))-1 {
			return nil, errors.New("invalid nested field group")
		}
		name, err := decodeKeyToken(trimSpaces(part[:open]))
		if err != nil {
			return nil, err
		}
		children, err := parseFieldNodes(part[open+1:close], delimiter)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			return nil, errors.New("empty nested field group")
		}
		fields = append(fields, fieldNode{name: name, children: children})
	}
	return fields, nil
}

func splitFieldEntries(s string, delimiter rune) ([]string, error) {
	var parts []string
	start, depth := 0, 0
	inQuotes, escaped := false, false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inQuotes {
			escaped = true
			continue
		}
		if r == '"' {
			inQuotes = !inQuotes
			continue
		}
		if inQuotes {
			continue
		}
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return nil, errors.New("unbalanced field group")
			}
		default:
			if r == delimiter && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if inQuotes || depth != 0 {
		return nil, errors.New("unbalanced field group")
	}
	return append(parts, s[start:]), nil
}

func matchingBrace(s string, start int) int {
	depth := 0
	inQuotes, escaped := false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inQuotes {
			escaped = true
			continue
		}
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if inQuotes {
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseBracketSegment(segment string) (int, Delimiter, bool, error) {
	useMarker := false
	keyed := false
	if strings.HasSuffix(segment, ":") {
		keyed = true
		segment = strings.TrimSuffix(segment, ":")
	}
	if strings.HasPrefix(segment, "#") {
		useMarker = true
		segment = segment[1:]
	}
	if segment == "" {
		return 0, DelimiterComma, keyed, errors.New("missing array length")
	}
	var digits strings.Builder
	var delim = DelimiterComma
	for _, r := range segment {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
			continue
		}
		switch r {
		case '\t':
			delim = DelimiterTab
		case '|':
			delim = DelimiterPipe
		default:
			return 0, DelimiterComma, keyed, fmt.Errorf("invalid delimiter symbol %q", r)
		}
	}
	lengthStr := digits.String()
	if lengthStr == "" {
		return 0, DelimiterComma, keyed, errors.New("missing digits in array length")
	}
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return 0, DelimiterComma, keyed, err
	}
	_ = useMarker // legacy marker remains rejected by later grammar work.
	_ = keyed
	return length, delim, keyed, nil
}

func splitKeyValue(content string) (string, string, error) {
	colon := indexOutsideQuotes(content, ':')
	if colon == -1 {
		return "", "", errors.New("missing colon after key")
	}
	keyToken := trimSpaces(content[:colon])
	valueToken := trimSpaces(content[colon+1:])
	key, err := decodeKeyToken(keyToken)
	if err != nil {
		return "", "", err
	}
	return key, valueToken, nil
}

func trimSpaces(s string) string {
	return strings.Trim(s, " ")
}

func decodeKeyToken(token string) (string, error) {
	if token == "" {
		return "", errors.New("empty key")
	}
	if token[0] == '"' {
		return parsepkg.UnquoteString(token)
	}
	if !formatpkg.IsValidUnquotedKey(token) {
		return "", fmt.Errorf("invalid unquoted key %q", token)
	}
	return token, nil
}

func decodePrimitiveToken(token string) (any, error) {
	if token == "" {
		return "", nil
	}
	if token[0] == '"' {
		return parsepkg.UnquoteString(token)
	}
	switch token {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if num, ok := formatpkg.ParseNumber(token); ok {
		return num, nil
	}
	return token, nil
}

func hasForbiddenLeadingZeros(token string) bool {
	if len(token) < 2 {
		return false
	}
	if token[0] != '0' && (len(token) <= 1 || token[0] != '-' || token[1] != '0') {
		return false
	}
	// tokens like -0.x are legitimate numbers.
	if strings.Contains(token, ".") || strings.ContainsAny(token, "eE") {
		return false
	}
	if token[0] == '-' {
		return len(token) > 2 && token[1] == '0' && unicode.IsDigit(rune(token[2]))
	}
	return unicode.IsDigit(rune(token[1]))
}

func isKeyValue(content string) bool {
	return indexOutsideQuotes(content, ':') > 0
}

func indexOutsideQuotes(s string, target rune) int {
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
