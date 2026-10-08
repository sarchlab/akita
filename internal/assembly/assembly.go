// Package assembly tracks construction and initialization of one simulation.
package assembly

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/sarchlab/akita/v5/sim/naming"
)

const (
	constructing uint32 = iota
	initializing
	ready
)

type initializer interface {
	ValidateSetup() error
	InitializeComponent()
}

// Setup is shared by full and standalone simulations. Assembly is single-threaded.
type Setup struct {
	entities []naming.Named
	names    map[string]bool
	phase    atomic.Uint32
}

func (s *Setup) RequireSetup() {
	if s.phase.Load() != constructing {
		panic("simulation: topology is frozen after initialization begins")
	}
}

func (s *Setup) RequireNameAvailable(name string) {
	s.RequireSetup()
	if name == "" {
		panic("entity name cannot be empty")
	}
	if s.names[name] {
		panic("entity " + name + " already registered")
	}
}

func (s *Setup) Register(e naming.Named) {
	s.RequireNameAvailable(e.Name())
	if s.names == nil {
		s.names = make(map[string]bool)
	}
	s.names[e.Name()] = true
	s.entities = append(s.entities, e)
}

func (s *Setup) Ready() error {
	if s.phase.Load() != ready {
		return errors.New("simulation: call Initialize successfully before execution or checkpointing")
	}
	return nil
}

func (s *Setup) Initialize() error {
	if s.phase.Load() != constructing {
		return errors.New("simulation: initialization already attempted")
	}
	var failures []error
	for _, e := range s.entities {
		if v, ok := e.(interface{ ValidateSetup() error }); ok {
			if err := v.ValidateSetup(); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", e.Name(), err))
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	// A panic in arbitrary initialization code leaves the simulation unusable.
	if !s.phase.CompareAndSwap(constructing, initializing) {
		return errors.New("simulation: initialization already attempted")
	}
	for _, e := range s.entities {
		if c, ok := e.(initializer); ok {
			c.InitializeComponent()
		}
	}
	s.phase.Store(ready)
	return nil
}
