package engine

import (
	"errors"
	"strings"
	"testing"
)

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
