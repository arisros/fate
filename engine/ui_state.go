package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/arisros/fate/describe"
	"github.com/arisros/fate/persist"
)

// UIState evaluates the view models of the active configuration v against ctx,
// keyed by the dot path of the state that declares each one.
//
// For each active leaf, the nearest state on its path (the leaf itself or an
// ancestor) that declares a UIState contributes, once even when several leaves
// share it. The result is nil when no active state declares one. A view model
// that fails to marshal, or whose function panics, returns an error naming the
// state.
func (m *Machine[Ctx, Evt]) UIState(v persist.StateValue, ctx Ctx) (map[string]json.RawMessage, error) {
	var out map[string]json.RawMessage
	for _, leaf := range resolveLeaves[Ctx, Evt](m.root, v) {
		for n := leaf; n != nil; n = n.parent {
			if n.uiState == nil {
				continue
			}
			key := strings.Join(n.path, ".")
			if _, done := out[key]; done {
				break
			}
			b, err := evalUIState(key, n.uiState, ctx)
			if err != nil {
				return nil, err
			}
			if out == nil {
				out = map[string]json.RawMessage{}
			}
			out[key] = b
			break
		}
	}
	return out, nil
}

func evalUIState[Ctx any](path string, u *describe.UIState[Ctx], ctx Ctx) (b json.RawMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("fate: ui state of %q panicked: %v", path, r)
		}
	}()
	b, err = json.Marshal(u.Eval(ctx))
	if err != nil {
		return nil, fmt.Errorf("fate: ui state of %q: %w", path, err)
	}
	return b, nil
}
