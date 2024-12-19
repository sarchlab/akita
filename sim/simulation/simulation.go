package simulation

import (
	"os"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/naming"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func init() {
	serialization.RegisterType(reflect.TypeOf(&Simulation{}))
}

// NamedSerializable is a location that can be stored and indexed in a
// simulation.
type NamedSerializable interface {
	naming.Named
	serialization.Serializable
}

// A Simulation provides the service requires to define a simulation.
type Simulation struct {
	id            string
	engine        timing.Engine
	locations     []NamedSerializable
	locationIndex map[string]int
}

// NewSimulation creates a new simulation.
func NewSimulation() *Simulation {
	return &Simulation{
		id:            id.Generate(),
		locationIndex: make(map[string]int),
	}
}

// ID returns the ID of the simulation.
func (s *Simulation) ID() string {
	return s.id
}

// Serialize serializes the simulation into a map.
func (s *Simulation) Serialize() (map[string]any, error) {
	return map[string]any{
		"locations": s.locations,
	}, nil
}

// Deserialize deserializes the simulation from a map.
func (s *Simulation) Deserialize(
	m map[string]any,
) error {
	locInData := m["locations"].([]any)

	for i, loc := range s.locations {
		loc.Deserialize(locInData[i].(map[string]any))
	}

	return nil
}

// RegisterEngine registers the engine used in the simulation.
func (s *Simulation) RegisterEngine(e timing.Engine) {
	s.engine = e
}

// GetEngine returns the engine used in the simulation.
func (s *Simulation) GetEngine() timing.Engine {
	return s.engine
}

// RegisterLocation registers a location with the simulation.
func (s *Simulation) RegisterLocation(l NamedSerializable) {
	locName := l.Name()
	if s.locationIndex[locName] != 0 {
		panic("location " + locName + " already registered")
	}

	s.locations = append(s.locations, l)
	s.locationIndex[locName] = len(s.locations) - 1
}

func (s *Simulation) GetLocation(name string) naming.Named {
	return s.locations[s.locationIndex[name]]
}

func (s *Simulation) Save(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	codec := serialization.NewJSONCodec(file, file)
	serializer := serialization.NewManager(codec)

	err = serializer.Serialize(s)
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

	codec := serialization.NewJSONCodec(file, file)
	serializer := serialization.NewManager(codec)

	err = serializer.Deserialize(s)
	if err != nil {
		panic(err)
	}
}
