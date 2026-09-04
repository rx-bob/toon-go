package codec

import (
	"fmt"
	"strconv"
	"strings"
)

// Encoder serializes Go values as TOON documents.
type Encoder struct {
	cfg encoderOptions
}

// NewEncoder constructs an Encoder using the supplied options. Absent options
// default to the TOON Core Profile recommendations (Section 19).
func NewEncoder(opts ...EncoderOption) *Encoder {
	cfg := defaultEncoderOptions()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Encoder{cfg: cfg}
}

// Marshal renders v into a TOON document. Values are first normalized to the
// TOON data model (Section 2), then encoded using the concrete syntax rules
// in Sections 5–12.
func (e *Encoder) Marshal(v any) ([]byte, error) {
	normalized, err := normalize(v, e.cfg)
	if err != nil {
		return nil, err
	}
	state := &encodeState{
		cfg: e.cfg,
		buf: newEncBuffer(estimateBufferSize(normalized)),
	}
	if err := state.encodeRoot(normalized); err != nil {
		return nil, err
	}
	return state.buf.Bytes(), nil
}

// MarshalString is equivalent to Marshal but returns a string.
func (e *Encoder) MarshalString(v any) (string, error) {
	normalized, err := normalize(v, e.cfg)
	if err != nil {
		return "", err
	}
	state := &encodeState{
		cfg: e.cfg,
		buf: newEncBuffer(estimateBufferSize(normalized)),
	}
	if err := state.encodeRoot(normalized); err != nil {
		return "", err
	}
	return state.buf.String(), nil
}

// Marshal encodes v using a temporary encoder.
func Marshal(v any, opts ...EncoderOption) ([]byte, error) {
	return NewEncoder(opts...).Marshal(v)
}

// MarshalString encodes v as a TOON document string.
func MarshalString(v any, opts ...EncoderOption) (string, error) {
	return NewEncoder(opts...).MarshalString(v)
}

type encodeState struct {
	cfg encoderOptions
	buf encBuffer
}

func (s *encodeState) startLine() {
	if s.buf.Len() > 0 {
		s.buf.WriteByte('\n')
	}
}

func (s *encodeState) emit(line string) {
	s.startLine()
	s.buf.WriteString(line)
}

func (s *encodeState) writeIndent(depth int) {
	if depth > 0 {
		s.buf.WriteSpaces(depth * s.cfg.indentSize)
	}
}

func (s *encodeState) indent(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat(" ", depth*s.cfg.indentSize)
}

func (s *encodeState) encodeRoot(value normalizedValue) error {
	switch val := value.(type) {
	case nil, bool, string, numberValue:
		s.startLine()
		return s.buf.appendPrimitive(val, formatContext{
			active:   s.cfg.delimiter,
			document: s.cfg.delimiter,
			inArray:  false,
		})
	case Object:
		if err := s.encodeObject(val, 0); err != nil {
			return err
		}
	case []normalizedValue:
		if err := s.encodeArray("", val, 0, true); err != nil {
			return err
		}
	default:
		return fmt.Errorf("toon: unsupported root value %T", value)
	}
	return nil
}

func (s *encodeState) encodeObject(obj Object, depth int) error {
	if depth == 0 && obj.IsEmpty() {
		return nil
	}
	if depth == 0 {
		if ok, err := s.encodeKeyedObject(obj, "", depth, false); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	indent := s.indent(depth)
	for _, field := range obj.Fields {
		switch val := field.Value.(type) {
		case nil, bool, string, numberValue:
			keyLiteral, err := encodeKey(field.Key)
			if err != nil {
				return err
			}
			s.startLine()
			s.writeIndent(depth)
			s.buf.WriteString(keyLiteral)
			s.buf.WriteString(": ")
			if err := s.buf.appendPrimitive(val, formatContext{
				active:   s.cfg.delimiter,
				document: s.cfg.delimiter,
				inArray:  false,
			}); err != nil {
				return err
			}
		case Object:
			keyLiteral, err := encodeKey(field.Key)
			if err != nil {
				return err
			}
			if ok, err := s.encodeKeyedObject(val, keyLiteral, depth, false); err != nil {
				return err
			} else if ok {
				continue
			}
			s.emit(indent + keyLiteral + ":")
			if err := s.encodeObject(val, depth+1); err != nil {
				return err
			}
		case []normalizedValue:
			if err := s.encodeArray(field.Key, val, depth, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("toon: unsupported object field %s of type %T", field.Key, val)
		}
	}
	return nil
}

func (s *encodeState) encodeArray(key string, values []normalizedValue, depth int, root bool) error {
	indent := s.indent(depth)
	delimiter := s.cfg.delimiter
	ctx := formatContext{
		active:   delimiter,
		document: delimiter,
		inArray:  true,
	}

	keyLiteral := ""
	var err error
	if key != "" || !root {
		keyLiteral, err = encodeKey(key)
		if err != nil {
			return err
		}
	}

	if isPrimitiveArray(values) {
		if len(values) == 0 {
			if root {
				s.emit(indent + "[]")
			} else {
				s.emit(indent + keyLiteral + ": []")
			}
			return nil
		}
		header := renderHeader(keyLiteral, len(values), delimiter, false, nil)
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteString(header)
		if len(values) > 0 {
			s.buf.WriteByte(' ')
			delimRune := delimiter.rune()
			for i, v := range values {
				if i > 0 {
					s.buf.WriteRune(delimRune)
				}
				if err := s.buf.appendPrimitive(v, ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if fields, ok := detectTabular(values); ok {
		header := renderHeader(keyLiteral, len(values), delimiter, false, fields)
		s.emit(indent + header)
		delimRune := delimiter.rune()
		for _, row := range values {
			obj := row.(Object)
			s.startLine()
			s.writeIndent(depth + 1)
			for i, field := range flattenObjectValues(obj, fields) {
				if i > 0 {
					s.buf.WriteRune(delimRune)
				}
				if err := s.buf.appendPrimitive(field, ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}

	header := renderHeader(keyLiteral, len(values), delimiter, false, nil)
	s.emit(indent + header)
	for _, item := range values {
		if root {
			if err := s.encodeListItem(item, depth+1, ctx); err != nil {
				return err
			}
			continue
		}
		if err := s.encodeArrayItem(item, depth+1, ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *encodeState) encodeArrayItem(item normalizedValue, depth int, ctx formatContext) error {
	switch v := item.(type) {
	case nil, bool, string, numberValue:
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteString("- ")
		return s.buf.appendPrimitive(v, ctx)
	case Object:
		if err := s.encodeObjectListItem(v, depth, ctx); err != nil {
			return err
		}
	case []normalizedValue:
		return s.encodeArrayForObjectListItem("", v, depth, ctx)
	default:
		return fmt.Errorf("toon: unsupported array item %T", v)
	}
	return nil
}

func (s *encodeState) encodeListItem(item normalizedValue, depth int, ctx formatContext) error {
	switch v := item.(type) {
	case nil, bool, string, numberValue:
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteString("- ")
		return s.buf.appendPrimitive(v, ctx)
	case Object:
		if err := s.encodeObjectListItem(v, depth, ctx); err != nil {
			return err
		}
	case []normalizedValue:
		return s.encodeArrayForObjectListItem("", v, depth, ctx)
	default:
		return fmt.Errorf("toon: unsupported list item %T", v)
	}
	return nil
}

func (s *encodeState) encodeObjectListItem(obj Object, depth int, ctx formatContext) error {
	if obj.IsEmpty() {
		s.emit(s.indent(depth) + "-")
		return nil
	}
	first := obj.Fields[0]
	if isPrimitive(first.Value) {
		keyLiteral, err := encodeKey(first.Key)
		if err != nil {
			return err
		}
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteString("- ")
		s.buf.WriteString(keyLiteral)
		s.buf.WriteString(": ")
		if err := s.buf.appendPrimitive(first.Value, ctx); err != nil {
			return err
		}
		if len(obj.Fields) > 1 {
			if err := s.encodeObject(Object{Fields: obj.Fields[1:]}, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if arr, ok := first.Value.([]normalizedValue); ok {
		keyLiteral, err := encodeKey(first.Key)
		if err != nil {
			return err
		}
		if err := s.encodeArrayForObjectListItem(keyLiteral, arr, depth, ctx); err != nil {
			return err
		}
		if len(obj.Fields) > 1 {
			if err := s.encodeObject(Object{Fields: obj.Fields[1:]}, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if keyed, ok := first.Value.(Object); ok {
		keyLiteral, err := encodeKey(first.Key)
		if err != nil {
			return err
		}
		if emitted, err := s.encodeKeyedObject(keyed, keyLiteral, depth, true); err != nil {
			return err
		} else if emitted {
			if len(obj.Fields) > 1 {
				if err := s.encodeObject(Object{Fields: obj.Fields[1:]}, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if nested, ok := first.Value.(Object); ok {
		keyLiteral, err := encodeKey(first.Key)
		if err != nil {
			return err
		}
		s.emit(s.indent(depth) + "- " + keyLiteral + ":")
		if err := s.encodeObject(nested, depth+2); err != nil {
			return err
		}
		if len(obj.Fields) > 1 {
			return s.encodeObject(Object{Fields: obj.Fields[1:]}, depth+1)
		}
		return nil
	}
	s.emit(s.indent(depth) + "-")
	return nil
}

func (s *encodeState) encodeKeyedObject(obj Object, keyLiteral string, depth int, listItem bool) (bool, error) {
	if len(obj.Fields) < 2 {
		return false, nil
	}
	entries := make([]normalizedValue, len(obj.Fields))
	for i, field := range obj.Fields {
		entry, ok := field.Value.(Object)
		if !ok || entry.IsEmpty() {
			return false, nil
		}
		entries[i] = entry
	}
	fields, ok := detectTabular(entries)
	if !ok {
		return false, nil
	}
	header := renderKeyedHeader(keyLiteral, len(entries), s.cfg.delimiter, fields)
	if listItem {
		s.emit(s.indent(depth) + "- " + header)
	} else {
		s.emit(s.indent(depth) + header)
	}
	rowDepth := depth + 1
	if listItem {
		rowDepth++
	}
	for i, field := range obj.Fields {
		entryKey, err := encodeKey(field.Key)
		if err != nil {
			return false, err
		}
		entry := entries[i].(Object)
		s.startLine()
		s.writeIndent(rowDepth)
		s.buf.WriteString(entryKey)
		s.buf.WriteString(": ")
		delimRune := s.cfg.delimiter.rune()
		elemCtx := formatContext{
			active:   s.cfg.delimiter,
			document: s.cfg.delimiter,
			inArray:  true,
		}
		for j, value := range flattenObjectValues(entry, fields) {
			if j > 0 {
				s.buf.WriteRune(delimRune)
			}
			if err := s.buf.appendPrimitive(value, elemCtx); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

func (s *encodeState) encodeArrayForObjectListItem(keyLiteral string, values []normalizedValue, depth int, ctx formatContext) error {
	delimiter := ctx.active
	indent := s.indent(depth)

	// Anonymous nested arrays use list form in a list-item position, even
	// when their elements happen to be tabular-shaped. A fields-bearing
	// header here would change the required §10 layout.
	if keyLiteral != "" {
		if fields, ok := detectTabular(values); ok {
			header := renderHeader(keyLiteral, len(values), delimiter, false, fields)
			s.emit(indent + "- " + header)
			delimRune := delimiter.rune()
			for _, row := range values {
				obj := row.(Object)
				s.startLine()
				s.writeIndent(depth + 2)
				for i, field := range flattenObjectValues(obj, fields) {
					if i > 0 {
						s.buf.WriteRune(delimRune)
					}
					if err := s.buf.appendPrimitive(field, ctx); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	if isPrimitiveArray(values) {
		if len(values) == 0 {
			if keyLiteral != "" {
				s.emit(indent + "- " + keyLiteral + ": []")
			} else {
				s.emit(indent + "- [0]:")
			}
			return nil
		}
		header := renderHeader(keyLiteral, len(values), delimiter, false, nil)
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteString("- ")
		s.buf.WriteString(header)
		if len(values) > 0 {
			s.buf.WriteByte(' ')
			delimRune := delimiter.rune()
			for i, v := range values {
				if i > 0 {
					s.buf.WriteRune(delimRune)
				}
				if err := s.buf.appendPrimitive(v, ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}

	header := renderHeader(keyLiteral, len(values), delimiter, false, nil)
	s.emit(indent + "- " + header)
	childDepth := depth + 1
	if keyLiteral != "" {
		childDepth++
	}
	for _, item := range values {
		if err := s.encodeListItem(item, childDepth, ctx); err != nil {
			return err
		}
	}
	return nil
}

func detectTabular(values []normalizedValue) ([]fieldNode, bool) {
	if len(values) == 0 {
		return nil, false
	}
	first, ok := values[0].(Object)
	if !ok || first.IsEmpty() {
		return nil, false
	}
	fields := make([]fieldNode, len(first.Fields))
	fieldSet := make(map[string]struct{}, len(first.Fields))
	for i, field := range first.Fields {
		column := make([]normalizedValue, 0, len(values))
		for _, value := range values {
			obj, rowOK := value.(Object)
			if !rowOK {
				return nil, false
			}
			column = append(column, objField(obj, field.Key))
		}
		fields[i], ok = detectFieldNode(field.Key, field.Value, column)
		if !ok {
			return nil, false
		}
		fieldSet[field.Key] = struct{}{}
	}
	for _, value := range values[1:] {
		obj, ok := value.(Object)
		if !ok {
			return nil, false
		}
		if len(obj.Fields) != len(fields) {
			return nil, false
		}
		seen := make(map[string]struct{}, len(fields))
		for _, field := range obj.Fields {
			if _, ok := fieldSet[field.Key]; !ok {
				return nil, false
			}
			seen[field.Key] = struct{}{}
		}
		if len(seen) != len(fields) {
			return nil, false
		}
	}
	return fields, true
}

func detectFieldNode(name string, firstValue normalizedValue, rows []normalizedValue) (fieldNode, bool) {
	if isPrimitive(firstValue) {
		for _, value := range rows {
			if !isPrimitive(value) {
				return fieldNode{}, false
			}
		}
		return fieldNode{name: name}, true
	}

	firstObject, ok := firstValue.(Object)
	if !ok || firstObject.IsEmpty() {
		return fieldNode{}, false
	}
	children := make([]fieldNode, len(firstObject.Fields))
	childSet := make(map[string]struct{}, len(firstObject.Fields))
	for i, child := range firstObject.Fields {
		childRows := make([]normalizedValue, 0, len(rows))
		for _, value := range rows {
			obj, ok := value.(Object)
			if !ok {
				return fieldNode{}, false
			}
			if obj.IsEmpty() {
				return fieldNode{}, false
			}
			childRows = append(childRows, objField(obj, child.Key))
		}
		children[i], ok = detectFieldNode(child.Key, child.Value, childRows)
		if !ok {
			return fieldNode{}, false
		}
		childSet[child.Key] = struct{}{}
	}
	for _, value := range rows {
		obj, ok := value.(Object)
		if !ok {
			return fieldNode{}, false
		}
		if len(obj.Fields) != len(childSet) {
			return fieldNode{}, false
		}
		for _, child := range obj.Fields {
			if _, ok := childSet[child.Key]; !ok {
				return fieldNode{}, false
			}
		}
	}
	return fieldNode{name: name, children: children}, true
}

func flattenObjectValues(obj Object, fields []fieldNode) []normalizedValue {
	values := make([]normalizedValue, 0)
	for _, field := range fields {
		value := objField(obj, field.name)
		if len(field.children) == 0 {
			values = append(values, value)
			continue
		}
		nested, _ := value.(Object)
		values = append(values, flattenObjectValues(nested, field.children)...)
	}
	return values
}

func objField(obj Object, key string) normalizedValue {
	for _, field := range obj.Fields {
		if field.Key == key {
			return field.Value
		}
	}
	return nil
}

func isPrimitive(value normalizedValue) bool {
	switch value.(type) {
	case nil, bool, string, numberValue:
		return true
	default:
		return false
	}
}

func isPrimitiveArray(values []normalizedValue) bool {
	for _, v := range values {
		if !isPrimitive(v) {
			return false
		}
	}
	return true
}

func renderHeader(keyLiteral string, length int, delimiter Delimiter, _ bool, fields []fieldNode) string {
	var b strings.Builder
	if keyLiteral != "" {
		b.WriteString(keyLiteral)
	}
	b.WriteByte('[')
	b.WriteString(strconv.Itoa(length))
	if delimiter != DelimiterComma {
		b.WriteRune(delimiter.rune())
	}
	b.WriteByte(']')
	if len(fields) > 0 {
		b.WriteByte('{')
		for i, field := range fields {
			if i > 0 {
				b.WriteRune(delimiter.rune())
			}
			writeFieldNode(&b, field, delimiter)
		}
		b.WriteByte('}')
	}
	b.WriteByte(':')
	return b.String()
}

func renderKeyedHeader(keyLiteral string, length int, delimiter Delimiter, fields []fieldNode) string {
	header := renderHeader(keyLiteral, length, delimiter, false, fields)
	open := strings.IndexByte(header, '[')
	close := strings.IndexByte(header[open+1:], ']') + open + 1
	if delimiter == DelimiterComma {
		return header[:close] + ":" + header[close:]
	}
	return header[:close-1] + ":" + header[close-1:]
}

func writeFieldNode(b *strings.Builder, field fieldNode, delimiter Delimiter) {
	fieldLiteral, _ := encodeKey(field.name)
	b.WriteString(fieldLiteral)
	if len(field.children) == 0 {
		return
	}
	b.WriteByte('{')
	for i, child := range field.children {
		if i > 0 {
			b.WriteRune(delimiter.rune())
		}
		writeFieldNode(b, child, delimiter)
	}
	b.WriteByte('}')
}
