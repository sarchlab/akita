package simulation

import (
	"os"

	"github.com/sarchlab/akita/v4/sim/naming"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// NamedSerializable is a location that can be stored and indexed in a
// simulation.
type NamedSerializable interface {
	naming.Named
	serialization.Serializable
}

// A Simulation provides the service requires to define a simulation.
type Simulation struct {
	engine timing.Engine

	stateHolder map[string]StateHolder
	states      map[string]State
}

// NewSimulation creates a new simulation.
func NewSimulation() *Simulation {
	return &Simulation{
		stateHolder: make(map[string]StateHolder),
		states:      make(map[string]State),
	}
}

// RegisterEngine registers the engine used in the simulation.
func (s *Simulation) RegisterEngine(e timing.Engine) {
	s.engine = e
}

// GetEngine returns the engine used in the simulation.
func (s *Simulation) GetEngine() timing.Engine {
	return s.engine
}

// RegisterStateful registers a stateful object with the simulation.
func (s *Simulation) RegisterStateHolder(obj StateHolder) {
	s.stateHolder[obj.Name()] = obj
	s.states[obj.Name()] = obj.State()
}

// GetStateHolder returns a stateful object by its name.
func (s *Simulation) GetStateHolder(name string) StateHolder {
	return s.stateHolder[name]
}

// GetState returns a state by its name.
func (s *Simulation) GetState(name string) State {
	return s.states[name]
}

// Save saves the state of the simulation to a file.
func (s *Simulation) Save(filename string) {
	codec := serialization.NewJSONCodec()
	serializer := serialization.NewManager(codec)

	serializer.StartSerialization()

	for _, obj := range s.states {
		_, err := serializer.Serialize(obj)
		if err != nil {
			panic(err)
		}
	}

	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	serializer.FinalizeSerialization(file)
}

func (s *Simulation) Load(filename string) {
	codec := serialization.NewJSONCodec()
	serializer := serialization.NewManager(codec)

	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	serializer.StartDeserialization(file)

	for name, obj := range s.states {
		v := serialization.IDToDeserialize(obj.Name())

		newState, err := serializer.Deserialize(v)
		if err != nil {
			panic(err)
		}

		s.states[name] = newState.(State)
		s.stateHolder[name].SetState(newState.(State))
	}

	serializer.FinalizeDeserialization()
}
