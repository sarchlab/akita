package modeling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

// componentCheckpoint is the serialized form of a generic component: a spec hash
// for compatibility checking plus the mutable State. Resources and ports are
// rebuilt by setup, not serialized. The tick-scheduler guard is intentionally
// omitted: the foundation supports quiescent checkpoints only (empty event
// queue), so a freshly rebuilt component's "no pending tick" guard is already
// correct. The guard is restored alongside the event queue in a later milestone.
type componentCheckpoint struct {
	SpecHash string          `json:"spec_hash"`
	State    json.RawMessage `json:"state"`
}

// SaveCheckpoint writes the component's spec hash and State as JSON. It
// implements the structural checkpoint.Checkpointable contract without the
// modeling package importing checkpoint.
func (c *Component[S, T, R]) SaveCheckpoint(w io.Writer) error {
	state, err := json.Marshal(c.State)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	return json.NewEncoder(w).Encode(componentCheckpoint{
		SpecHash: c.specHash(),
		State:    state,
	})
}

// LoadCheckpoint restores State after verifying that the saved spec hash matches
// the rebuilt component's.
func (c *Component[S, T, R]) LoadCheckpoint(r io.Reader) error {
	var dto componentCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return fmt.Errorf("modeling: decode component checkpoint: %w", err)
	}

	if got := c.specHash(); got != dto.SpecHash {
		return fmt.Errorf(
			"modeling: spec hash mismatch: checkpoint %s, rebuilt %s",
			dto.SpecHash, got)
	}

	var state T
	if err := json.Unmarshal(dto.State, &state); err != nil {
		return fmt.Errorf("modeling: unmarshal state: %w", err)
	}
	c.State = state

	return nil
}

// specHash is a deterministic fingerprint of the component's immutable Spec, used
// to reject loading a checkpoint into a component built with a different config.
func (c *Component[S, T, R]) specHash() string {
	data, err := json.Marshal(c.spec)
	if err != nil {
		panic(fmt.Sprintf("modeling: cannot hash spec: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
