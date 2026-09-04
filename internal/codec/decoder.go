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

func (d *Decoder) decodeForUnmarshal(data []byte) (any, error) {
	if d.cfg.strict && !utf8.Valid(data) {
		return nil, errors.New("toon: input is not valid UTF-8")
	}
	parser, err := newParser(string(data), d.cfg)
	if err != nil {
		return nil, err
	}
	parser.preserveNumbers = true
	return parser.parseDocument()
}

// DecodeString is a convenience wrapper around Decode.
func (d *Decoder) DecodeString(doc string) (any, error) {
	parser, err := newParser(doc, d.cfg)
	if err != nil {
		return nil, err
	}
	return parser.parseDocument()
}

func (d *Decoder) decodeStringForUnmarshal(doc string) (any, error) {
	parser, err := newParser(doc, d.cfg)
	if err != nil {
		return nil, err
	}
	parser.preserveNumbers = true
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
	lines             []parsedLine
	pos               int
	cfg               decoderOptions
	preserveNumbers   bool
	remainingNonBlank []int
	nextNonBlank      []int
}

// These limits protect recursive parsing and header bookkeeping from hostile
// input. They are implementation safeguards, not a format-level restriction.
const (
	maxDecodeDepth = 64
	maxHeaderBytes = 64 * 1024
)

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
	remainingNonBlank := make([]int, len(lines)+1)
	nextNonBlank := make([]int, len(lines)+1)
	for i := len(lines) - 1; i >= 0; i-- {
		remainingNonBlank[i] = remainingNonBlank[i+1]
		if !lines[i].blank {
			remainingNonBlank[i]++
			nextNonBlank[i] = lines[i].indent
		} else {
			nextNonBlank[i] = nextNonBlank[i+1]
		}
	}
	return &parser{
		lines:             lines,
		cfg:               cfg,
		remainingNonBlank: remainingNonBlank,
		nextNonBlank:      nextNonBlank,
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
		value, err := p.decodePrimitiveToken(token)
		if err != nil {
			return nil, errorWrap(first.number, err)
		}
		p.pos++
		return value, nil
	}

	if ok && first.indent == 0 && !header.keyPresent {
		p.pos++
		if header.keyed {
			value, err := p.parseKeyedObject(header, 0)
			if err != nil {
				return nil, err
			}
			return p.finishRoot(value)
		}
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
	if err := p.checkDepth(depth); err != nil {
		return nil, err
	}
	result := make(map[string]any)
	seen := make(map[string]struct{})
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
			var value any
			if header.keyed {
				value, err = p.parseKeyedObject(header, depth)
			} else {
				value, err = p.parseArray(header, depth)
			}
			if err != nil {
				return nil, err
			}
			if err := p.setObjectField(result, seen, header.key, value, line.number); err != nil {
				return nil, err
			}
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
			if err := p.setObjectField(result, seen, key, nextValue, line.number); err != nil {
				return nil, err
			}
			continue
		}

		var value any
		if rest == "[]" {
			value = []any{}
		} else {
			value, err = p.decodePrimitiveToken(rest)
			if err != nil {
				return nil, errorWrap(line.number, err)
			}
		}
		if err := p.setObjectField(result, seen, key, value, line.number); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *parser) setObjectField(obj map[string]any, seen map[string]struct{}, key string, value any, line int) error {
	if _, exists := seen[key]; exists && p.cfg.strict {
		return errorAtf(line, "duplicate object key %q", key)
	}
	seen[key] = struct{}{}
	obj[key] = value
	return nil
}

func (p *parser) parseArray(header parsedHeader, depth int) (any, error) {
	if err := p.checkDepth(depth); err != nil {
		return nil, err
	}
	delimiter := header.delimiter.rune()
	var values []any
	ctx := p.cfg

	if len(header.inlineValues) > 0 {
		raw, err := parsepkg.SplitInlineValues(header.inlineValues, delimiter)
		if err != nil {
			return nil, errorWrap(p.lines[p.pos-1].number, err)
		}
		for _, token := range raw {
			value, err := p.decodePrimitiveToken(token)
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
		rows := make([]any, 0)
		for p.pos < len(p.lines) {
			line := p.current()
			if line.blank {
				if ctx.strict {
					if len(rows) == 0 {
						p.pos++
						continue
					}
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
			colon := indexOutsideQuotes(trimmed, ':')
			separator := indexOutsideQuotes(trimmed, delimiter)
			if colon >= 0 && (separator < 0 || colon < separator) {
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
			row, err := decodeTabularRow(header.fieldTree, raw)
			if err != nil {
				return nil, errorWrap(line.number, err)
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

	values = make([]any, 0)
	for p.pos < len(p.lines) {
		line := p.current()
		if line.blank {
			if ctx.strict {
				if len(values) == 0 {
					p.pos++
					continue
				}
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
			if itemContent == "[]" {
				values = append(values, []any{})
				continue
			}
			itemHeader, ok, err := tryParseHeader(itemContent)
			itemHeader.sourceLine = line.number
			if err != nil {
				return nil, errorWrap(line.number, err)
			}
			if !ok {
				return nil, errorAt(line.number, "invalid array header in list item")
			}
			if len(itemHeader.fieldTree) > 0 && !itemHeader.keyPresent {
				return nil, errorAt(line.number, "keyless fields-bearing header is not valid in a list item")
			}
			itemDepth := depth + 1
			if itemHeader.keyPresent {
				itemDepth = depth + 2
			}
			itemValue, err := p.parseArray(itemHeader, itemDepth)
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
			var arrayValue any
			if header.keyed {
				arrayValue, err = p.parseKeyedObject(header, depth+2)
			} else {
				arrayValue, err = p.parseArray(header, depth+2)
			}
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
			val, err := p.decodePrimitiveToken(rest)
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

		value, err := p.decodePrimitiveToken(itemContent)
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

func decodeTabularRow(fields []fieldNode, raw []string) (map[string]any, error) {
	row := make(map[string]any)
	index := 0
	if err := decodeTabularFields(row, fields, raw, &index); err != nil {
		return nil, err
	}
	return row, nil
}

func (p *parser) parseKeyedObject(header parsedHeader, depth int) (map[string]any, error) {
	if err := p.checkDepth(depth); err != nil {
		return nil, err
	}
	result := make(map[string]any)
	seen := make(map[string]struct{})
	for p.pos < len(p.lines) {
		line := p.current()
		if line.blank {
			if p.cfg.strict {
				if len(seen) == 0 {
					p.pos++
					continue
				}
				if nextIndent, ok := p.nextNonBlankIndent(p.pos); !ok || nextIndent <= depth {
					break
				}
				return nil, errorAt(line.number, "blank line inside keyed array")
			}
			p.pos++
			continue
		}
		if line.indent <= depth {
			break
		}
		if line.indent != depth+1 {
			if p.cfg.strict {
				return nil, errorAt(line.number, "invalid indentation for keyed entry row")
			}
			p.pos++
			continue
		}
		colon := indexOutsideQuotes(line.content, ':')
		if colon < 0 {
			if p.cfg.strict {
				return nil, errorAt(line.number, "keyed entry row missing colon")
			}
			p.pos++
			continue
		}
		key, err := decodeKeyToken(trimSpaces(line.content[:colon]))
		if err != nil {
			return nil, errorWrap(line.number, err)
		}
		rest := trimSpaces(line.content[colon+1:])
		var raw []string
		if rest != "" {
			raw, err = parsepkg.SplitInlineValues(rest, header.delimiter.rune())
			if err != nil {
				return nil, errorWrap(line.number, err)
			}
		}
		if p.cfg.strict && len(raw) != len(header.leafFields) {
			return nil, errorAt(line.number, "keyed entry row width mismatch")
		}
		value, err := decodeTabularRow(header.fieldTree, raw)
		if err != nil {
			return nil, errorWrap(line.number, err)
		}
		p.pos++
		if _, exists := seen[key]; exists && p.cfg.strict {
			return nil, errorAtf(line.number, "duplicate object key %q", key)
		}
		seen[key] = struct{}{}
		result[key] = value
		if p.cfg.strict && len(seen) > header.length {
			return nil, errorAtf(line.number, "too many keyed entries (expected %d)", header.length)
		}
	}
	if p.cfg.strict && len(seen) != header.length {
		line := header.sourceLine
		if line == 0 && p.pos > 0 {
			line = p.lines[p.pos-1].number
		}
		return nil, errorAtf(line, "keyed entry count mismatch; expected %d rows", header.length)
	}
	return result, nil
}

func decodeTabularFields(row map[string]any, fields []fieldNode, raw []string, index *int) error {
	for _, field := range fields {
		if len(field.children) > 0 {
			nested := make(map[string]any)
			before := *index
			if err := decodeTabularFields(nested, field.children, raw, index); err != nil {
				return err
			}
			if *index > before {
				row[field.name] = nested
			}
			continue
		}
		if *index >= len(raw) {
			continue
		}
		value, err := decodePrimitiveToken(raw[*index])
		if err != nil {
			return err
		}
		row[field.name] = value
		*index++
	}
	return nil
}

func (p *parser) current() parsedLine {
	return p.lines[p.pos]
}

func (p *parser) decodePrimitiveToken(token string) (any, error) {
	value, err := decodePrimitiveToken(token)
	if err != nil || !p.preserveNumbers || !jsonNumberLexeme.MatchString(token) || hasNumberLeadingZeros(token) {
		return value, err
	}
	if number, ok := value.(float64); ok {
		return decodedNumber{literal: token, value: number}, nil
	}
	if _, ok := value.(string); ok {
		number, _ := strconv.ParseFloat(token, 64)
		return decodedNumber{literal: token, value: number}, nil
	}
	return value, nil
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
	return p.remainingNonBlank[p.pos]
}

func (p *parser) nextNonBlankIndent(from int) (int, bool) {
	from++
	if from < len(p.lines) && p.remainingNonBlank[from] != p.remainingNonBlank[len(p.lines)] {
		return p.nextNonBlank[from], true
	}
	return 0, false
}

func (p *parser) checkDepth(depth int) error {
	if depth <= maxDecodeDepth {
		return nil
	}
	line := 0
	if p.pos < len(p.lines) {
		line = p.lines[p.pos].number
	} else if p.pos > 0 {
		line = p.lines[p.pos-1].number
	}
	return errorAtf(line, "maximum nesting depth %d exceeded", maxDecodeDepth)
}

func (p *parser) collectObjectListSiblings(obj map[string]any, depth int) error {
	seen := make(map[string]struct{}, len(obj))
	for key := range obj {
		seen[key] = struct{}{}
	}
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
			var value any
			if header.keyed {
				value, err = p.parseKeyedObject(header, depth+2)
			} else {
				value, err = p.parseArray(header, depth+1)
			}
			if err != nil {
				return err
			}
			if !header.keyPresent {
				return errorAt(next.number, "arrays within objects must have a key")
			}
			if err := p.setObjectField(obj, seen, header.key, value, next.number); err != nil {
				return err
			}
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
			if err := p.setObjectField(obj, seen, key, nested, next.number); err != nil {
				return err
			}
		} else {
			var value any
			if rest == "[]" {
				value = []any{}
			} else {
				value, err = p.decodePrimitiveToken(rest)
				if err != nil {
					return errorWrap(next.number, err)
				}
			}
			if err := p.setObjectField(obj, seen, key, value, next.number); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseHeader(line parsedLine) (parsedHeader, bool, error) {
	header, ok, err := tryParseHeader(line.content)
	if err != nil && !p.cfg.strict {
		return parsedHeader{}, false, nil
	}
	if ok && p.cfg.strict {
		if err := validateFieldTree(header.fieldTree); err != nil {
			return parsedHeader{}, false, errorWrap(line.number, err)
		}
	}
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
	colonBeforeBracket := indexOutsideQuotes(content, ':')
	bracketStart := indexOutsideQuotes(content, '[')
	if bracketStart == -1 {
		return parsedHeader{}, false, nil
	}
	if len(content) > maxHeaderBytes {
		return parsedHeader{}, false, fmt.Errorf("array header exceeds maximum size of %d bytes", maxHeaderBytes)
	}
	if colonBeforeBracket >= 0 && colonBeforeBracket < bracketStart {
		return parsedHeader{}, false, nil
	}
	bracketEnd := matchingBracket(content, bracketStart)
	if bracketEnd == -1 {
		return parsedHeader{}, false, errors.New("missing closing bracket in array header")
	}
	colon := indexOutsideQuotesFrom(content, ':', bracketEnd+1)
	if colon == -1 {
		if trimSpaces(content) == "[]" {
			return parsedHeader{}, false, nil
		}
		return parsedHeader{}, false, errors.New("missing colon after array header")
	}
	right := trimSpaces(content[colon+1:])
	rawKeyPart := content[:bracketStart]
	keyPart := trimSpaces(rawKeyPart)
	if keyPart != "" && len(rawKeyPart) > 0 && (rawKeyPart[len(rawKeyPart)-1] == ' ' || rawKeyPart[len(rawKeyPart)-1] == '\t') {
		return parsedHeader{}, false, errors.New("whitespace between key and array bracket")
	}
	bracketSegment := content[bracketStart+1 : bracketEnd]
	rawFieldSegment := content[bracketEnd+1 : colon]
	if rawFieldSegment != "" && rawFieldSegment[0] != '{' {
		return parsedHeader{}, false, errors.New("content between array bracket and colon")
	}
	fieldSegment := rawFieldSegment

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
	if len(header.fieldTree) > 0 && right != "" {
		return parsedHeader{}, false, errors.New("content after fields-bearing header")
	}
	if header.keyed && len(header.fieldTree) == 0 {
		return parsedHeader{}, false, errors.New("keyed header requires a field list")
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
	return parseFieldNodesAtDepth(segment, delimiter, 0)
}

func parseFieldNodesAtDepth(segment string, delimiter rune, depth int) ([]fieldNode, error) {
	if depth > maxDecodeDepth {
		return nil, fmt.Errorf("maximum nesting depth %d exceeded", maxDecodeDepth)
	}
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
		children, err := parseFieldNodesAtDepth(part[open+1:close], delimiter, depth+1)
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
			if depth >= 0 {
				switch r {
				case ',', '|', '\t':
					if r != delimiter {
						return nil, fmt.Errorf("field-list delimiter does not match header delimiter")
					}
					if depth == 0 {
						parts = append(parts, s[start:i])
						start = i + 1
					}
				}
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
	keyed := false
	if colon := strings.IndexByte(segment, ':'); colon >= 0 {
		keyed = true
		if strings.Contains(segment[colon+1:], ":") {
			return 0, DelimiterComma, keyed, errors.New("multiple keyed markers")
		}
		tail := segment[colon+1:]
		if strings.ContainsAny(tail, ",:") || len(tail) > 1 || (len(tail) == 1 && tail[0] != '|' && tail[0] != '\t') {
			return 0, DelimiterComma, keyed, errors.New("invalid keyed marker")
		}
		if strings.ContainsAny(segment[:colon], ",|\t") {
			return 0, DelimiterComma, keyed, errors.New("misplaced keyed marker")
		}
		segment = segment[:colon] + tail
	}
	if segment == "" {
		return 0, DelimiterComma, keyed, errors.New("missing array length")
	}
	lengthText := segment
	delim := DelimiterComma
	if last := segment[len(segment)-1]; last == '\t' || last == '|' {
		lengthText = segment[:len(segment)-1]
		if last == '\t' {
			delim = DelimiterTab
		} else {
			delim = DelimiterPipe
		}
	}
	if lengthText == "" {
		return 0, DelimiterComma, keyed, errors.New("missing digits in array length")
	}
	if len(lengthText) > 1 && lengthText[0] == '0' {
		return 0, DelimiterComma, keyed, errors.New("array length has leading zeros")
	}
	for i := 0; i < len(lengthText); i++ {
		if lengthText[i] < '0' || lengthText[i] > '9' {
			return 0, DelimiterComma, keyed, fmt.Errorf("invalid array length %q", lengthText)
		}
	}
	parsed, err := strconv.ParseUint(lengthText, 10, strconv.IntSize)
	if err != nil {
		return 0, DelimiterComma, keyed, err
	}
	return int(parsed), delim, keyed, nil
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
		if closingQuote(token) != len(token)-1 {
			return "", errors.New("content after quoted key")
		}
		return parsepkg.UnquoteString(token)
	}
	// Decoder keys are literal tokens. The encoder's stricter unquoted-key
	// pattern does not constrain accepted input.
	return token, nil
}

func closingQuote(token string) int {
	escaped := false
	for i := 1; i < len(token); i++ {
		if escaped {
			escaped = false
			continue
		}
		if token[i] == '\\' {
			escaped = true
			continue
		}
		if token[i] == '"' {
			return i
		}
	}
	return -1
}

func validateFieldTree(fields []fieldNode) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, exists := seen[field.name]; exists {
			return fmt.Errorf("duplicate field name %q", field.name)
		}
		seen[field.name] = struct{}{}
	}
	for _, field := range fields {
		if err := validateFieldTree(field.children); err != nil {
			return err
		}
	}
	return nil
}

func decodePrimitiveToken(token string) (any, error) {
	if token == "" {
		return "", nil
	}
	if token[0] == '"' {
		if closingQuote(token) != len(token)-1 {
			return nil, errors.New("content after quoted string")
		}
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
