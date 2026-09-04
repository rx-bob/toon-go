package codec

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
	rowType       reflect.Type
	delimiter     Delimiter
	fields        []tabularFieldPlan
	fieldNodes    []fieldNode
	headerLiteral string
	eligible      bool
	reason        string
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

	return &tabularRowPlan{
		rowType:       t,
		delimiter:     delim,
		fields:        fieldPlans,
		fieldNodes:    fieldNodes,
		headerLiteral: hb.String(),
		eligible:      true,
	}
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
