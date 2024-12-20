package simulation

import (
	"os"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/stateful"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A Simulation provides the service requires to define a simulation.
type Simulation struct {
	idGenerator id.IDGenerator
	engine      timing.Engine
	stateHolder map[string]stateful.StateHolder
	states      map[string]stateful.State
}

// NewSimulation creates a new simulation.
func NewSimulation() *Simulation {
	return &Simulation{
		idGenerator: id.NewIDGenerator(),
		stateHolder: make(map[string]stateful.StateHolder),
		states:      make(map[string]stateful.State),
	}
}

// ID returns the ID of the simulation.
func (s *Simulation) ID() string {
	return "simulation"
}

// RegisterEngine registers the engine used in the simulation.
func (s *Simulation) RegisterEngine(e timing.Engine) {
	s.engine = e
}

// GetEngine returns the engine used in the simulation.
func (s *Simulation) GetEngine() timing.Engine {
	return s.engine
}

// RegisterStateHolder registers a state holder and its state with the
// simulation.
func (s *Simulation) RegisterStateHolder(l stateful.StateHolder) {
	locName := l.Name()

	if _, ok := s.stateHolder[locName]; ok {
		panic("state holder " + locName + " already registered")
	}

	s.stateHolder[locName] = l
	s.states[locName] = l.State()
}

// GetStateHolderByName returns the state holder with the given name.
func (s *Simulation) GetStateHolderByName(name string) stateful.StateHolder {
	return s.stateHolder[name]
}

// GetStateByName returns the state with the given name.
func (s *Simulation) GetStateByName(name string) stateful.State {
	return s.states[name]
}

func (s *Simulation) Save(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	codec := stateful.JSONCodec{}

	data := make(map[string]any)
	for name, state := range s.states {
		if state == nil {
			continue
		}

		data[name] = state
	}

	err = codec.Encode(file, data)
	if err != nil {
		panic(err)
	}
}

func (s *Simulation) Load(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	codec := stateful.JSONCodec{}

	data, err := codec.Decode(file)
	if err != nil {
		panic(err)
	}

	for name, state := range data {
		s.states[name] = state.(stateful.State)
		s.stateHolder[name].SetState(state.(stateful.State))
	}
}
