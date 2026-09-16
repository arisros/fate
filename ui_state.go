package fate

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// UIState projects the context into a view model for one state, together with
// a JSON Schema of that view model. Build one with UIStateOf and set it on
// StateNodeConfig.UIState. Machine.UIState evaluates it for the active
// configuration and Describe publishes the schema.
type UIState[Ctx any] struct {
	fn     func(Ctx) any
	schema json.RawMessage
}

// UIStateOf wraps fn and derives a JSON Schema for U by reflection, once, at
// call time:
//
//	StateNodeConfig[Ctx, Evt]{
//		UIState: fate.UIStateOf(func(c Ctx) ReviewView {
//			return ReviewView{Score: c.Score, Status: c.Status}
//		}),
//	}
//
// The schema follows encoding/json field naming: json tags, "-" skips a
// field, omitempty fields are optional, and untagged embedded structs are
// flattened. Types implementing json.Marshaler are left unconstrained.
func UIStateOf[Ctx any, U any](fn func(Ctx) U) *UIState[Ctx] {
	schema, err := json.Marshal(schemaFor(reflect.TypeFor[U](), map[reflect.Type]bool{}))
	if err != nil {
		panic(fmt.Sprintf("fate: UIStateOf schema: %v", err))
	}
	return &UIState[Ctx]{
		fn:     func(c Ctx) any { return fn(c) },
		schema: schema,
	}
}

// Schema returns the JSON Schema of the view model.
func (u *UIState[Ctx]) Schema() json.RawMessage { return u.schema }

// UIState evaluates the UIState of the active configuration v against ctx.
//
// For each active leaf, the nearest state on its path (the leaf itself or an
// ancestor) that declares a UIState contributes once. With no contributor the
// result is nil. With one, the result is its marshaled view model. With
// several, as in parallel regions, the result is a JSON object keyed by each
// contributing state's dot path.
func (m *Machine[Ctx, Evt]) UIState(v StateValue, ctx Ctx) (json.RawMessage, error) {
	var owners []*stateNode[Ctx, Evt]
	seen := map[*stateNode[Ctx, Evt]]bool{}
	for _, leaf := range resolveLeaves[Ctx, Evt](m.root, v) {
		for n := leaf; n != nil; n = n.parent {
			if n.uiState == nil {
				continue
			}
			if !seen[n] {
				seen[n] = true
				owners = append(owners, n)
			}
			break
		}
	}
	switch len(owners) {
	case 0:
		return nil, nil
	case 1:
		return marshalUIState(owners[0], ctx)
	}
	merged := make(map[string]json.RawMessage, len(owners))
	for _, n := range owners {
		b, err := marshalUIState(n, ctx)
		if err != nil {
			return nil, err
		}
		merged[strings.Join(n.path, ".")] = b
	}
	return json.Marshal(merged)
}

func marshalUIState[Ctx any, Evt any](n *stateNode[Ctx, Evt], ctx Ctx) (json.RawMessage, error) {
	b, err := json.Marshal(n.uiState.fn(ctx))
	if err != nil {
		return nil, fmt.Errorf("fate: ui state of %q: %w", strings.Join(n.path, "."), err)
	}
	return b, nil
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// schemaFor returns a draft-07 subset schema for t. visiting breaks cycles:
// a type already on the current path is emitted as an unconstrained schema.
func schemaFor(t reflect.Type, visiting map[reflect.Type]bool) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType) {
		return map[string]any{}
	}
	if t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType) {
		return map[string]any{"type": "string"}
	}
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string"}
		}
		return map[string]any{"type": "array", "items": schemaFor(t.Elem(), visiting)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaFor(t.Elem(), visiting)}
	case reflect.Struct:
		if visiting[t] {
			return map[string]any{}
		}
		visiting[t] = true
		defer delete(visiting, t)
		props := map[string]any{}
		var required []string
		addStructFields(t, props, &required, visiting)
		s := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			sort.Strings(required)
			s["required"] = required
		}
		return s
	default:
		return map[string]any{}
	}
}

func addStructFields(t reflect.Type, props map[string]any, required *[]string, visiting map[reflect.Type]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		name, opts, tagged := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" && !tagged {
			continue
		}
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				addStructFields(ft, props, required, visiting)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		props[name] = schemaFor(f.Type, visiting)
		if !hasTagOption(opts, "omitempty") && !hasTagOption(opts, "omitzero") {
			*required = append(*required, name)
		}
	}
}

func hasTagOption(opts, want string) bool {
	for _, o := range strings.Split(opts, ",") {
		if o == want {
			return true
		}
	}
	return false
}
