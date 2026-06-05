package temporal_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	fate "github.com/arisros/fate"
	fatetemporal "github.com/arisros/fate/temporal"
)

// These tests exercise the adapter end-to-end inside Temporal's in-memory test
// environment, which advances timers on a mock clock and runs registered
// activities — validating that the clock-agnostic core, driven through the pull
// API, behaves correctly when hosted in a real workflow runtime.

// --- invocation → activity ---

type ivCtx struct{ Approved bool }
type ivEvt interface{ isIv() }
type ivDone struct{ ok bool }
type ivFail struct{}

func (ivDone) isIv() {}
func (ivFail) isIv() {}

func invokeMachine() (*fate.Machine[ivCtx, ivEvt], error) {
	return fate.CreateMachine(fate.MachineConfig[ivCtx, ivEvt]{
		ID:      "verify",
		Initial: "checking",
		States: map[string]fate.StateNodeConfig[ivCtx, ivEvt]{
			"checking": {
				Invoke: []fate.Invocation[ivCtx, ivEvt]{{
					ID:      "verify",
					Src:     "verifyToken",
					Input:   func(ivCtx) any { return "tok" },
					OnDone:  func(out any) ivEvt { return ivDone{ok: out.(bool)} },
					OnError: func(error) ivEvt { return ivFail{} },
				}},
				On: map[string][]fate.TransitionConfig[ivCtx, ivEvt]{
					"ivDone": {{
						Target: "approved",
						Guard:  func(_ ivCtx, e ivEvt) bool { return e.(ivDone).ok },
						Actions: []fate.Action[ivCtx, ivEvt]{
							fate.Assign(func(c ivCtx, _ ivEvt) ivCtx { c.Approved = true; return c }),
						},
					}},
					"ivFail": {{Target: "rejected"}},
				},
			},
			"approved": {Type: fate.NodeFinal},
			"rejected": {Type: fate.NodeFinal},
		},
	})
}

func invokeWorkflow(ctx workflow.Context) (string, error) {
	m, err := invokeMachine()
	if err != nil {
		return "", err
	}
	wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{
		ActivityOptions: workflow.ActivityOptions{StartToCloseTimeout: time.Minute},
	})
	if err != nil {
		return "", err
	}
	snap, err := wa.Run()
	if err != nil {
		return "", err
	}
	return snap.Value.Path(), nil
}

func TestWorkflowActor_InvocationDrivesActivity(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(invokeWorkflow)
	env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (interface{}, error) { return true, nil },
		activity.RegisterOptions{Name: "verifyToken"},
	)

	env.ExecuteWorkflow(invokeWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var path string
	require.NoError(t, env.GetWorkflowResult(&path))
	require.Equal(t, "approved", path)
}

func TestWorkflowActor_InvocationFailureDrivesOnError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(invokeWorkflow)
	env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (interface{}, error) {
			return nil, assertErr{}
		},
		activity.RegisterOptions{Name: "verifyToken"},
	)

	env.ExecuteWorkflow(invokeWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var path string
	require.NoError(t, env.GetWorkflowResult(&path))
	require.Equal(t, "rejected", path)
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }

// --- delayed transition → workflow timer ---

type tCtx struct{}
type tEvt interface{ isT() }

func timerMachine() (*fate.Machine[tCtx, tEvt], error) {
	return fate.CreateMachine(fate.MachineConfig[tCtx, tEvt]{
		ID:      "timer",
		Initial: "waiting",
		States: map[string]fate.StateNodeConfig[tCtx, tEvt]{
			"waiting": {After: map[time.Duration][]fate.TransitionConfig[tCtx, tEvt]{
				time.Hour: {{Target: "fired"}},
			}},
			"fired": {Type: fate.NodeFinal},
		},
	})
}

func timerWorkflow(ctx workflow.Context) (string, error) {
	m, err := timerMachine()
	if err != nil {
		return "", err
	}
	wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{})
	if err != nil {
		return "", err
	}
	snap, err := wa.Run()
	if err != nil {
		return "", err
	}
	return snap.Value.Path(), nil
}

func TestWorkflowActor_AfterDrivesWorkflowTimer(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(timerWorkflow)

	env.ExecuteWorkflow(timerWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var path string
	require.NoError(t, env.GetWorkflowResult(&path))
	require.Equal(t, "fired", path)
}

// --- external event → signal ---

func signalMachine() (*fate.Machine[struct{}, string], error) {
	return fate.CreateMachine(fate.MachineConfig[struct{}, string]{
		ID:      "gate",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[struct{}, string]{
			"idle": {On: map[string][]fate.TransitionConfig[struct{}, string]{
				"OPEN": {{Target: "open"}},
			}},
			"open": {Type: fate.NodeFinal},
		},
	})
}

func signalWorkflow(ctx workflow.Context) (string, error) {
	m, err := signalMachine()
	if err != nil {
		return "", err
	}
	wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{SignalName: "events"})
	if err != nil {
		return "", err
	}
	snap, err := wa.Run()
	if err != nil {
		return "", err
	}
	return snap.Value.Path(), nil
}

func TestWorkflowActor_SignalDeliversEvent(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(signalWorkflow)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("events", "OPEN")
	}, time.Second)

	env.ExecuteWorkflow(signalWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var path string
	require.NoError(t, env.GetWorkflowResult(&path))
	require.Equal(t, "open", path)
}
