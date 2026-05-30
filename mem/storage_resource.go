package mem

import "fmt"

// StorageResource adapts Storage to the simulation's shared-state Resource
// interface, so a Storage can be registered with the simulation and resolved by
// name through the global state manager without the simulation package
// depending on mem.
type StorageResource struct {
	name    string
	storage *Storage
}

// NewStorageResource creates a resource wrapper that registers a Storage
// instance as shared state under the given name.
func NewStorageResource(name string, storage *Storage) StorageResource {
	return StorageResource{
		name:    name,
		storage: storage,
	}
}

// Name returns the stable name the storage is registered under.
func (r StorageResource) Name() string {
	return r.name
}

// Kind returns the resource kind.
func (r StorageResource) Kind() string {
	return "mem.Storage"
}

// Identity returns a runtime identity used to deduplicate references to the
// same storage object.
func (r StorageResource) Identity() string {
	return fmt.Sprintf("%p", r.storage)
}

// Storage returns the underlying storage, so a consumer that resolves this
// resource by name can read and mutate the memory contents directly.
func (r StorageResource) Storage() *Storage {
	return r.storage
}
