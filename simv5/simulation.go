package simv5

import (
    "fmt"
    "sync"

    "github.com/sarchlab/akita/v4/sim"
)

// Simulation is the root object for a v5-style simulator. It carries the
// discrete-event Engine and registries for shared emulation states.
type Simulation struct {
    Engine sim.Engine
    emu    *EmuStateRegistry
}

// NewSimulation creates a Simulation wrapping the given engine.
func NewSimulation(engine sim.Engine) *Simulation {
    return &Simulation{Engine: engine, emu: NewEmuStateRegistry()}
}

// Emu returns the emulation state registry.
func (s *Simulation) Emu() *EmuStateRegistry { return s.emu }

// EmuStateRegistry is a threadsafe registry for shared emulation states
// (e.g., memory/storage images) keyed by ID.
type EmuStateRegistry struct {
    mu    sync.RWMutex
    items map[string]interface{}
}

// NewEmuStateRegistry creates an empty registry.
func NewEmuStateRegistry() *EmuStateRegistry {
    return &EmuStateRegistry{items: make(map[string]interface{})}
}

// Register associates an ID with a value. Returns error if ID already exists.
func (r *EmuStateRegistry) Register(id string, v interface{}) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.items[id]; exists {
        return fmt.Errorf("emu state id already registered: %s", id)
    }
    r.items[id] = v
    return nil
}

// Put sets an ID with a value, overwriting any existing value.
func (r *EmuStateRegistry) Put(id string, v interface{}) {
    r.mu.Lock()
    r.items[id] = v
    r.mu.Unlock()
}

// Get returns the raw value and whether it exists.
func (r *EmuStateRegistry) Get(id string) (interface{}, bool) {
    r.mu.RLock()
    v, ok := r.items[id]
    r.mu.RUnlock()
    return v, ok
}

// Delete removes an item.
func (r *EmuStateRegistry) Delete(id string) {
    r.mu.Lock()
    delete(r.items, id)
    r.mu.Unlock()
}

