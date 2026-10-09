package internal

import "testing"

type nameKind string

type nameEnum int

type nameMethod int

func (nameMethod) EventName() string { return "METHOD" }

type nameEmptyMethod struct{}

func (nameEmptyMethod) EventName() string { return "" }

type SubmitEvent struct{}

type ResetT struct{}

type Plain struct{}

func TestEventName(t *testing.T) {
	kind := nameKind("PTR_KIND")
	var nilPlain *Plain
	cases := []struct {
		label string
		evt   any
		want  string
		ok    bool
	}{
		{"plain string", "GO", "GO", true},
		{"empty string", "", "", false},
		{"named string", nameKind("KIND"), "KIND", true},
		{"empty named string", nameKind(""), "", false},
		{"pointer to named string", &kind, "PTR_KIND", true},
		{"method on a basic kind", nameMethod(3), "METHOD", true},
		{"method returning empty", nameEmptyMethod{}, "", false},
		{"struct", Plain{}, "Plain", true},
		{"pointer to struct", &Plain{}, "Plain", true},
		{"Event suffix", SubmitEvent{}, "Submit", true},
		{"T suffix", ResetT{}, "Reset", true},
		{"anonymous struct", struct{}{}, "", false},
		{"int enum", nameEnum(2), "", false},
		{"nil", nil, "", false},
		{"nil pointer", nilPlain, "", false},
	}
	for _, c := range cases {
		got, ok := EventName(c.evt)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: EventName = (%q, %v), want (%q, %v)", c.label, got, ok, c.want, c.ok)
		}
	}
}
