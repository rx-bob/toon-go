package codec

import (
	"fmt"
	"reflect"
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
	if val := reflect.ValueOf(v); val.IsValid() {
		for val.Kind() == reflect.Pointer {
			if val.IsNil() {
				break
			}
			val = val.Elem()
		}
		if (val.Kind() == reflect.Slice || val.Kind() == reflect.Array) && val.Type().Elem().Kind() == reflect.Struct {
			plan := cachedTabularRowPlan(val.Type().Elem(), e.cfg.delimiter)
			if plan.IsEligible() {
				ok, estBytes, preflightErr := preflightTabularSlice(val, plan)
				if preflightErr != nil {
					return nil, preflightErr
				}
				if ok {
					state := &encodeState{
						cfg: e.cfg,
						buf: newEncBuffer(estBytes),
					}
					ok, err := plan.appendRows(&state.buf, val, "", 0, e.cfg.indentSize, false)
					if err != nil {
						return nil, err
					}
					if ok {
						return state.buf.Bytes(), nil
					}
				}
			}
		}
	}

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
	if val := reflect.ValueOf(v); val.IsValid() {
		for val.Kind() == reflect.Pointer {
			if val.IsNil() {
				break
			}
			val = val.Elem()
		}
		if (val.Kind() == reflect.Slice || val.Kind() == reflect.Array) && val.Type().Elem().Kind() == reflect.Struct {
			plan := cachedTabularRowPlan(val.Type().Elem(), e.cfg.delimiter)
			if plan.IsEligible() {
				ok, estBytes, preflightErr := preflightTabularSlice(val, plan)
				if preflightErr != nil {
					return "", preflightErr
				}
				if ok {
					state := &encodeState{
						cfg: e.cfg,
						buf: newEncBuffer(estBytes),
					}
					ok, err := plan.appendRows(&state.buf, val, "", 0, e.cfg.indentSize, false)
					if err != nil {
						return "", err
					}
					if ok {
						return state.buf.String(), nil
					}
				}
			}
		}
	}

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
	case rawTabularSlice:
		if _, err := val.plan.appendRows(&s.buf, val.val, "", 0, s.cfg.indentSize, false); err != nil {
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
			s.startLine()
			s.writeIndent(depth)
			s.buf.WriteString(keyLiteral)
			s.buf.WriteByte(':')
			if err := s.encodeObject(val, depth+1); err != nil {
				return err
			}
		case []normalizedValue:
			if err := s.encodeArray(field.Key, val, depth, false); err != nil {
				return err
			}
		case rawTabularSlice:
			keyLiteral, err := encodeKey(field.Key)
			if err != nil {
				return err
			}
			if _, err := val.plan.appendRows(&s.buf, val.val, keyLiteral, depth, s.cfg.indentSize, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("toon: unsupported object field %s of type %T", field.Key, val)
		}
	}
	return nil
}

func (s *encodeState) encodeArray(key string, values []normalizedValue, depth int, root bool) error {
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
			s.startLine()
			s.writeIndent(depth)
			if root {
				s.buf.WriteString("[]")
			} else {
				s.buf.WriteString(keyLiteral)
				s.buf.WriteString(": []")
			}
			return nil
		}
		s.startLine()
		s.writeIndent(depth)
		s.buf.appendHeader(keyLiteral, len(values), delimiter, nil)
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

	if layout, ok := compileTabularLayout(values); ok {
		s.startLine()
		s.writeIndent(depth)
		s.buf.appendHeader(keyLiteral, len(values), delimiter, layout.fields)
		delimRune := delimiter.rune()
		for rowIndex, row := range values {
			obj := row.(Object)
			s.startLine()
			s.writeIndent(depth + 1)
			if err := layout.appendRow(&s.buf, obj, rowIndex, ctx, delimRune); err != nil {
				return err
			}
		}
		return nil
	}

	s.startLine()
	s.writeIndent(depth)
	s.buf.appendHeader(keyLiteral, len(values), delimiter, nil)
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
	case rawTabularSlice:
		_, err := v.plan.appendRows(&s.buf, v.val, "", depth, s.cfg.indentSize, true)
		return err
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
	case rawTabularSlice:
		_, err := v.plan.appendRows(&s.buf, v.val, "", depth, s.cfg.indentSize, true)
		return err
	default:
		return fmt.Errorf("toon: unsupported list item %T", v)
	}
	return nil
}

func (s *encodeState) encodeObjectListItem(obj Object, depth int, ctx formatContext) error {
	if obj.IsEmpty() {
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteByte('-')
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
	if raw, ok := first.Value.(rawTabularSlice); ok {
		keyLiteral, err := encodeKey(first.Key)
		if err != nil {
			return err
		}
		if _, err := raw.plan.appendRows(&s.buf, raw.val, keyLiteral, depth, s.cfg.indentSize, true); err != nil {
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
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteString("- ")
		s.buf.WriteString(keyLiteral)
		s.buf.WriteByte(':')
		if err := s.encodeObject(nested, depth+2); err != nil {
			return err
		}
		if len(obj.Fields) > 1 {
			return s.encodeObject(Object{Fields: obj.Fields[1:]}, depth+1)
		}
		return nil
	}
	s.startLine()
	s.writeIndent(depth)
	s.buf.WriteByte('-')
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
	layout, ok := compileTabularLayout(entries)
	if !ok {
		return false, nil
	}
	s.startLine()
	s.writeIndent(depth)
	if listItem {
		s.buf.WriteString("- ")
	}
	s.buf.appendKeyedHeader(keyLiteral, len(entries), s.cfg.delimiter, layout.fields)
	rowDepth := depth + 1
	if listItem {
		rowDepth++
	}
	delimRune := s.cfg.delimiter.rune()
	elemCtx := formatContext{
		active:   s.cfg.delimiter,
		document: s.cfg.delimiter,
		inArray:  true,
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
		if err := layout.appendRow(&s.buf, entry, i, elemCtx, delimRune); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *encodeState) encodeArrayForObjectListItem(keyLiteral string, values []normalizedValue, depth int, ctx formatContext) error {
	delimiter := ctx.active

	// Anonymous nested arrays use list form in a list-item position, even
	// when their elements happen to be tabular-shaped. A fields-bearing
	// header here would change the required §10 layout.
	if keyLiteral != "" {
		if layout, ok := compileTabularLayout(values); ok {
			s.startLine()
			s.writeIndent(depth)
			s.buf.WriteString("- ")
			s.buf.appendHeader(keyLiteral, len(values), delimiter, layout.fields)
			delimRune := delimiter.rune()
			for rowIndex, row := range values {
				obj := row.(Object)
				s.startLine()
				s.writeIndent(depth + 2)
				if err := layout.appendRow(&s.buf, obj, rowIndex, ctx, delimRune); err != nil {
					return err
				}
			}
			return nil
		}
	}

	if isPrimitiveArray(values) {
		if len(values) == 0 {
			s.startLine()
			s.writeIndent(depth)
			s.buf.WriteString("- ")
			if keyLiteral != "" {
				s.buf.WriteString(keyLiteral)
				s.buf.WriteString(": []")
			} else {
				s.buf.WriteString("[0]:")
			}
			return nil
		}
		s.startLine()
		s.writeIndent(depth)
		s.buf.WriteString("- ")
		s.buf.appendHeader(keyLiteral, len(values), delimiter, nil)
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

	s.startLine()
	s.writeIndent(depth)
	s.buf.WriteString("- ")
	s.buf.appendHeader(keyLiteral, len(values), delimiter, nil)
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
	layout, ok := compileTabularLayout(values)
	if !ok {
		return nil, false
	}
	return layout.fields, true
}

func compileTabularLayout(values []normalizedValue) (*tabularLayout, bool) {
	if len(values) == 0 {
		return nil, false
	}
	first, ok := values[0].(Object)
	if !ok {
		return nil, false
	}
	layout, ok := buildTabularLayout(first)
	if !ok {
		return nil, false
	}

	var rowMappings [][]int
	hasReordered := false

	for i := 1; i < len(values); i++ {
		obj, ok := values[i].(Object)
		if !ok {
			return nil, false
		}
		mapping, valid := layout.validateRow(obj)
		if !valid {
			return nil, false
		}
		if mapping != nil {
			if !hasReordered {
				rowMappings = make([][]int, len(values))
				hasReordered = true
			}
			rowMappings[i] = mapping
		}
	}

	if hasReordered {
		layout.rowMappings = rowMappings
	}
	return layout, true
}

func buildTabularLayout(first Object) (*tabularLayout, bool) {
	if first.IsEmpty() {
		return nil, false
	}
	cols := make([]tabularColumn, len(first.Fields))
	nodes := make([]fieldNode, len(first.Fields))

	for i := 0; i < len(first.Fields); i++ {
		key := first.Fields[i].Key
		for j := 0; j < i; j++ {
			if first.Fields[j].Key == key {
				return nil, false
			}
		}
		val := first.Fields[i].Value
		if isPrimitive(val) {
			cols[i] = tabularColumn{name: key, isNested: false}
			nodes[i] = fieldNode{name: key}
		} else if obj, ok := val.(Object); ok {
			childLayout, childOK := buildTabularLayout(obj)
			if !childOK {
				return nil, false
			}
			cols[i] = tabularColumn{name: key, isNested: true, childLayout: childLayout}
			nodes[i] = fieldNode{name: key, children: childLayout.fields}
		} else {
			return nil, false
		}
	}
	return &tabularLayout{
		fields:  nodes,
		columns: cols,
	}, true
}

func (l *tabularLayout) validateRow(obj Object) ([]int, bool) {
	n := len(l.columns)
	if len(obj.Fields) != n {
		return nil, false
	}

	stable := true
	for i := 0; i < n; i++ {
		if obj.Fields[i].Key != l.columns[i].name {
			stable = false
			break
		}
	}

	if stable {
		for i := 0; i < n; i++ {
			col := &l.columns[i]
			val := obj.Fields[i].Value
			if !col.isNested {
				if !isPrimitive(val) {
					return nil, false
				}
			} else {
				childObj, ok := val.(Object)
				if !ok || childObj.IsEmpty() {
					return nil, false
				}
				if _, childOK := col.childLayout.validateRow(childObj); !childOK {
					return nil, false
				}
			}
		}
		return nil, true
	}

	mapping := make([]int, n)
	if n <= 64 {
		var seen uint64
		for j, f := range obj.Fields {
			colIdx := -1
			for k := 0; k < n; k++ {
				if l.columns[k].name == f.Key {
					colIdx = k
					break
				}
			}
			if colIdx < 0 {
				return nil, false
			}
			if seen&(uint64(1)<<colIdx) != 0 {
				return nil, false
			}
			seen |= uint64(1) << colIdx
			mapping[colIdx] = j
		}
		if seen != (uint64(1)<<n)-1 {
			return nil, false
		}
	} else {
		seen := make([]bool, n)
		for j, f := range obj.Fields {
			colIdx := -1
			for k := 0; k < n; k++ {
				if l.columns[k].name == f.Key {
					colIdx = k
					break
				}
			}
			if colIdx < 0 || seen[colIdx] {
				return nil, false
			}
			seen[colIdx] = true
			mapping[colIdx] = j
		}
	}

	for colIdx := 0; colIdx < n; colIdx++ {
		col := &l.columns[colIdx]
		val := obj.Fields[mapping[colIdx]].Value
		if !col.isNested {
			if !isPrimitive(val) {
				return nil, false
			}
		} else {
			childObj, ok := val.(Object)
			if !ok || childObj.IsEmpty() {
				return nil, false
			}
			if _, childOK := col.childLayout.validateRow(childObj); !childOK {
				return nil, false
			}
		}
	}
	return mapping, true
}

func (l *tabularLayout) appendRow(b *encBuffer, obj Object, rowIndex int, ctx formatContext, delimRune rune) error {
	var mapping []int
	if l.rowMappings != nil && rowIndex < len(l.rowMappings) {
		mapping = l.rowMappings[rowIndex]
	}
	_, err := l.appendColumns(b, obj, mapping, ctx, delimRune, true)
	return err
}

func (l *tabularLayout) appendColumns(b *encBuffer, obj Object, mapping []int, ctx formatContext, delimRune rune, isFirst bool) (bool, error) {
	n := len(l.columns)
	for colIdx := 0; colIdx < n; colIdx++ {
		col := &l.columns[colIdx]
		var val normalizedValue
		if mapping == nil {
			if colIdx >= len(obj.Fields) || obj.Fields[colIdx].Key != col.name {
				return isFirst, fmt.Errorf("toon: field %q not found in row", col.name)
			}
			val = obj.Fields[colIdx].Value
		} else {
			if colIdx >= len(mapping) {
				return isFirst, fmt.Errorf("toon: column index %d out of mapping bounds", colIdx)
			}
			fieldIdx := mapping[colIdx]
			if fieldIdx < 0 || fieldIdx >= len(obj.Fields) || obj.Fields[fieldIdx].Key != col.name {
				return isFirst, fmt.Errorf("toon: field %q not found in row via mapping", col.name)
			}
			val = obj.Fields[fieldIdx].Value
		}

		if !col.isNested {
			if !isFirst {
				b.WriteRune(delimRune)
			}
			isFirst = false
			if err := b.appendPrimitive(val, ctx); err != nil {
				return isFirst, err
			}
		} else {
			childObj, ok := val.(Object)
			if !ok {
				return isFirst, fmt.Errorf("toon: expected nested object for field %q, got %T", col.name, val)
			}
			var childMapping []int
			childN := len(col.childLayout.columns)
			if len(childObj.Fields) != childN {
				return isFirst, fmt.Errorf("toon: nested object %q has %d fields, want %d", col.name, len(childObj.Fields), childN)
			}
			childStable := true
			for ci := 0; ci < childN; ci++ {
				if childObj.Fields[ci].Key != col.childLayout.columns[ci].name {
					childStable = false
					break
				}
			}
			if !childStable {
				childMapping = make([]int, childN)
				for ci := 0; ci < childN; ci++ {
					targetKey := col.childLayout.columns[ci].name
					found := -1
					for fj, f := range childObj.Fields {
						if f.Key == targetKey {
							found = fj
							break
						}
					}
					if found < 0 {
						return isFirst, fmt.Errorf("toon: field %q not found in nested object %q", targetKey, col.name)
					}
					childMapping[ci] = found
				}
			}
			var err error
			isFirst, err = col.childLayout.appendColumns(b, childObj, childMapping, ctx, delimRune, isFirst)
			if err != nil {
				return isFirst, err
			}
		}
	}
	return isFirst, nil
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

func (b *encBuffer) appendHeader(keyLiteral string, length int, delimiter Delimiter, fields []fieldNode) {
	if keyLiteral != "" {
		b.WriteString(keyLiteral)
	}
	b.WriteByte('[')
	b.buf = strconv.AppendInt(b.buf, int64(length), 10)
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
			b.appendFieldNode(field, delimiter)
		}
		b.WriteByte('}')
	}
	b.WriteByte(':')
}

func (b *encBuffer) appendKeyedHeader(keyLiteral string, length int, delimiter Delimiter, fields []fieldNode) {
	if keyLiteral != "" {
		b.WriteString(keyLiteral)
	}
	b.WriteByte('[')
	b.buf = strconv.AppendInt(b.buf, int64(length), 10)
	b.WriteByte(':')
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
			b.appendFieldNode(field, delimiter)
		}
		b.WriteByte('}')
	}
	b.WriteByte(':')
}

func (b *encBuffer) appendFieldNode(field fieldNode, delimiter Delimiter) {
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
		b.appendFieldNode(child, delimiter)
	}
	b.WriteByte('}')
}

func renderHeader(keyLiteral string, length int, delimiter Delimiter, _ bool, fields []fieldNode) string {
	var b encBuffer
	b.appendHeader(keyLiteral, length, delimiter, fields)
	return b.String()
}

func renderKeyedHeader(keyLiteral string, length int, delimiter Delimiter, fields []fieldNode) string {
	var b encBuffer
	b.appendKeyedHeader(keyLiteral, length, delimiter, fields)
	return b.String()
}
