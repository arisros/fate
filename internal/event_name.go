package internal

import "reflect"

// EventName extracts a string tag for an event. The convention is:
//
//  1. If Evt is a string (or string-typed), it is the name directly.
//  2. If Evt has an EventName() method, that is used.
//  3. Otherwise, reflection takes the concrete struct type's name and
//     strips conventional suffixes ("T", "Event") used by codegen.
//
// Codegen-emitted typed events (per ADR-006) implement EventName() so they
// don't pay the reflection cost.
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
