package persist

import (
	"encoding/json"
	"testing"
)

func TestStateValue_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  StateValue
		want string
	}{
		{"atomic", AtomicValue("red"), `"red"`},
		{"compound one child", CompoundValue(map[string]StateValue{
			"door": AtomicValue("open"),
		}), `{"door":"open"}`},
		{"nested compound", CompoundValue(map[string]StateValue{
			"door": CompoundValue(map[string]StateValue{
				"closed": AtomicValue("idle"),
			}),
		}), `{"door":{"closed":"idle"}}`},
		{"parallel sorted", CompoundValue(map[string]StateValue{
			"zeta":  AtomicValue("z"),
			"alpha": AtomicValue("a"),
		}), `{"alpha":"a","zeta":"z"}`}, // key order is sorted for determinism (ADR-002)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.val)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Marshal: got %s want %s", string(got), tc.want)
			}
			var back StateValue
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			roundTrip, _ := json.Marshal(back)
			if string(roundTrip) != tc.want {
				t.Errorf("RoundTrip: got %s want %s", string(roundTrip), tc.want)
			}
		})
	}
}

func TestStateValue_Matches(t *testing.T) {
	v := CompoundValue(map[string]StateValue{
		"door": CompoundValue(map[string]StateValue{
			"closed": AtomicValue("idle"),
		}),
	})
	cases := []struct {
		target string
		want   bool
	}{
		{"", true},
		{"door", true},
		{"door.closed", true},
		{"door.closed.idle", true},
		{"door.closed.moving", false},
		{"door.open", false},
		{"window", false},
	}
	for _, c := range cases {
		if got := v.Matches(c.target); got != c.want {
			t.Errorf("Matches(%q): got %v want %v", c.target, got, c.want)
		}
	}
}
