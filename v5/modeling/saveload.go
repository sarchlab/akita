package modeling

import (
	"encoding/json"
	"io"
)

// componentSnapshot is the serializable representation of a Component's
// spec and state. It is used by SaveState and LoadState.
type componentSnapshot[S any, T any] struct {
	Spec  S `json:"spec"`
	State T `json:"state"`
}

// SaveState marshals the component's spec and current state as JSON and writes
// it to w. Only the current (A-buffer) state is serialized. Both S and T must
// be JSON-serializable (which is guaranteed by the Spec/State constraints).
func (c *Component[S, T]) SaveState(w io.Writer) error {
	snap := componentSnapshot[S, T]{
		Spec:  c.spec,
		State: c.current,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	_, err = w.Write(data)

	return err
}

// LoadState reads JSON from r and restores the component's spec and state.
// The loaded state is written to both current and next buffers (via deep copy)
// so that the component starts in a consistent double-buffered state.
func (c *Component[S, T]) LoadState(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var snap componentSnapshot[S, T]
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	c.spec = snap.Spec
	c.current = snap.State
	c.next = deepCopy(snap.State)

	return nil
}
