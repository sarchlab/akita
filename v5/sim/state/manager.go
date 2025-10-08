package state

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
	"sync"
)

// Manager owns named state objects and coordinates transactional updates.
type Manager struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

type entry struct {
	typ       reflect.Type
	active    any
	staged    any
	hasStaged bool
}

// NewManager constructs a Manager with no registered states.
func NewManager() *Manager {
	return &Manager{entries: make(map[string]*entry)}
}

// Register installs a new state value under the provided key.
// The supplied value becomes the active value for that key and is deep copied
// so that further mutations to the original value do not affect the manager.
func (m *Manager) Register(key string, value any) error {
	if key == "" {
		return fmt.Errorf("state: key must be non-empty")
	}
	if value == nil {
		return fmt.Errorf("state: value for %q must be non-nil", key)
	}

	copyVal, err := deepCopy(value)
	if err != nil {
		return fmt.Errorf("state: unable to copy value for %q: %w", key, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[key]; exists {
		return fmt.Errorf("state: key %q already registered", key)
	}

	m.entries[key] = &entry{
		typ:    reflect.TypeOf(value),
		active: copyVal,
	}

	return nil
}

// Load returns a deep copy of the active value stored under key.
func (m *Manager) Load(key string) (any, error) {
	m.mu.RLock()
	e, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("state: key %q is not registered", key)
	}

	return deepCopy(e.active)
}

// Stage returns a mutable copy of the active value for the provided key. The
// same staged value is returned on subsequent calls within the same staging
// window.
func (m *Manager) Stage(key string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[key]
	if !ok {
		return nil, fmt.Errorf("state: key %q is not registered", key)
	}

	if e.hasStaged {
		return e.staged, nil
	}

	copyVal, err := deepCopy(e.active)
	if err != nil {
		return nil, fmt.Errorf("state: unable to copy value for %q: %w", key, err)
	}

	e.staged = copyVal
	e.hasStaged = true
	return e.staged, nil
}

// Commit writes the staged value for key into the active slot. It is an error
// to call Commit when there is no staged value.
func (m *Manager) Commit(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[key]
	if !ok {
		return fmt.Errorf("state: key %q is not registered", key)
	}
	if !e.hasStaged {
		return fmt.Errorf("state: key %q has no staged value", key)
	}

	e.active = e.staged
	e.staged = nil
	e.hasStaged = false
	return nil
}

// CommitAll applies all staged values to their corresponding active entries.
func (m *Manager) CommitAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.entries {
		if e.hasStaged {
			e.active = e.staged
			e.staged = nil
			e.hasStaged = false
		}
	}
}

// DiscardAll forgets every staged value without committing it.
func (m *Manager) DiscardAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.entries {
		e.staged = nil
		e.hasStaged = false
	}
}

func deepCopy(value any) (any, error) {
	registerGobType(value)

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}

	typ := reflect.TypeOf(value)
	var target reflect.Value
	if typ.Kind() == reflect.Ptr {
		target = reflect.New(typ.Elem())
	} else {
		target = reflect.New(typ)
	}

	dec := gob.NewDecoder(&buf)
	if err := dec.Decode(target.Interface()); err != nil {
		return nil, err
	}

	if typ.Kind() == reflect.Ptr {
		return target.Interface(), nil
	}
	return target.Elem().Interface(), nil
}

func registerGobType(value any) {
	typ := reflect.TypeOf(value)
	gob.Register(value)
	if typ.Kind() != reflect.Ptr {
		gob.Register(reflect.New(typ).Interface())
	}
}
