package temporal_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/arisros/fate/engine"
	fatetemporal "github.com/arisros/fate/temporal"
)

type legacyEvt interface{ isLegacy() }
type legacyApprove struct{ By string }

func (legacyApprove) isLegacy()         {}
func (legacyApprove) EventName() string { return "APPROVE" }

func legacyWorkflow(ctx workflow.Context) (string, error) {
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, legacyEvt]{
		ID:      "legacy",
		Initial: "waiting",
		States: map[string]engine.StateNodeConfig[struct{}, legacyEvt]{
			"waiting": {
				On:    map[string][]engine.TransitionConfig[struct{}, legacyEvt]{"APPROVE": {{Target: "approved"}}},
				After: map[time.Duration][]engine.TransitionConfig[struct{}, legacyEvt]{time.Hour: {{Target: "expired"}}},
			},
			"approved": {Type: engine.NodeFinal},
			"expired":  {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		return "", err
	}
	wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{SignalName: "events"})
	if err != nil {
		return "", err
	}
	snap, err := wa.Run()
	return snap.Value.Path(), err
}

func TestReplay_InterfaceSignalHistoryFromV0_11(t *testing.T) {
	r := worker.NewWorkflowReplayer()
	r.RegisterWorkflowWithOptions(legacyWorkflow, workflow.RegisterOptions{Name: "LegacySignalWorkflow"})
	require.NoError(t, r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/legacy_interface_signal_history.json"))
}
