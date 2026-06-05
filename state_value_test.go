package fate

import (
	"encoding/json"
	"errors"
	"strings"
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

type negCtx struct{}
type negEvt interface{ isNegEvt() }

func TestCreateMachine_InvalidConfig(t *testing.T) {
	cases := []struct {
		name    string
		cfg     MachineConfig[negCtx, negEvt]
		errKind error
		msgPart string
	}{
		{
			name:    "no ID",
			cfg:     MachineConfig[negCtx, negEvt]{Initial: "x", States: map[string]StateNodeConfig[negCtx, negEvt]{"x": {}}},
			errKind: ErrInvalidConfig,
			msgPart: "ID is required",
		},
		{
			name:    "no Initial",
			cfg:     MachineConfig[negCtx, negEvt]{ID: "m"},
			errKind: ErrNoInitial,
			msgPart: "no Initial",
		},
		{
			name: "Initial not in States",
			cfg: MachineConfig[negCtx, negEvt]{
				ID: "m", Initial: "missing",
				States: map[string]StateNodeConfig[negCtx, negEvt]{"x": {}},
			},
			errKind: ErrUnknownInitial,
			msgPart: "missing",
		},
		{
			name: "compound child without initial",
			cfg: MachineConfig[negCtx, negEvt]{
				ID: "m", Initial: "outer",
				States: map[string]StateNodeConfig[negCtx, negEvt]{
					"outer": {
						States: map[string]StateNodeConfig[negCtx, negEvt]{"inner": {}},
					},
				},
			},
			errKind: ErrNoInitial,
			msgPart: "outer",
		},
		{
			name: "unknown transition target",
			cfg: MachineConfig[negCtx, negEvt]{
				ID: "m", Initial: "x",
				States: map[string]StateNodeConfig[negCtx, negEvt]{
					"x": {On: map[string][]TransitionConfig[negCtx, negEvt]{"E": {{Target: "ghost"}}}},
				},
			},
			errKind: ErrUnknownTarget,
			msgPart: "ghost",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CreateMachine(c.cfg)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, c.errKind) {
				t.Errorf("error kind: got %v want kind %v", err, c.errKind)
			}
			if !strings.Contains(err.Error(), c.msgPart) {
				t.Errorf("error message: %q does not contain %q", err.Error(), c.msgPart)
			}
		})
	}
}
