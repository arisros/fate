package fate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// CondMeta describes, for tooling, which context fields a transition's Guard checks.
type CondMeta struct {
	Fields []CondField     `json:"fields,omitempty"`
	Sample json.RawMessage `json:"sample,omitempty"`
}

// CondOp is the comparison a CondField applies.
type CondOp string

// Supported CondField operators.
const (
	CondEq     CondOp = "eq"
	CondNeq    CondOp = "neq"
	CondGt     CondOp = "gt"
	CondGte    CondOp = "gte"
	CondLt     CondOp = "lt"
	CondLte    CondOp = "lte"
	CondIn     CondOp = "in"
	CondTruthy CondOp = "truthy"
	CondFalsy  CondOp = "falsy"
)

// CondField is one condition a Guard checks on the context.
type CondField struct {
	Path  string `json:"path"`
	Op    CondOp `json:"op"`
	Value any    `json:"value"`
	Label string `json:"label,omitempty"`
}

// GatesBuilder collects CondFields into a CondMeta.
type GatesBuilder struct {
	fields []CondField
}

// Gates starts a CondMeta from the given fields:
func Gates(fields ...CondField) *GatesBuilder {
	return &GatesBuilder{fields: fields}
}

// Sample returns the CondMeta with rawJSON attached as its sample context.
func (b *GatesBuilder) Sample(rawJSON string) *CondMeta {
	return &CondMeta{Fields: b.fields, Sample: json.RawMessage(rawJSON)}
}

// Build returns the CondMeta without a sample.
func (b *GatesBuilder) Build() *CondMeta {
	return &CondMeta{Fields: b.fields}
}

// CondFieldBuilder builds one CondField.
type CondFieldBuilder struct {
	path  string
	label string
}

// Field starts a CondField for the given "$"-rooted dot path.
func Field(path string) *CondFieldBuilder {
	return &CondFieldBuilder{path: path}
}

// WithLabel sets the field's display label.
func (f *CondFieldBuilder) WithLabel(label string) *CondFieldBuilder {
	f.label = label
	return f
}

func (f *CondFieldBuilder) build(op CondOp, value any) CondField {
	return CondField{Path: f.path, Op: op, Value: value, Label: f.label}
}

// Eq checks that the field equals value.
func (f *CondFieldBuilder) Eq(value any) CondField { return f.build(CondEq, value) }

// Neq checks that the field does not equal value.
func (f *CondFieldBuilder) Neq(value any) CondField { return f.build(CondNeq, value) }

// Gt checks that the field is numerically greater than value.
func (f *CondFieldBuilder) Gt(value any) CondField { return f.build(CondGt, value) }

// Gte checks that the field is numerically greater than or equal to value.
func (f *CondFieldBuilder) Gte(value any) CondField { return f.build(CondGte, value) }

// Lt checks that the field is numerically less than value.
func (f *CondFieldBuilder) Lt(value any) CondField { return f.build(CondLt, value) }

// Lte checks that the field is numerically less than or equal to value.
func (f *CondFieldBuilder) Lte(value any) CondField { return f.build(CondLte, value) }

// In checks that the field equals one of values.
func (f *CondFieldBuilder) In(values ...any) CondField {
	if len(values) == 1 {
		if v := reflect.ValueOf(values[0]); v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			return f.build(CondIn, values[0])
		}
	}
	return f.build(CondIn, values)
}

// Truthy checks that the field is set to a non-zero, non-empty value.
func (f *CondFieldBuilder) Truthy() CondField { return f.build(CondTruthy, nil) }

// Falsy checks that the field is absent, null, zero, empty, or false.
func (f *CondFieldBuilder) Falsy() CondField { return f.build(CondFalsy, nil) }

func (m *CondMeta) seal(where string) (*CondMeta, error) {
	if m == nil {
		return nil, nil
	}
	out := &CondMeta{Fields: make([]CondField, 0, len(m.Fields))}
	for i, f := range m.Fields {
		if !validCondPath(f.Path) {
			return nil, fmt.Errorf("%w: %s cond field %d has path %q, want \"$.key\" or \"$.key.0\"", ErrInvalidConfig, where, i, f.Path)
		}
		if err := checkOperand(f); err != nil {
			return nil, fmt.Errorf("%w: %s cond field %q: %v", ErrInvalidConfig, where, f.Path, err)
		}
		if f.Value != nil {
			raw, err := json.Marshal(f.Value)
			if err != nil {
				return nil, fmt.Errorf("%w: %s cond field %q value: %v", ErrInvalidConfig, where, f.Path, err)
			}
			f.Value = json.RawMessage(raw)
		}
		out.Fields = append(out.Fields, f)
	}
	if len(m.Sample) > 0 {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(m.Sample, &obj); err != nil || obj == nil {
			return nil, fmt.Errorf("%w: %s cond sample is not a JSON object", ErrInvalidConfig, where)
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, m.Sample); err != nil {
			return nil, fmt.Errorf("%w: %s cond sample: %v", ErrInvalidConfig, where, err)
		}
		out.Sample = buf.Bytes()
	}
	return out, nil
}

func (m *CondMeta) clone() *CondMeta {
	if m == nil {
		return nil
	}
	out := &CondMeta{Fields: make([]CondField, len(m.Fields)), Sample: cloneRaw(m.Sample)}
	for i, f := range m.Fields {
		if raw, ok := f.Value.(json.RawMessage); ok {
			f.Value = cloneRaw(raw)
		}
		out.Fields[i] = f
	}
	return out
}

func checkOperand(f CondField) error {
	v := reflect.ValueOf(f.Value)
	switch f.Op {
	case CondEq, CondNeq:
		return nil
	case CondGt, CondGte, CondLt, CondLte:
		if _, ok := f.Value.(json.Number); ok {
			return nil
		}
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return nil
		}
		return fmt.Errorf("op %q needs a number, got %T", f.Op, f.Value)
	case CondIn:
		if (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) || v.Len() == 0 {
			return fmt.Errorf("op %q needs a non-empty list", f.Op)
		}
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			return fmt.Errorf("op %q got %T, which marshals as a string", f.Op, f.Value)
		}
		return nil
	case CondTruthy, CondFalsy:
		if f.Value != nil {
			return fmt.Errorf("op %q takes no value", f.Op)
		}
		return nil
	default:
		return fmt.Errorf("unknown op %q", f.Op)
	}
}

func validCondPath(p string) bool {
	rest, ok := strings.CutPrefix(p, "$.")
	if !ok {
		return false
	}
	for _, seg := range strings.Split(rest, ".") {
		if seg == "" {
			return false
		}
		for _, r := range seg {
			if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(`[]*$"'\`, r) {
				return false
			}
		}
	}
	return true
}
