package temporal_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
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

func invokeMachine() (*engine.Machine[ivCtx, ivEvt], error) {
	return engine.CreateMachine(engine.MachineConfig[ivCtx, ivEvt]{
		ID:      "verify",
		Initial: "checking",
		States: map[string]engine.StateNodeConfig[ivCtx, ivEvt]{
			"checking": {
				Invoke: []effect.Invocation[ivCtx, ivEvt]{{
					ID:      "verify",
					Src:     "verifyToken",
					Input:   func(ivCtx) any { return "tok" },
					OnDone:  func(out any) ivEvt { return ivDone{ok: out.(bool)} },
					OnError: func(error) ivEvt { return ivFail{} },
				}},
				On: map[string][]engine.TransitionConfig[ivCtx, ivEvt]{
					"ivDone": {{
						Target: "approved",
						Guard:  func(_ ivCtx, e ivEvt) bool { return e.(ivDone).ok },
						Actions: []action.Action[ivCtx, ivEvt]{
							action.Assign(func(c ivCtx, _ ivEvt) ivCtx { c.Approved = true; return c }),
						},
					}},
					"ivFail": {{Target: "rejected"}},
				},
			},
			"approved": {Type: engine.NodeFinal},
			"rejected": {Type: engine.NodeFinal},
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

func timerMachine() (*engine.Machine[tCtx, tEvt], error) {
	return engine.CreateMachine(engine.MachineConfig[tCtx, tEvt]{
		ID:      "timer",
		Initial: "waiting",
		States: map[string]engine.StateNodeConfig[tCtx, tEvt]{
			"waiting": {After: map[time.Duration][]engine.TransitionConfig[tCtx, tEvt]{
				time.Hour: {{Target: "fired"}},
			}},
			"fired": {Type: engine.NodeFinal},
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

func signalMachine() (*engine.Machine[struct{}, string], error) {
	return engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "gate",
		Initial: "idle",
		States: map[string]engine.StateNodeConfig[struct{}, string]{
			"idle": {On: map[string][]engine.TransitionConfig[struct{}, string]{
				"OPEN": {{Target: "open"}},
			}},
			"open": {Type: engine.NodeFinal},
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

// --- a state left and entered again restarts its timer ---

func retryMachine() (*engine.Machine[struct{}, string], error) {
	return engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "approval",
		Initial: "waiting",
		States: map[string]engine.StateNodeConfig[struct{}, string]{
			"waiting": {
				After: map[time.Duration][]engine.TransitionConfig[struct{}, string]{
					time.Hour: {{Target: "expired"}},
				},
				On: map[string][]engine.TransitionConfig[struct{}, string]{
					"RETRY": {{Target: "waiting"}},
				},
			},
			"expired": {Type: engine.NodeFinal},
		},
	})
}

func retryWorkflow(ctx workflow.Context) (time.Duration, error) {
	started := workflow.Now(ctx)
	m, err := retryMachine()
	if err != nil {
		return 0, err
	}
	wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{SignalName: "events"})
	if err != nil {
		return 0, err
	}
	if _, err := wa.Run(); err != nil {
		return 0, err
	}
	return workflow.Now(ctx).Sub(started), nil
}

func TestWorkflowActor_ReEntryRestartsTheTimer(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(retryWorkflow)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("events", "RETRY")
	}, 30*time.Minute)

	env.ExecuteWorkflow(retryWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var elapsed time.Duration
	require.NoError(t, env.GetWorkflowResult(&elapsed))
	require.Equal(t, 90*time.Minute, elapsed, "the hour restarts when the state is re-entered")
}

// --- typed results and per-invocation options ---

type score struct{ Value int }

type scoreEvt struct{ Score score }

func scoreMachine() (*engine.Machine[int, scoreEvt], error) {
	return engine.CreateMachine(engine.MachineConfig[int, scoreEvt]{
		ID:      "scoring",
		Initial: "scoring",
		States: map[string]engine.StateNodeConfig[int, scoreEvt]{
			"scoring": {
				Invoke: []effect.Invocation[int, scoreEvt]{{
					ID: "score", Src: "score",
					OnDone: func(out any) scoreEvt { return scoreEvt{Score: out.(score)} },
				}},
				On: map[string][]engine.TransitionConfig[int, scoreEvt]{
					"scoreEvt": {{Target: "scored", Actions: []action.Action[int, scoreEvt]{
						action.Assign(func(_ int, e scoreEvt) int { return e.Score.Value }),
					}}},
				},
			},
			"scored": {Type: engine.NodeFinal},
		},
	})
}

func scoreWorkflow(ctx workflow.Context) (int, error) {
	m, err := scoreMachine()
	if err != nil {
		return 0, err
	}
	wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{
		InvocationOptions: func(inv effect.PendingInvocation) workflow.ActivityOptions {
			if inv.Src != "score" || inv.State != "scoring" {
				return workflow.ActivityOptions{}
			}
			return workflow.ActivityOptions{StartToCloseTimeout: time.Minute}
		},
		DecodeResult: func(_ effect.PendingInvocation, f workflow.Future) (any, error) {
			var s score
			err := f.Get(ctx, &s)
			return s, err
		},
	})
	if err != nil {
		return 0, err
	}
	snap, err := wa.Run()
	if err != nil {
		return 0, err
	}
	return snap.Context, nil
}

func TestWorkflowActor_TypedResultAndPerInvocationOptions(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(scoreWorkflow)
	env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (score, error) { return score{Value: 72}, nil },
		activity.RegisterOptions{Name: "score"},
	)

	env.ExecuteWorkflow(scoreWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var got int
	require.NoError(t, env.GetWorkflowResult(&got))
	require.Equal(t, 72, got)
}
