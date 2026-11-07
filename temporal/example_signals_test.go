package temporal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"go.temporal.io/sdk/activity"
	tlog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
	fatetemporal "github.com/arisros/fate/temporal"
)

type orderCtx struct {
	ApprovedBy string
	Receipt    string
}

type orderEvt interface{ isOrderEvt() }

type approve struct{ By string }
type charged struct{ Receipt string }
type chargeFailed struct{}

func (approve) isOrderEvt()      {}
func (charged) isOrderEvt()      {}
func (chargeFailed) isOrderEvt() {}

func (approve) EventName() string      { return "APPROVE" }
func (charged) EventName() string      { return "CHARGED" }
func (chargeFailed) EventName() string { return "CHARGE_FAILED" }

type orderOn = map[string][]engine.TransitionConfig[orderCtx, orderEvt]

func orderMachine() (*engine.Machine[orderCtx, orderEvt], error) {
	return engine.CreateMachine(engine.MachineConfig[orderCtx, orderEvt]{
		ID:      "order",
		Initial: "waiting",
		States: map[string]engine.StateNodeConfig[orderCtx, orderEvt]{
			"waiting": {
				On: orderOn{"APPROVE": {{
					Target: "charging",
					Actions: []action.Action[orderCtx, orderEvt]{action.Assign(func(c orderCtx, e orderEvt) orderCtx {
						c.ApprovedBy = e.(approve).By
						return c
					})},
				}}},
				After: map[time.Duration][]engine.TransitionConfig[orderCtx, orderEvt]{24 * time.Hour: {{Target: "expired"}}},
			},
			"charging": {
				Invoke: []effect.Invocation[orderCtx, orderEvt]{{
					ID:      "charge",
					Src:     "chargeCard",
					Input:   func(c orderCtx) any { return c.ApprovedBy },
					OnDone:  func(out any) orderEvt { return charged{Receipt: out.(string)} },
					OnError: func(error) orderEvt { return chargeFailed{} },
				}},
				On: orderOn{
					"CHARGED": {{
						Target: "paid",
						Actions: []action.Action[orderCtx, orderEvt]{action.Assign(func(c orderCtx, e orderEvt) orderCtx {
							c.Receipt = e.(charged).Receipt
							return c
						})},
					}},
					"CHARGE_FAILED": {{Target: "failed"}},
				},
			},
			"paid":    {Type: engine.NodeFinal},
			"failed":  {Type: engine.NodeFinal},
			"expired": {Type: engine.NodeFinal},
		},
	})
}

type signal struct {
	Name string
	Data json.RawMessage
}

func newSignal(e interface{ EventName() string }) (signal, error) {
	data, err := json.Marshal(e)
	return signal{Name: e.EventName(), Data: data}, err
}

func decodeAs[E orderEvt](data json.RawMessage) (orderEvt, error) {
	var e E
	err := json.Unmarshal(data, &e)
	return e, err
}

var signalEvents = map[string]func(json.RawMessage) (orderEvt, error){
	"APPROVE": decodeAs[approve],
}

func decodeSignal(ctx workflow.Context, c workflow.ReceiveChannel) (any, error) {
	var s signal
	c.Receive(ctx, &s)
	decode, ok := signalEvents[s.Name]
	if !ok {
		workflow.GetLogger(ctx).Warn("dropping unknown signal event", "name", s.Name)
		return nil, nil
	}
	return decode(s.Data)
}

func orderWorkflow(ctx workflow.Context) (string, error) {
	m, err := orderMachine()
	if err != nil {
		return "", err
	}
	wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{
		ActivityOptions: workflow.ActivityOptions{StartToCloseTimeout: time.Minute},
		SignalName:      "events",
		DecodeSignal:    decodeSignal,
	})
	if err != nil {
		return "", err
	}
	snap, err := wa.Run()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s %s", snap.Value.Path(), snap.Context.ApprovedBy, snap.Context.Receipt), nil
}

func chargeCard(_ context.Context, approvedBy string) (string, error) {
	return "rcpt-" + approvedBy, nil
}

func newOrderEnv() *testsuite.TestWorkflowEnvironment {
	var suite testsuite.WorkflowTestSuite
	suite.SetLogger(tlog.NewStructuredLogger(slog.New(slog.DiscardHandler)))
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(chargeCard, activity.RegisterOptions{Name: "chargeCard"})
	return env
}

func Example_signals() {
	env := newOrderEnv()
	env.RegisterDelayedCallback(func() {
		s, _ := newSignal(approve{By: "ana"})
		env.SignalWorkflow("events", s)
	}, time.Hour)

	env.ExecuteWorkflow(orderWorkflow)

	var out string
	if err := env.GetWorkflowResult(&out); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(out)
	// Output: paid ana rcpt-ana
}
