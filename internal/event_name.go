package internal

import "reflect"

// EventName extracts the name an event dispatches on.
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
