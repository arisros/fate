package internal

import "reflect"

// EventName extracts the name an event dispatches on. The rules are, in order:
//
//  1. A plain string is its own name.
//  2. A type with an EventName() method is named by it.
//  3. A value of a named string type (type Kind string) is its own value.
//  4. A named struct, or a pointer to one, is named after its type, with the
//     conventional codegen suffixes ("T", "Event") stripped.
//
// Anything else has no name these rules can derive: every value of a named
// int enum would collapse to the type name. EventName then reports false, as
// it does for an empty name and for a nil event.
func EventName(evt any) (string, bool) {
	if s, ok := evt.(string); ok {
		return s, s != ""
	}
	v := reflect.ValueOf(evt)
	if !v.IsValid() || (v.Kind() == reflect.Pointer && v.IsNil()) {
		return "", false
	}
	if named, ok := evt.(interface{ EventName() string }); ok {
		name := named.EventName()
		return name, name != ""
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String(), v.String() != ""
	case reflect.Struct:
		name := v.Type().Name()
		for _, suffix := range []string{"T", "Event"} {
			if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
				return name[:len(name)-len(suffix)], true
			}
		}
		return name, name != ""
	default:
		return "", false
	}
}
