package simulation

import "io"

// SaveContext carries cross-entity information available to an entity while it
// serializes itself. It is intentionally opaque for now and reserved for
// Phase B (serialization); fields may be added without changing the
// Checkpointable contract.
type SaveContext struct{}

// LoadContext carries cross-entity information available to an entity while it
// loads its checkpoint, such as the name-based lookups used to re-link
// references after raw state is restored. It is reserved for Phase B.
type LoadContext struct{}

// Checkpointable is implemented by entities that own and serialize their own
// runtime state. The global state manager reserves this contract so Phase B can
// implement save/load per entity without reshaping the entity model; it is not
// yet invoked by Save or Load.
type Checkpointable interface {
	CheckpointName() string
	CheckpointKind() string
	SaveCheckpoint(ctx SaveContext, w io.Writer) error
	LoadCheckpoint(ctx LoadContext, r io.Reader) error
}

// AfterCheckpointLoad is an optional hook an entity implements to restore
// derived wiring, reset guards, and schedule required wakeups after all raw
// state is loaded. Reserved for Phase B.
type AfterCheckpointLoad interface {
	AfterCheckpointLoad(ctx LoadContext) error
}
