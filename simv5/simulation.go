package simv5

import (
    "fmt"
    "sync"

    "github.com/rs/xid"
    "github.com/sarchlab/akita/v4/datarecording"
    "github.com/sarchlab/akita/v4/monitoring"
    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
    "github.com/sarchlab/akita/v4/tracing"
)

// Simulation mirrors the mature Simulation in the simulation package and adds
// an emulation state registry for V5 components.
type Simulation struct {
    id           string
    engine       sim.Engine
    dataRecorder datarecording.DataRecorder
    monitor      *monitoring.Monitor
    visTracer    *tracing.DBTracer

    components    []sim.Component
    compNameIndex map[string]int
    ports         []sim.Port
    portNameIndex map[string]int

    state *stateRegistry
    conv  *converterRegistry
}

// NewSimulation wraps an engine into a Simulation with defaults.
func NewSimulation(engine sim.Engine) *Simulation {
    s := &Simulation{
        engine:        engine,
        id:            xid.New().String(),
        compNameIndex: make(map[string]int),
        portNameIndex: make(map[string]int),
        state:         newStateRegistry(),
        conv:          newConverterRegistry(),
    }
    // Minimal default data recorder and tracer; monitoring can be added via builder.
    s.dataRecorder = datarecording.NewDataRecorder("akita_sim_" + s.id)
    s.visTracer = tracing.NewDBTracer(s.engine, s.dataRecorder)
    return s
}

// ID returns the simulation ID.
func (s *Simulation) ID() string { return s.id }

// GetEngine returns the engine used in the simulation.
func (s *Simulation) GetEngine() sim.Engine { return s.engine }

// GetDataRecorder returns the data recorder.
func (s *Simulation) GetDataRecorder() datarecording.DataRecorder { return s.dataRecorder }

// GetMonitor returns the monitor used in the simulation.
func (s *Simulation) GetMonitor() *monitoring.Monitor { return s.monitor }

// GetVisTracer returns the tracer used in the simulation.
func (s *Simulation) GetVisTracer() *tracing.DBTracer { return s.visTracer }

// Components returns all the components registered in the simulation.
func (s *Simulation) Components() []sim.Component { return s.components }

// RegisterComponent registers a component with the simulation.
func (s *Simulation) RegisterComponent(c sim.Component) {
    compName := c.Name()
    if s.compNameIndex[compName] != 0 {
        panic("component " + compName + " already registered")
    }
    s.components = append(s.components, c)
    s.compNameIndex[compName] = len(s.components) - 1
    if s.monitor != nil {
        s.monitor.RegisterComponent(c)
    }
    for _, p := range c.Ports() {
        s.registerPort(p)
    }
}

func (s *Simulation) registerPort(p sim.Port) {
    portName := p.Name()
    if s.portNameIndex[portName] != 0 {
        panic("port " + portName + " already registered")
    }
    s.ports = append(s.ports, p)
    s.portNameIndex[portName] = len(s.ports) - 1
}

// GetComponentByName returns the component with the given name.
func (s *Simulation) GetComponentByName(name string) sim.Component {
    return s.components[s.compNameIndex[name]]
}

// GetPortByName returns the port with the given name.
func (s *Simulation) GetPortByName(name string) sim.Port {
    return s.ports[s.portNameIndex[name]]
}

// Terminate terminates the simulation.
func (s *Simulation) Terminate() {
    s.dataRecorder.Close()
    if s.monitor != nil {
        s.monitor.StopServer()
    }
}

// stateRegistry is a threadsafe registry for shared emulation states.
type stateRegistry struct {
    mu    sync.RWMutex
    items map[string]interface{}
}

// newStateRegistry creates an empty registry.
func newStateRegistry() *stateRegistry {
    return &stateRegistry{items: make(map[string]interface{})}
}

// register associates an ID with a value. Returns error if ID already exists.
func (r *stateRegistry) register(id string, v interface{}) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.items[id]; exists {
        return fmt.Errorf("emu state id already registered: %s", id)
    }
    r.items[id] = v
    return nil
}

// put sets an ID with a value, overwriting any existing value.
func (r *stateRegistry) put(id string, v interface{}) {
    r.mu.Lock()
    r.items[id] = v
    r.mu.Unlock()
}

// get returns the raw value and whether it exists.
func (r *stateRegistry) get(id string) (interface{}, bool) {
    r.mu.RLock()
    v, ok := r.items[id]
    r.mu.RUnlock()
    return v, ok
}

// delete removes an item.
func (r *stateRegistry) delete(id string) {
    r.mu.Lock()
    delete(r.items, id)
    r.mu.Unlock()
}

// RegisterState registers a shared emulation state by ID.
func (s *Simulation) RegisterState(id string, v interface{}) error {
    return s.state.register(id, v)
}

// PutState upserts a shared emulation state by ID.
func (s *Simulation) PutState(id string, v interface{}) { s.state.put(id, v) }

// GetState returns a shared emulation state by ID.
func (s *Simulation) GetState(id string) (interface{}, bool) { return s.state.get(id) }

// DeleteState removes a shared emulation state by ID.
func (s *Simulation) DeleteState(id string) { s.state.delete(id) }

// Address converter DI -------------------------------------------------------

// AddressConverterFactory builds a mem.AddressConverter from primitive params.
type AddressConverterFactory func(params map[string]uint64) (mem.AddressConverter, error)

type converterRegistry struct {
    mu    sync.RWMutex
    items map[string]AddressConverterFactory
}

func newConverterRegistry() *converterRegistry {
    return &converterRegistry{items: make(map[string]AddressConverterFactory)}
}

// RegisterAddressConverter registers a factory for a given kind.
func (s *Simulation) RegisterAddressConverter(kind string, f AddressConverterFactory) error {
    s.conv.mu.Lock()
    defer s.conv.mu.Unlock()
    if _, ok := s.conv.items[kind]; ok {
        return fmt.Errorf("address converter kind already registered: %s", kind)
    }
    s.conv.items[kind] = f
    return nil
}

// BuildAddressConverter resolves a converter by kind/params. Provides built-in
// defaults for "identity" and "interleaving" if not registered.
func (s *Simulation) BuildAddressConverter(kind string, params map[string]uint64) (mem.AddressConverter, error) {
    if kind == "" || kind == "identity" {
        return identityConverter{}, nil
    }

    s.conv.mu.RLock()
    f, ok := s.conv.items[kind]
    s.conv.mu.RUnlock()
    if ok {
        return f(params)
    }

    switch kind {
    case "interleaving":
        var c mem.InterleavingConverter
        if v, ok := params["InterleavingSize"]; ok { c.InterleavingSize = v } else { return nil, fmt.Errorf("missing InterleavingSize") }
        if v, ok := params["TotalNumOfElements"]; ok { c.TotalNumOfElements = int(v) } else { return nil, fmt.Errorf("missing TotalNumOfElements") }
        if v, ok := params["CurrentElementIndex"]; ok { c.CurrentElementIndex = int(v) } else { return nil, fmt.Errorf("missing CurrentElementIndex") }
        if v, ok := params["Offset"]; ok { c.Offset = v } else { c.Offset = 0 }
        return c, nil
    default:
        return nil, fmt.Errorf("unknown address converter kind: %s", kind)
    }
}

// identityConverter implements mem.AddressConverter as a no-op.
type identityConverter struct{}

func (identityConverter) ConvertExternalToInternal(external uint64) uint64 { return external }
func (identityConverter) ConvertInternalToExternal(internal uint64) uint64 { return internal }
