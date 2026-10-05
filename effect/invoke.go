package effect

// InvokeID identifies one armed invocation instance for as long as its state is active.
type InvokeID string

// Invocation declares external work a state runs while it is active — XState's invoke.
type Invocation[Ctx any, Evt any] struct {
	ID  string
	Src string
	// Input, OnDone and OnError run locked: must not call the actor or block.
	Input   func(ctx Ctx) any
	OnDone  func(output any) Evt
	OnError func(err error) Evt
}

// PendingInvocation is what an adapter reads from engine.Actor.PendingInvocations to learn which work to run.
type PendingInvocation struct {
	ID    InvokeID
	Src   string
	Input any
	State string
}
