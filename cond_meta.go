package fate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// CondMeta describes, for tooling, which context fields a transition's Guard
// checks. It never affects whether the transition fires: Guard remains the only
// runtime predicate. Describe copies it into TransitionDescriptor so a viewer
// can show each condition and evaluate it against a live context.
type CondMeta struct {
	Fields []CondField `json:"fields,omitempty"`
	// Sample is an example context, as a JSON object, that passes the guard.
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
	// Path selects a context field with a dot path rooted at "$", such as
	// "$.score" or "$.customer.tier", resolved against the context's JSON form.
	Path string `json:"path"`
	Op   CondOp `json:"op"`
	// Value is the operand. It is a non-empty slice for CondIn and unset for
	// CondTruthy and CondFalsy.
	Value any `json:"value,omitempty"`
	// Label replaces the default "path op value" text in a viewer.
	Label string `json:"label,omitempty"`
}

// GatesBuilder collects CondFields into a CondMeta. Start one with Gates.
type GatesBuilder struct {
	fields []CondField
}

// Gates starts a CondMeta from the given fields:
//
//	TransitionConfig[Ctx, Evt]{
//		Guard: approved,
//		CondMeta: fate.Gates(
//			fate.Field("$.score").Gte(60),
//			fate.Field("$.status").Eq("approved"),
//		).Sample(`{"score": 65, "status": "approved"}`),
//	}
func Gates(fields ...CondField) *GatesBuilder {
	return &GatesBuilder{fields: fields}
}

// Sample returns the CondMeta with rawJSON attached as its sample context.
// CreateMachine rejects a sample that is not a JSON object.
func (b *GatesBuilder) Sample(rawJSON string) *CondMeta {
	return &CondMeta{Fields: b.fields, Sample: json.RawMessage(rawJSON)}
}

// Build returns the CondMeta without a sample.
func (b *GatesBuilder) Build() *CondMeta {
	return &CondMeta{Fields: b.fields}
}

// CondFieldBuilder builds one CondField. Start one with Field.
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
func (f *CondFieldBuilder) In(values ...any) CondField { return f.build(CondIn, values) }

// Truthy checks that the field is set to a non-zero, non-empty value.
func (f *CondFieldBuilder) Truthy() CondField { return f.build(CondTruthy, nil) }

// Falsy checks that the field is absent, null, zero, empty, or false.
func (f *CondFieldBuilder) Falsy() CondField { return f.build(CondFalsy, nil) }

func validateCondMeta(where string, m *CondMeta) error {
	if m == nil {
		return nil
	}
	for i, f := range m.Fields {
		if !validCondPath(f.Path) {
			return fmt.Errorf("%w: %s cond field %d has path %q, want \"$\" or \"$.a.b\"", ErrInvalidConfig, where, i, f.Path)
		}
		switch f.Op {
		case CondEq, CondNeq, CondGt, CondGte, CondLt, CondLte, CondTruthy, CondFalsy:
		case CondIn:
			if v := reflect.ValueOf(f.Value); (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) || v.Len() == 0 {
				return fmt.Errorf("%w: %s cond field %q uses %q with no values", ErrInvalidConfig, where, f.Path, f.Op)
			}
		default:
			return fmt.Errorf("%w: %s cond field %q has unknown op %q", ErrInvalidConfig, where, f.Path, f.Op)
		}
		if _, err := json.Marshal(f.Value); err != nil {
			return fmt.Errorf("%w: %s cond field %q value: %v", ErrInvalidConfig, where, f.Path, err)
		}
	}
	if len(m.Sample) > 0 {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(m.Sample, &obj); err != nil || obj == nil {
			return fmt.Errorf("%w: %s cond sample is not a JSON object", ErrInvalidConfig, where)
		}
	}
	return nil
}

func validCondPath(p string) bool {
	if p == "$" {
		return true
	}
	rest, ok := strings.CutPrefix(p, "$.")
	if !ok {
		return false
	}
	for _, seg := range strings.Split(rest, ".") {
		if seg == "" || strings.ContainsAny(seg, " \t[]") {
			return false
		}
	}
	return true
}
