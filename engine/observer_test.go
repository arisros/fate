package engine_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/persist"
)

func observerActor(t *testing.T) *engine.Actor[int, string] {
	t.Helper()
	type state = engine.StateNodeConfig[int, string]
	type trans = engine.TransitionConfig[int, string]
	m, err := engine.CreateMachine(engine.MachineConfig[int, string]{
		ID: "toggle", Initial: "a",
		States: map[string]state{
			"a": {On: map[string][]trans{"GO": {{Target: "b"}}}},
			"b": {On: map[string][]trans{"GO": {{Target: "a"}}, "END": {{Target: "c"}}}},
			"c": {},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return engine.NewActor(m)
}

// returns fails the test when f does not return, which is how a callback that
// blocks on the actor's own lock shows up.
func returns(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: did not return, the observer is blocked on the actor", what)
	}
}

func TestObservers_MayCallTheActor(t *testing.T) {
	ctx := context.Background()
	for name, subscribe := range map[string]func(a *engine.Actor[int, string]){
		"snapshot observer reads the actor": func(a *engine.Actor[int, string]) {
			a.Subscribe(func(persist.Snapshot[int]) {
				_ = a.Snapshot()
				_ = a.Can("GO")
				_ = a.NextEvents()
				_ = a.PendingTimers()
				_, _ = a.Persist()
				_, _ = a.Preview("GO")
			})
		},
		"step observer reads the actor": func(a *engine.Actor[int, string]) {
			a.SubscribeSteps(func(engine.Step) {
				_ = a.Snapshot()
				_ = a.Can("GO")
			})
		},
		"observer subscribes another": func(a *engine.Actor[int, string]) {
			a.Subscribe(func(persist.Snapshot[int]) { a.Subscribe(func(persist.Snapshot[int]) {}) })
		},
	} {
		a := observerActor(t)
		subscribe(a)
		returns(t, name, func() {
			_ = a.Start(ctx)
			_ = a.Send(ctx, "GO")
		})
	}
}

func TestObservers_UnsubscribeFromInside(t *testing.T) {
	ctx := context.Background()
	a := observerActor(t)
	calls := 0
	var stop func()
	stop = a.Subscribe(func(persist.Snapshot[int]) {
		calls++
		stop()
	})
	returns(t, "self-unsubscribe", func() {
		_ = a.Start(ctx)
		_ = a.Send(ctx, "GO")
		_ = a.Send(ctx, "GO")
	})
	if calls != 1 {
		t.Errorf("observer ran %d times after unsubscribing itself, want 1", calls)
	}
}

func TestObservers_NestedSendIsDeliveredInOrderBeforeTheOuterReturns(t *testing.T) {
	ctx := context.Background()
	a := observerActor(t)
	var log []string
	depth := 0
	a.SubscribeSteps(func(s engine.Step) {
		depth++
		if depth > 1 {
			t.Errorf("step %d delivered inside another observer call", s.Seq)
		}
		log = append(log, fmt.Sprintf("step %d %s", s.Seq, s.Value.Path()))
		if s.Seq == 2 {
			_ = a.Send(ctx, "END")
			log = append(log, "nested Send returned")
		}
		depth--
	})
	a.Subscribe(func(s persist.Snapshot[int]) {
		log = append(log, "snapshot "+s.Value.Path())
	})
	returns(t, "nested Send", func() {
		_ = a.Start(ctx)
		_ = a.Send(ctx, "GO")
		log = append(log, "outer Send returned")
	})
	want := []string{
		"step 1 a",
		"snapshot a",
		"step 2 b",
		"nested Send returned",
		"snapshot b",
		"step 3 c",
		"snapshot c",
		"outer Send returned",
	}
	if !reflect.DeepEqual(log, want) {
		t.Errorf("delivery order:\n got %q\nwant %q", log, want)
	}
}

func TestObservers_APanicDoesNotWedgeTheActor(t *testing.T) {
	ctx := context.Background()
	a := observerActor(t)
	boom := true
	seen := 0
	a.Subscribe(func(persist.Snapshot[int]) {
		if boom {
			boom = false
			panic("observer failed")
		}
		seen++
	})
	func() {
		defer func() { _ = recover() }()
		_ = a.Start(ctx)
	}()
	returns(t, "Send after an observer panic", func() { _ = a.Send(ctx, "GO") })
	if seen != 1 {
		t.Errorf("observer ran %d times after the panic, want 1", seen)
	}
	if got := a.Snapshot().Value.Path(); got != "b" {
		t.Errorf("value %q, want %q", got, "b")
	}
}

func TestObservers_ConcurrentSendersDeliverEveryStepInOrder(t *testing.T) {
	ctx := context.Background()
	a := observerActor(t)
	var seqs []uint64
	a.SubscribeSteps(func(s engine.Step) { seqs = append(seqs, s.Seq) })
	_ = a.Start(ctx)

	const senders, each = 8, 50
	var wg sync.WaitGroup
	for range senders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				_ = a.Send(ctx, "GO")
			}
		}()
	}
	wg.Wait()
	_ = a.Send(ctx, "GO")

	if want := senders*each + 2; len(seqs) != want {
		t.Fatalf("delivered %d steps, want %d", len(seqs), want)
	}
	for i, seq := range seqs {
		if seq != uint64(i+1) {
			t.Fatalf("step %d delivered at position %d", seq, i)
		}
	}
}
