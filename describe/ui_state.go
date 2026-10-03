package describe

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// UIState projects the context into a view model for one state, together with a JSON Schema of that view model.
type UIState[Ctx any] struct {
	fn     func(Ctx) any
	schema json.RawMessage
}

// UIStateOf wraps fn and derives a JSON Schema for U by reflection, once, at call time:
func UIStateOf[Ctx any, U any](fn func(Ctx) U) *UIState[Ctx] {
	s := schemaFor(reflect.TypeFor[U](), false, map[reflect.Type]bool{})
	schema, err := json.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("fate: UIStateOf schema: %v", err))
	}
	return &UIState[Ctx]{
		fn:     func(c Ctx) any { return fn(c) },
		schema: schema,
	}
}

// Schema returns a copy of the view model's JSON Schema.
func (u *UIState[Ctx]) Schema() json.RawMessage {
	if u == nil {
		return nil
	}
	return cloneRaw(u.schema)
}

// Valid reports whether u was built with UIStateOf.
func (u *UIState[Ctx]) Valid() bool { return u != nil && u.fn != nil }

// Eval projects ctx into the view model.
func (u *UIState[Ctx]) Eval(ctx Ctx) any { return u.fn(ctx) }

func cloneRaw(b json.RawMessage) json.RawMessage {
	if b == nil {
		return nil
	}
	return append(json.RawMessage(nil), b...)
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	jsonNumberType    = reflect.TypeFor[json.Number]()
)

func implements(t, iface reflect.Type, addressable bool) bool {
	return t.Implements(iface) || (addressable && t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(iface))
}

func schemaFor(t reflect.Type, addressable bool, visiting map[reflect.Type]bool) map[string]any {
	if t.Name() != "" {
		if visiting[t] {
			return map[string]any{}
		}
		visiting[t] = true
		defer delete(visiting, t)
	}
	if implements(t, jsonMarshalerType, addressable) {
		return map[string]any{}
	}
	if implements(t, textMarshalerType, addressable) {
		return typed("string", t.Kind() == reflect.Pointer)
	}
	switch t.Kind() {
	case reflect.Pointer:
		return nullable(schemaFor(t.Elem(), true, visiting))
	case reflect.Interface:
		return map[string]any{}
	case reflect.Bool:
		return typed("boolean", false)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return typed("integer", false)
	case reflect.Float32, reflect.Float64:
		return typed("number", false)
	case reflect.String:
		if t == jsonNumberType {
			return typed("number", false)
		}
		return typed("string", false)
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 && !implements(t.Elem(), jsonMarshalerType, true) && !implements(t.Elem(), textMarshalerType, true) {
			return typed("string", true)
		}
		return nullable(map[string]any{"type": "array", "items": schemaFor(t.Elem(), true, visiting)})
	case reflect.Array:
		return map[string]any{"type": "array", "items": schemaFor(t.Elem(), addressable, visiting)}
	case reflect.Map:
		return nullable(map[string]any{"type": "object", "additionalProperties": schemaFor(t.Elem(), false, visiting)})
	case reflect.Struct:
		return structSchema(t, addressable, visiting)
	default:
		return map[string]any{}
	}
}

func typed(name string, null bool) map[string]any {
	if null {
		return map[string]any{"type": []string{name, "null"}}
	}
	return map[string]any{"type": name}
}

func nullable(s map[string]any) map[string]any {
	if name, ok := s["type"].(string); ok {
		s["type"] = []string{name, "null"}
	}
	return s
}

type jsonField struct {
	name     string
	depth    int
	tagged   bool
	optional bool
	quoted   bool
	typ      reflect.Type
}

func structSchema(t reflect.Type, addressable bool, visiting map[reflect.Type]bool) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, f := range dominantFields(t) {
		var s map[string]any
		if f.quoted {
			s = typed("string", f.typ.Kind() == reflect.Pointer)
		} else {
			s = schemaFor(f.typ, addressable, visiting)
		}
		props[f.name] = s
		if !f.optional {
			required = append(required, f.name)
		}
	}
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		s["required"] = required
	}
	return s
}

func dominantFields(t reflect.Type) []jsonField {
	var all []jsonField
	seen := map[reflect.Type]bool{}
	var walk func(t reflect.Type, depth int)
	walk = func(t reflect.Type, depth int) {
		if seen[t] {
			return
		}
		seen[t] = true
		defer delete(seen, t)
		for i := range t.NumField() {
			sf := t.Field(i)
			ft := sf.Type
			if sf.Anonymous {
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if !sf.IsExported() && ft.Kind() != reflect.Struct {
					continue
				}
			} else if !sf.IsExported() {
				continue
			}
			tag := sf.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" && sf.Anonymous && ft.Kind() == reflect.Struct {
				if sf.Type.Kind() == reflect.Pointer && !sf.IsExported() {
					continue
				}
				walk(ft, depth+1)
				continue
			}
			f := jsonField{
				name:     name,
				depth:    depth,
				tagged:   name != "",
				optional: hasTagOption(opts, "omitempty") || hasTagOption(opts, "omitzero"),
				typ:      sf.Type,
			}
			if f.name == "" {
				f.name = sf.Name
			}
			if hasTagOption(opts, "string") {
				q := sf.Type
				if q.Name() == "" && q.Kind() == reflect.Pointer {
					q = q.Elem()
				}
				switch q.Kind() {
				case reflect.Bool, reflect.String,
					reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
					reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
					reflect.Float32, reflect.Float64:
					f.quoted = true
				}
			}
			all = append(all, f)
		}
	}
	walk(t, 0)

	byName := map[string][]jsonField{}
	var order []string
	for _, f := range all {
		if _, ok := byName[f.name]; !ok {
			order = append(order, f.name)
		}
		byName[f.name] = append(byName[f.name], f)
	}
	var out []jsonField
	for _, name := range order {
		if f, ok := dominant(byName[name]); ok {
			out = append(out, f)
		}
	}
	return out
}

func dominant(fs []jsonField) (jsonField, bool) {
	minDepth := fs[0].depth
	for _, f := range fs {
		minDepth = min(minDepth, f.depth)
	}
	var top []jsonField
	for _, f := range fs {
		if f.depth == minDepth {
			top = append(top, f)
		}
	}
	if len(top) == 1 {
		return top[0], true
	}
	var tagged []jsonField
	for _, f := range top {
		if f.tagged {
			tagged = append(tagged, f)
		}
	}
	if len(tagged) == 1 {
		return tagged[0], true
	}
	return jsonField{}, false
}

func hasTagOption(opts, want string) bool {
	for _, o := range strings.Split(opts, ",") {
		if o == want {
			return true
		}
	}
	return false
}
