package simulation

import (
	"os"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/naming"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func init() {
	serialization.RegisterType(&Simulation{})
}

// A Simulation provides the service requires to define a simulation.
type Simulation struct {
	id            string
	engine        timing.Engine
	locations     []naming.Named
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
		"id":        s.id,
		"engine":    s.engine,
		"locations": s.locations,
	}, nil
}

// Deserialize deserializes the simulation from a map.
func (s *Simulation) Deserialize(
	m map[string]any,
) (serialization.Serializable, error) {
	s.id = m["id"].(string)
	s.engine = m["engine"].(timing.Engine)
	s.locations = m["locations"].([]naming.Named)

	s.locationIndex = make(map[string]int)
	for i, loc := range s.locations {
		s.locationIndex[loc.Name()] = i
	}

	return s, nil
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
func (s *Simulation) RegisterLocation(l naming.Named) {
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

	newSim, err := serializer.Deserialize()
	if err != nil {
		panic(err)
	}

	s = newSim.(*Simulation)
}
