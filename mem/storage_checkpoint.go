package mem

import (
	"fmt"
	"io"
)

// StorageCheckpointResource adapts Storage to the simulation resource
// checkpoint interface without making the simulation package depend on mem.
type StorageCheckpointResource struct {
	name    string
	storage *Storage
}

// NewStorageCheckpointResource creates a checkpoint resource wrapper for a
// Storage instance.
func NewStorageCheckpointResource(
	name string,
	storage *Storage,
) StorageCheckpointResource {
	return StorageCheckpointResource{
		name:    name,
		storage: storage,
	}
}

// Name returns the stable manifest name for the storage resource.
func (r StorageCheckpointResource) Name() string {
	return r.name
}

// Kind returns the resource kind written into the checkpoint manifest.
func (r StorageCheckpointResource) Kind() string {
	return "mem.Storage"
}

// Format returns the storage checkpoint payload format.
func (r StorageCheckpointResource) Format() string {
	return "binary"
}

// FileExtension returns the payload filename extension.
func (r StorageCheckpointResource) FileExtension() string {
	return ".bin"
}

// Identity returns a runtime identity for deduplicating references to the same
// storage object.
func (r StorageCheckpointResource) Identity() string {
	return fmt.Sprintf("%p", r.storage)
}

// Save writes storage contents to w.
func (r StorageCheckpointResource) Save(w io.Writer) error {
	return r.storage.Save(w)
}

// Load reads storage contents from r.
func (r StorageCheckpointResource) Load(reader io.Reader) error {
	return r.storage.Load(reader)
}
