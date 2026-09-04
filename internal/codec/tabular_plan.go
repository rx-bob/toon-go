package codec

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	formatpkg "github.com/toon-format/toon-go/internal/format"
)

var errPlanIneligible = errors.New("toon: plan ineligible")

type rowOpcode uint8

const (
	opInvalid rowOpcode = iota
	opString
	opBool
	opInt
	opUint
	opFloat32
	opFloat64
)

func (op rowOpcode) String() string {
	switch op {
	case opString:
		return "string"
	case opBool:
		return "bool"
	case opInt:
		return "int"
	case opUint:
		return "uint"
	case opFloat32:
		return "float32"
	case opFloat64:
		return "float64"
	default:
		return "invalid"
	}
}

type tabularFieldPlan struct {
	name          string
	encodedHeader string
	index         []int
	flatIndex     int
	kind          reflect.Kind
	bitWidth      int
	op            rowOpcode
}

type tabularRowPlan struct {
	rowType        reflect.Type
	delimiter      Delimiter
	fields         []tabularFieldPlan
	fieldNodes     []fieldNode
	headerLiteral  string
	eligible       bool
	reason         string
	fctx           formatpkg.Context
	stringFields   []int
	rowStaticWidth int
}

func (p *tabularRowPlan) IsEligible() bool {
	return p != nil && p.eligible
}

func (p *tabularRowPlan) Reason() string {
	if p == nil {
		return "nil plan"
	}
	return p.reason
}

type planCacheKey struct {
	rowType   reflect.Type
	delimiter Delimiter
}

var (
	planCache    sync.Map // map[planCacheKey]*tabularRowPlan
	stringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
	timeType     = reflect.TypeOf(time.Time{})
)

func isCustomStringer(t reflect.Type) bool {
	if t.Implements(stringerType) {
		return true
	}
	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(stringerType) {
		return true
	}
	return false
}

func isTimeType(t reflect.Type) bool {
	return t == timeType || t.ConvertibleTo(timeType)
}

func classifyField(t reflect.Type) (rowOpcode, int, bool) {
	if isCustomStringer(t) || isTimeType(t) {
		return opInvalid, 0, false
	}
	switch t.Kind() {
	case reflect.String:
		return opString, 0, true
	case reflect.Bool:
		return opBool, 1, true
	case reflect.Int:
		return opInt, strconv.IntSize, true
	case reflect.Int8:
		return opInt, 8, true
	case reflect.Int16:
		return opInt, 16, true
	case reflect.Int32:
		return opInt, 32, true
	case reflect.Int64:
		return opInt, 64, true
	case reflect.Uint:
		return opUint, strconv.IntSize, true
	case reflect.Uint8:
		return opUint, 8, true
	case reflect.Uint16:
		return opUint, 16, true
	case reflect.Uint32:
		return opUint, 32, true
	case reflect.Uint64:
		return opUint, 64, true
	case reflect.Float32:
		return opFloat32, 32, true
	case reflect.Float64:
		return opFloat64, 64, true
	default:
		return opInvalid, 0, false
	}
}

func buildTabularPlansForStruct(t reflect.Type, meta structMeta) map[Delimiter]*tabularRowPlan {
	plans := make(map[Delimiter]*tabularRowPlan, 3)
	for _, delim := range []Delimiter{DelimiterComma, DelimiterTab, DelimiterPipe} {
		plans[delim] = compileTabularRowPlan(t, meta, delim)
	}
	return plans
}

func compileTabularRowPlan(t reflect.Type, meta structMeta, delim Delimiter) *tabularRowPlan {
	if meta.err != nil {
		return &tabularRowPlan{
			rowType:   t,
			delimiter: delim,
			eligible:  false,
			reason:    meta.err.Error(),
		}
	}
	if len(meta.fields) == 0 {
		return &tabularRowPlan{
			rowType:   t,
			delimiter: delim,
			eligible:  false,
			reason:    "struct has no exported fields",
		}
	}

	fieldPlans := make([]tabularFieldPlan, len(meta.fields))
	fieldNodes := make([]fieldNode, len(meta.fields))
	var stringFields []int

	for i, f := range meta.fields {
		if f.omitEmpty {
			return &tabularRowPlan{
				rowType:   t,
				delimiter: delim,
				eligible:  false,
				reason:    fmt.Sprintf("field %q has omitempty", f.name),
			}
		}

		if len(f.index) == 0 {
			return &tabularRowPlan{
				rowType:   t,
				delimiter: delim,
				eligible:  false,
				reason:    fmt.Sprintf("field %q has empty index path", f.name),
			}
		}

		currType := t
		for _, idx := range f.index {
			if currType.Kind() == reflect.Pointer {
				return &tabularRowPlan{
					rowType:   t,
					delimiter: delim,
					eligible:  false,
					reason:    fmt.Sprintf("field %q index path traverses pointer", f.name),
				}
			}
			if currType.Kind() != reflect.Struct {
				return &tabularRowPlan{
					rowType:   t,
					delimiter: delim,
					eligible:  false,
					reason:    fmt.Sprintf("field %q index path traverses non-struct", f.name),
				}
			}
			if idx < 0 || idx >= currType.NumField() {
				return &tabularRowPlan{
					rowType:   t,
					delimiter: delim,
					eligible:  false,
					reason:    fmt.Sprintf("field %q index %d out of bounds for %v", f.name, idx, currType),
				}
			}
			currType = currType.Field(idx).Type
		}
		fieldType := currType

		if isTimeType(fieldType) {
			return &tabularRowPlan{
				rowType:   t,
				delimiter: delim,
				eligible:  false,
				reason:    fmt.Sprintf("field %q is time type", f.name),
			}
		}
		if isCustomStringer(fieldType) {
			return &tabularRowPlan{
				rowType:   t,
				delimiter: delim,
				eligible:  false,
				reason:    fmt.Sprintf("field %q implements fmt.Stringer", f.name),
			}
		}

		op, bitWidth, ok := classifyField(fieldType)
		if !ok {
			return &tabularRowPlan{
				rowType:   t,
				delimiter: delim,
				eligible:  false,
				reason:    fmt.Sprintf("field %q has unsupported type %v (kind %v)", f.name, fieldType, fieldType.Kind()),
			}
		}

		encodedKey, err := encodeKey(f.name)
		if err != nil {
			return &tabularRowPlan{
				rowType:   t,
				delimiter: delim,
				eligible:  false,
				reason:    fmt.Sprintf("field %q key cannot be encoded: %v", f.name, err),
			}
		}

		flatIdx := -1
		if len(f.index) == 1 {
			flatIdx = f.index[0]
		}
		if op == opString {
			stringFields = append(stringFields, flatIdx)
		}

		fieldPlans[i] = tabularFieldPlan{
			name:          f.name,
			encodedHeader: encodedKey,
			index:         f.index,
			flatIndex:     flatIdx,
			kind:          fieldType.Kind(),
			bitWidth:      bitWidth,
			op:            op,
		}
		fieldNodes[i] = fieldNode{name: f.name}
	}

	var hb strings.Builder
	hb.WriteByte('{')
	for i, fp := range fieldPlans {
		if i > 0 {
			hb.WriteRune(delim.rune())
		}
		hb.WriteString(fp.encodedHeader)
	}
	hb.WriteString("}:")

	fctx := formatpkg.Context{
		Active:   delim.rune(),
		Document: delim.rune(),
		InArray:  true,
	}

	var staticWidth int
	for _, fp := range fieldPlans {
		switch fp.op {
		case opBool:
			staticWidth += 5
		case opInt, opUint:
			staticWidth += 12
		case opFloat32, opFloat64:
			staticWidth += 12
		}
	}
	if len(fieldPlans) > 1 {
		delimLen := len(string(delim.rune()))
		staticWidth += (len(fieldPlans) - 1) * delimLen
	}
	staticWidth += 1 + 2 // newline + indent

	return &tabularRowPlan{
		rowType:        t,
		delimiter:      delim,
		fields:         fieldPlans,
		fieldNodes:     fieldNodes,
		headerLiteral:  hb.String(),
		eligible:       true,
		fctx:           fctx,
		stringFields:   stringFields,
		rowStaticWidth: staticWidth,
	}
}

// preflightTabularSlice performs a read-only validation and sizing pass over candidate
// tabular rows before any destination mutation occurs.
// Returns:
//   - ok: true if eligible for fast-path encoding, false if ineligible (signals fallback to generic normalization).
//   - estBytes: conservative buffer capacity hint.
//   - err: non-nil if a real validation error is encountered (e.g. invalid UTF-8).
func preflightTabularSlice(val reflect.Value, plan *tabularRowPlan) (bool, int, error) {
	if plan == nil || !plan.eligible {
		return false, 0, nil
	}
	if !val.IsValid() {
		return false, 0, nil
	}
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return false, 0, nil
	}
	n := val.Len()
	if n == 0 {
		return false, 0, nil
	}
	if val.Type().Elem() != plan.rowType {
		return false, 0, nil
	}

	var totalStringBytes int
	if len(plan.stringFields) > 0 {
		for r := 0; r < n; r++ {
			rowVal := val.Index(r)
			for _, flatIdx := range plan.stringFields {
				s := rowVal.Field(flatIdx).String()
				if !utf8.ValidString(s) {
					return false, 0, fmt.Errorf("toon: string is not valid UTF-8")
				}
				totalStringBytes += len(s)
				if len(s) == 0 || s[0] == ' ' || s[len(s)-1] == ' ' {
					totalStringBytes += 2
				}
			}
		}
	}

	headerEst := len(plan.headerLiteral) + 128
	estBytes := headerEst + n*plan.rowStaticWidth + totalStringBytes
	estBytes += (estBytes / 20) + 64 // 5% conservative headroom
	return true, estBytes, nil
}

func (p *tabularRowPlan) validateSlice(sliceVal reflect.Value) error {
	ok, _, err := preflightTabularSlice(sliceVal, p)
	if err != nil {
		return err
	}
	if !ok {
		return errPlanIneligible
	}
	return nil
}

func (p *tabularRowPlan) appendRow(b *encBuffer, rowVal reflect.Value) error {
	delimRune := p.delimiter.rune()
	for i := 0; i < len(p.fields); i++ {
		fp := &p.fields[i]
		if i > 0 {
			b.WriteRune(delimRune)
		}
		fieldVal := rowVal.Field(fp.flatIndex)
		switch fp.op {
		case opString:
			s := fieldVal.String()
			var err error
			b.buf, err = formatpkg.AppendFormatString(b.buf, s, p.fctx)
			if err != nil {
				return err
			}
		case opBool:
			if fieldVal.Bool() {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
		case opInt:
			val := fieldVal.Int()
			if val > maxSafeInteger || val < -maxSafeInteger {
				b.WriteByte('"')
				b.buf = strconv.AppendInt(b.buf, val, 10)
				b.WriteByte('"')
			} else {
				b.buf = strconv.AppendInt(b.buf, val, 10)
			}
		case opUint:
			val := fieldVal.Uint()
			if val > maxSafeInteger {
				b.WriteByte('"')
				b.buf = strconv.AppendUint(b.buf, val, 10)
				b.WriteByte('"')
			} else {
				b.buf = strconv.AppendUint(b.buf, val, 10)
			}
		case opFloat32:
			f := float64(float32(fieldVal.Float()))
			b.buf = p.appendFloat(b.buf, f)
		case opFloat64:
			f := fieldVal.Float()
			b.buf = p.appendFloat(b.buf, f)
		}
	}
	return nil
}

func (p *tabularRowPlan) appendFloat(dst []byte, f float64) []byte {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return append(dst, "null"...)
	}
	if f == math.Copysign(0, -1) {
		return append(dst, '0')
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.AppendFloat(dst, f, 'f', -1, 64)
	}
	return append(dst, formatpkg.FormatNumber(f)...)
}

func (p *tabularRowPlan) appendRows(
	b *encBuffer,
	sliceVal reflect.Value,
	keyLiteral string,
	depth int,
	indentSize int,
	listItem bool,
) (bool, error) {
	if !p.eligible || !sliceVal.IsValid() || sliceVal.Len() == 0 {
		return false, nil
	}

	length := sliceVal.Len()

	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	if depth > 0 {
		b.WriteSpaces(depth * indentSize)
	}
	if listItem {
		b.WriteString("- ")
	}
	if keyLiteral != "" {
		b.WriteString(keyLiteral)
	}
	b.WriteByte('[')
	b.buf = strconv.AppendInt(b.buf, int64(length), 10)
	if p.delimiter != DelimiterComma {
		b.WriteRune(p.delimiter.rune())
	}
	b.WriteByte(']')
	b.WriteString(p.headerLiteral)

	rowDepth := depth + 1
	if listItem {
		rowDepth++
	}
	rowSpaces := rowDepth * indentSize

	for r := 0; r < length; r++ {
		b.WriteByte('\n')
		if rowSpaces > 0 {
			b.WriteSpaces(rowSpaces)
		}
		rowVal := sliceVal.Index(r)
		if err := p.appendRow(b, rowVal); err != nil {
			return false, err
		}
	}

	return true, nil
}

func cachedTabularRowPlan(t reflect.Type, delim Delimiter) *tabularRowPlan {
	if t == nil {
		return &tabularRowPlan{rowType: nil, delimiter: delim, eligible: false, reason: "nil type"}
	}
	if t.Kind() == reflect.Struct {
		meta := cachedStructMeta(t)
		if plan := meta.tabularPlan(delim); plan != nil {
			return plan
		}
	}
	key := planCacheKey{rowType: t, delimiter: delim}
	if v, ok := planCache.Load(key); ok {
		return v.(*tabularRowPlan)
	}
	var plan *tabularRowPlan
	if t.Kind() != reflect.Struct {
		plan = &tabularRowPlan{
			rowType:   t,
			delimiter: delim,
			eligible:  false,
			reason:    fmt.Sprintf("unsupported row kind %v", t.Kind()),
		}
	} else {
		meta := cachedStructMeta(t)
		plan = compileTabularRowPlan(t, meta, delim)
	}
	planCache.Store(key, plan)
	return plan
}

func cachedTabularRowPlanForOptions(t reflect.Type, opts encoderOptions) *tabularRowPlan {
	return cachedTabularRowPlan(t, opts.delimiter)
}
