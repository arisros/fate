package temporal_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/workflow"

	fatetemporal "github.com/arisros/fate/temporal"
)

func TestDecodeSignal_DeliversInterfaceEvents(t *testing.T) {
	env := newOrderEnv()
	env.RegisterDelayedCallback(func() {
		s, err := newSignal(approve{By: "ana"})
		require.NoError(t, err)
		env.SignalWorkflow("events", s)
	}, time.Hour)
	env.ExecuteWorkflow(orderWorkflow)

	require.NoError(t, env.GetWorkflowError())
	var out string
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, "paid ana rcpt-ana", out)
}

func TestDecodeSignal_NilSkipsTheSignal(t *testing.T) {
	env := newOrderEnv()
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("events", signal{Name: "REFUND"})
	}, time.Minute)
	env.RegisterDelayedCallback(func() {
		s, err := newSignal(approve{By: "bo"})
		require.NoError(t, err)
		env.SignalWorkflow("events", s)
	}, time.Hour)
	env.ExecuteWorkflow(orderWorkflow)

	var out string
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, "paid bo rcpt-bo", out)
}

func TestDecodeSignal_NoSignalLetsTheTimerExpire(t *testing.T) {
	env := newOrderEnv()
	env.ExecuteWorkflow(orderWorkflow)

	var out string
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, "expired  ", out)
}

func signalErrorWorkflow(decode func(workflow.Context, workflow.ReceiveChannel) (any, error)) func(workflow.Context) error {
	return func(ctx workflow.Context) error {
		m, err := orderMachine()
		if err != nil {
			return err
		}
		wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{SignalName: "events", DecodeSignal: decode})
		if err != nil {
			return err
		}
		_, err = wa.Run()
		return err
	}
}

func TestDecodeSignal_FailuresStopRun(t *testing.T) {
	cases := map[string]struct {
		decode func(workflow.Context, workflow.ReceiveChannel) (any, error)
		want   string
	}{
		"decoder error": {
			decode: func(ctx workflow.Context, c workflow.ReceiveChannel) (any, error) {
				c.Receive(ctx, nil)
				return nil, errors.New("bad payload")
			},
			want: "bad payload",
		},
		"wrong type": {
			decode: func(ctx workflow.Context, c workflow.ReceiveChannel) (any, error) {
				c.Receive(ctx, nil)
				return "APPROVE", nil
			},
			want: "DecodeSignal returned string, want temporal_test.orderEvt",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newOrderEnv()
			env.RegisterDelayedCallback(func() { env.SignalWorkflow("events", signal{Name: "APPROVE"}) }, time.Minute)
			env.ExecuteWorkflow(signalErrorWorkflow(tc.decode))

			err := env.GetWorkflowError()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestNewWorkflowActor_InterfaceEventNeedsDecoder(t *testing.T) {
	opts := fatetemporal.Options{SignalName: "events"}
	constructors := map[string]func(workflow.Context) error{
		"fresh": func(ctx workflow.Context) error {
			m, err := orderMachine()
			if err != nil {
				return err
			}
			_, err = fatetemporal.NewWorkflowActor(ctx, m, opts)
			return err
		},
		"from snapshot": func(ctx workflow.Context) error {
			m, err := orderMachine()
			if err != nil {
				return err
			}
			_, err = fatetemporal.NewWorkflowActorFromSnapshot(ctx, m, []byte("{}"), opts)
			return err
		},
	}
	for name, wf := range constructors {
		t.Run(name, func(t *testing.T) {
			env := newOrderEnv()
			env.ExecuteWorkflow(wf)

			err := env.GetWorkflowError()
			require.Error(t, err)
			require.Contains(t, err.Error(), fatetemporal.ErrNoSignalDecoder.Error())
		})
	}
}
