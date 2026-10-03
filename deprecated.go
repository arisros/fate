package fate

// The names below are the API as it was before the engine was split into
// packages (ADR-0007). They are frozen: new API is added to the packages only.

import (
	"github.com/arisros/fate/action"
	"github.com/arisros/fate/describe"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/persist"
)

// Actor is [engine.Actor].
//
// Deprecated: use [engine.Actor].
type Actor[Ctx any, Evt any] = engine.Actor[Ctx, Evt]

// ActorOption is [engine.ActorOption].
//
// Deprecated: use [engine.ActorOption].
type ActorOption = engine.ActorOption

// CreateMachine is [engine.CreateMachine].
//
// Deprecated: use [engine.CreateMachine].
func CreateMachine[Ctx any, Evt any](cfg engine.MachineConfig[Ctx, Evt]) (*engine.Machine[Ctx, Evt], error) {
	return engine.CreateMachine[Ctx, Evt](cfg)
}

// ErrActorNotStarted is [engine.ErrActorNotStarted].
//
// Deprecated: use [engine.ErrActorNotStarted].
var ErrActorNotStarted = engine.ErrActorNotStarted

// ErrActorStopped is [engine.ErrActorStopped].
//
// Deprecated: use [engine.ErrActorStopped].
var ErrActorStopped = engine.ErrActorStopped

// ErrDuplicateState is [engine.ErrDuplicateState].
//
// Deprecated: use [engine.ErrDuplicateState].
var ErrDuplicateState = engine.ErrDuplicateState

// ErrInvalidConfig is [engine.ErrInvalidConfig].
//
// Deprecated: use [engine.ErrInvalidConfig].
var ErrInvalidConfig = engine.ErrInvalidConfig

// ErrInvalidNodeType is [engine.ErrInvalidNodeType].
//
// Deprecated: use [engine.ErrInvalidNodeType].
var ErrInvalidNodeType = engine.ErrInvalidNodeType

// ErrNoInitial is [engine.ErrNoInitial].
//
// Deprecated: use [engine.ErrNoInitial].
var ErrNoInitial = engine.ErrNoInitial

// ErrUnknownInitial is [engine.ErrUnknownInitial].
//
// Deprecated: use [engine.ErrUnknownInitial].
var ErrUnknownInitial = engine.ErrUnknownInitial

// ErrUnknownTarget is [engine.ErrUnknownTarget].
//
// Deprecated: use [engine.ErrUnknownTarget].
var ErrUnknownTarget = engine.ErrUnknownTarget

// History is [engine.History].
//
// Deprecated: use [engine.History].
type History = engine.History

// HistoryDeep is [engine.HistoryDeep].
//
// Deprecated: use [engine.HistoryDeep].
const HistoryDeep = engine.HistoryDeep

// HistoryShallow is [engine.HistoryShallow].
//
// Deprecated: use [engine.HistoryShallow].
const HistoryShallow = engine.HistoryShallow

// Machine is [engine.Machine].
//
// Deprecated: use [engine.Machine].
type Machine[Ctx any, Evt any] = engine.Machine[Ctx, Evt]

// MachineConfig is [engine.MachineConfig].
//
// Deprecated: use [engine.MachineConfig].
type MachineConfig[Ctx any, Evt any] = engine.MachineConfig[Ctx, Evt]

// NewActor is [engine.NewActor].
//
// Deprecated: use [engine.NewActor].
func NewActor[Ctx any, Evt any](m *engine.Machine[Ctx, Evt], opts ...engine.ActorOption) *engine.Actor[Ctx, Evt] {
	return engine.NewActor[Ctx, Evt](m, opts...)
}

// NewActorFromSnapshot is [engine.NewActorFromSnapshot].
//
// Deprecated: use [engine.NewActorFromSnapshot].
func NewActorFromSnapshot[Ctx any, Evt any](m *engine.Machine[Ctx, Evt], persisted []byte) (*engine.Actor[Ctx, Evt], error) {
	return engine.NewActorFromSnapshot[Ctx, Evt](m, persisted)
}

// NewSetup is [engine.NewSetup].
//
// Deprecated: use [engine.NewSetup].
func NewSetup[Ctx any, Evt any]() *engine.Setup[Ctx, Evt] {
	return engine.NewSetup[Ctx, Evt]()
}

// NodeAtomic is [engine.NodeAtomic].
//
// Deprecated: use [engine.NodeAtomic].
const NodeAtomic = engine.NodeAtomic

// NodeCompound is [engine.NodeCompound].
//
// Deprecated: use [engine.NodeCompound].
const NodeCompound = engine.NodeCompound

// NodeFinal is [engine.NodeFinal].
//
// Deprecated: use [engine.NodeFinal].
const NodeFinal = engine.NodeFinal

// NodeHistory is [engine.NodeHistory].
//
// Deprecated: use [engine.NodeHistory].
const NodeHistory = engine.NodeHistory

// NodeParallel is [engine.NodeParallel].
//
// Deprecated: use [engine.NodeParallel].
const NodeParallel = engine.NodeParallel

// NodeType is [engine.NodeType].
//
// Deprecated: use [engine.NodeType].
type NodeType = engine.NodeType

// SelectedTransition is [engine.SelectedTransition].
//
// Deprecated: use [engine.SelectedTransition].
type SelectedTransition[Ctx any, Evt any] = engine.SelectedTransition[Ctx, Evt]

// Setup is [engine.Setup].
//
// Deprecated: use [engine.Setup].
type Setup[Ctx any, Evt any] = engine.Setup[Ctx, Evt]

// StateNodeConfig is [engine.StateNodeConfig].
//
// Deprecated: use [engine.StateNodeConfig].
type StateNodeConfig[Ctx any, Evt any] = engine.StateNodeConfig[Ctx, Evt]

// TransitionConfig is [engine.TransitionConfig].
//
// Deprecated: use [engine.TransitionConfig].
type TransitionConfig[Ctx any, Evt any] = engine.TransitionConfig[Ctx, Evt]

// WithInitialValue is [engine.WithInitialValue].
//
// Deprecated: use [engine.WithInitialValue].
func WithInitialValue[Ctx any, Evt any](v persist.StateValue) engine.ActorOption {
	return engine.WithInitialValue[Ctx, Evt](v)
}

// WithLogger is [engine.WithLogger].
//
// Deprecated: use [engine.WithLogger].
func WithLogger(fn func(string)) engine.ActorOption {
	return engine.WithLogger(fn)
}

// Action is [action.Action].
//
// Deprecated: use [action.Action].
type Action[Ctx any, Evt any] = action.Action[Ctx, Evt]

// AlwaysTrue is [action.AlwaysTrue].
//
// Deprecated: use [action.AlwaysTrue].
func AlwaysTrue[Ctx any, Evt any]() action.Guard[Ctx, Evt] {
	return action.AlwaysTrue[Ctx, Evt]()
}

// And is [action.And].
//
// Deprecated: use [action.And].
func And[Ctx any, Evt any](gs ...action.Guard[Ctx, Evt]) action.Guard[Ctx, Evt] {
	return action.And[Ctx, Evt](gs...)
}

// Assign is [action.Assign].
//
// Deprecated: use [action.Assign].
func Assign[Ctx any, Evt any](fn func(ctx Ctx, evt Evt) Ctx) action.Action[Ctx, Evt] {
	return action.Assign[Ctx, Evt](fn)
}

// Cond is [action.Cond].
//
// Deprecated: use [action.Cond].
type Cond = action.Cond

// CondAllOf is [action.CondAllOf].
//
// Deprecated: use [action.CondAllOf].
func CondAllOf(cs ...action.Cond) action.Cond {
	return action.CondAllOf(cs...)
}

// CondAnyOf is [action.CondAnyOf].
//
// Deprecated: use [action.CondAnyOf].
func CondAnyOf(cs ...action.Cond) action.Cond {
	return action.CondAnyOf(cs...)
}

// CondEq is [action.CondEq].
//
// Deprecated: use [action.CondEq].
const CondEq = action.CondEq

// CondFalsy is [action.CondFalsy].
//
// Deprecated: use [action.CondFalsy].
const CondFalsy = action.CondFalsy

// CondField is [action.CondField].
//
// Deprecated: use [action.CondField].
type CondField = action.CondField

// CondFieldBuilder is [action.CondFieldBuilder].
//
// Deprecated: use [action.CondFieldBuilder].
type CondFieldBuilder = action.CondFieldBuilder

// CondGt is [action.CondGt].
//
// Deprecated: use [action.CondGt].
const CondGt = action.CondGt

// CondGte is [action.CondGte].
//
// Deprecated: use [action.CondGte].
const CondGte = action.CondGte

// CondIn is [action.CondIn].
//
// Deprecated: use [action.CondIn].
const CondIn = action.CondIn

// CondLt is [action.CondLt].
//
// Deprecated: use [action.CondLt].
const CondLt = action.CondLt

// CondLte is [action.CondLte].
//
// Deprecated: use [action.CondLte].
const CondLte = action.CondLte

// CondMeta is [action.CondMeta].
//
// Deprecated: use [action.CondMeta].
type CondMeta = action.CondMeta

// CondNeq is [action.CondNeq].
//
// Deprecated: use [action.CondNeq].
const CondNeq = action.CondNeq

// CondNot is [action.CondNot].
//
// Deprecated: use [action.CondNot].
func CondNot(c action.Cond) action.Cond {
	return action.CondNot(c)
}

// CondOp is [action.CondOp].
//
// Deprecated: use [action.CondOp].
type CondOp = action.CondOp

// CondTruthy is [action.CondTruthy].
//
// Deprecated: use [action.CondTruthy].
const CondTruthy = action.CondTruthy

// EnqueueActions is [action.EnqueueActions].
//
// Deprecated: use [action.EnqueueActions].
func EnqueueActions[Ctx any, Evt any](fn func(enq *action.Enqueuer[Ctx, Evt])) action.Action[Ctx, Evt] {
	return action.EnqueueActions[Ctx, Evt](fn)
}

// Enqueuer is [action.Enqueuer].
//
// Deprecated: use [action.Enqueuer].
type Enqueuer[Ctx any, Evt any] = action.Enqueuer[Ctx, Evt]

// Field is [action.Field].
//
// Deprecated: use [action.Field].
func Field(path string) *action.CondFieldBuilder {
	return action.Field(path)
}

// Gates is [action.Gates].
//
// Deprecated: use [action.Gates].
func Gates(fields ...action.CondField) *action.GatesBuilder {
	return action.Gates(fields...)
}

// GatesBuilder is [action.GatesBuilder].
//
// Deprecated: use [action.GatesBuilder].
type GatesBuilder = action.GatesBuilder

// Guard is [action.Guard].
//
// Deprecated: use [action.Guard].
type Guard[Ctx any, Evt any] = action.Guard[Ctx, Evt]

// InState is [action.InState].
//
// Deprecated: use [action.InState].
func InState(path string) action.Cond {
	return action.InState(path)
}

// Log is [action.Log].
//
// Deprecated: use [action.Log].
func Log[Ctx any, Evt any](msg string) action.Action[Ctx, Evt] {
	return action.Log[Ctx, Evt](msg)
}

// Named is [action.Named].
//
// Deprecated: use [action.Named].
func Named[Ctx any, Evt any](name string, a action.Action[Ctx, Evt]) action.Action[Ctx, Evt] {
	return action.Named[Ctx, Evt](name, a)
}

// Not is [action.Not].
//
// Deprecated: use [action.Not].
func Not[Ctx any, Evt any](g action.Guard[Ctx, Evt]) action.Guard[Ctx, Evt] {
	return action.Not[Ctx, Evt](g)
}

// Or is [action.Or].
//
// Deprecated: use [action.Or].
func Or[Ctx any, Evt any](gs ...action.Guard[Ctx, Evt]) action.Guard[Ctx, Evt] {
	return action.Or[Ctx, Evt](gs...)
}

// Raise is [action.Raise].
//
// Deprecated: use [action.Raise].
func Raise[Ctx any, Evt any](evt Evt) action.Action[Ctx, Evt] {
	return action.Raise[Ctx, Evt](evt)
}

// StateIn is [action.StateIn].
//
// Deprecated: use [action.StateIn].
func StateIn(path string) action.Cond {
	return action.StateIn(path)
}

// Invocation is [effect.Invocation].
//
// Deprecated: use [effect.Invocation].
type Invocation[Ctx any, Evt any] = effect.Invocation[Ctx, Evt]

// InvokeID is [effect.InvokeID].
//
// Deprecated: use [effect.InvokeID].
type InvokeID = effect.InvokeID

// PendingInvocation is [effect.PendingInvocation].
//
// Deprecated: use [effect.PendingInvocation].
type PendingInvocation = effect.PendingInvocation

// PendingTimer is [effect.PendingTimer].
//
// Deprecated: use [effect.PendingTimer].
type PendingTimer = effect.PendingTimer

// TimerID is [effect.TimerID].
//
// Deprecated: use [effect.TimerID].
type TimerID = effect.TimerID

// ActorStatus is [persist.ActorStatus].
//
// Deprecated: use [persist.ActorStatus].
type ActorStatus = persist.ActorStatus

// AtomicValue is [persist.AtomicValue].
//
// Deprecated: use [persist.AtomicValue].
func AtomicValue(name string) persist.StateValue {
	return persist.AtomicValue(name)
}

// CompoundValue is [persist.CompoundValue].
//
// Deprecated: use [persist.CompoundValue].
func CompoundValue(children map[string]persist.StateValue) persist.StateValue {
	return persist.CompoundValue(children)
}

// Snapshot is [persist.Snapshot].
//
// Deprecated: use [persist.Snapshot].
type Snapshot[Ctx any] = persist.Snapshot[Ctx]

// SnapshotVersion is [persist.SnapshotVersion].
//
// Deprecated: use [persist.SnapshotVersion].
const SnapshotVersion = persist.SnapshotVersion

// StateValue is [persist.StateValue].
//
// Deprecated: use [persist.StateValue].
type StateValue = persist.StateValue

// StatusDone is [persist.StatusDone].
//
// Deprecated: use [persist.StatusDone].
const StatusDone = persist.StatusDone

// StatusError is [persist.StatusError].
//
// Deprecated: use [persist.StatusError].
const StatusError = persist.StatusError

// StatusRunning is [persist.StatusRunning].
//
// Deprecated: use [persist.StatusRunning].
const StatusRunning = persist.StatusRunning

// StatusStopped is [persist.StatusStopped].
//
// Deprecated: use [persist.StatusStopped].
const StatusStopped = persist.StatusStopped

// LoadDescriptor is [describe.LoadDescriptor].
//
// Deprecated: use [describe.LoadDescriptor].
func LoadDescriptor(data []byte) (describe.MachineDescriptor, error) {
	return describe.LoadDescriptor(data)
}

// MachineDescriptor is [describe.MachineDescriptor].
//
// Deprecated: use [describe.MachineDescriptor].
type MachineDescriptor = describe.MachineDescriptor

// StateNodeDescriptor is [describe.StateNodeDescriptor].
//
// Deprecated: use [describe.StateNodeDescriptor].
type StateNodeDescriptor = describe.StateNodeDescriptor

// TransitionDescriptor is [describe.TransitionDescriptor].
//
// Deprecated: use [describe.TransitionDescriptor].
type TransitionDescriptor = describe.TransitionDescriptor

// UIState is [describe.UIState].
//
// Deprecated: use [describe.UIState].
type UIState[Ctx any] = describe.UIState[Ctx]

// UIStateOf is [describe.UIStateOf].
//
// Deprecated: use [describe.UIStateOf].
func UIStateOf[Ctx any, U any](fn func(Ctx) U) *describe.UIState[Ctx] {
	return describe.UIStateOf[Ctx, U](fn)
}
