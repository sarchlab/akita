package sim

// A Simulation provides the service requires to define a simulation.
type Simulation struct {
	components    []Component
	compNameIndex map[string]int
	ports         []Port
	portNameIndex map[string]int
}

// NewSimulation creates a new simulation.
func NewSimulation() *Simulation {
	return &Simulation{
		compNameIndex: make(map[string]int),
		portNameIndex: make(map[string]int),
	}
}

// RegisterComponent registers a component with the simulation.
func (s *Simulation) RegisterComponent(c Component) {
	compName := c.Name()
	if s.compNameIndex[compName] != 0 {
		panic("component " + compName + " already registered")
	}

	s.components = append(s.components, c)
	s.compNameIndex[compName] = len(s.components) - 1

	for _, p := range c.Ports() {
		s.registerPort(p)
	}
}

// registerPort registers a port with the simulation.
func (s *Simulation) registerPort(p Port) {
	portName := p.Name()
	if s.portNameIndex[portName] != 0 {
		panic("port " + portName + " already registered")
	}

	s.ports = append(s.ports, p)
	s.portNameIndex[portName] = len(s.ports) - 1
}

// GetComponentByName returns the component with the given name.
func (s *Simulation) GetComponentByName(name string) Component {
	return s.components[s.compNameIndex[name]]
}

// GetPortByName returns the port with the given name.
func (s *Simulation) GetPortByName(name string) Port {
	return s.ports[s.portNameIndex[name]]
}
