package internal

import "reflect"

// EventName extracts a string tag for an event.
func EventName(evt any) string {
	if s, ok := evt.(string); ok {
		return s
	}
	if named, ok := evt.(interface{ EventName() string }); ok {
		return named.EventName()
	}
	t := reflect.TypeOf(evt)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := t.Name()
	for _, suffix := range []string{"T", "Event"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}
