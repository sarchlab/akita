package monitoring

import (
	"strconv"

	"github.com/sarchlab/akita/v5/daisen"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Monitor wraps a Daisen server to provide simulation monitoring capabilities.
// It is the AkitaRTM monitoring type that tracks component states, progress
// bars, and visualization traces during a live simulation run.
type Monitor struct {
	server *daisen.Server

	// Fields held until StartServer() is called.
	port       int
	engine     sim.Engine
	components []sim.Component
	visTracer  *tracing.DBTracer
	traceDBPath string
}

// NewMonitor creates a new Monitor with default settings. The monitor is not
// started until StartServer() is called. By default, the monitor listens on a
// random available port.
func NewMonitor() *Monitor {
	return &Monitor{}
}

// WithPortNumber sets the port number for the monitoring server. Returns the
// Monitor for method chaining.
func (m *Monitor) WithPortNumber(port int) *Monitor {
	m.port = port
	if m.server != nil {
		m.server.WithPortNumber(port)
	}
	return m
}

// RegisterEngine registers the simulation engine with the monitor.
func (m *Monitor) RegisterEngine(e sim.Engine) {
	m.engine = e
	if m.server != nil {
		m.server.RegisterEngine(e)
	}
}

// RegisterComponent registers a component with the monitor so its internal
// state can be inspected via the monitoring server.
func (m *Monitor) RegisterComponent(c sim.Component) {
	if m.server != nil {
		m.server.RegisterComponent(c)
	} else {
		m.components = append(m.components, c)
	}
}

// RegisterVisTracer registers a visualization tracer with the monitor.
func (m *Monitor) RegisterVisTracer(tr *tracing.DBTracer) {
	m.visTracer = tr
	if m.server != nil {
		m.server.RegisterVisTracer(tr)
	}
}

// SetTraceDBPath sets the path to the SQLite trace database used to serve
// trace data through the monitoring server.
func (m *Monitor) SetTraceDBPath(path string) {
	m.traceDBPath = path
	if m.server != nil {
		m.server.SetTraceDBPath(path)
	}
}

// StartServer initializes and starts the monitoring HTTP server. It creates the
// underlying Daisen live server and begins serving monitoring endpoints.
func (m *Monitor) StartServer() {
	if m.server != nil {
		return
	}

	addr := "localhost:0"
	if m.port > 0 {
		addr = "localhost:" + strconv.Itoa(m.port)
	}

	m.server = daisen.NewLiveServer(m.engine, addr)

	if m.port > 0 {
		m.server.WithPortNumber(m.port)
	}

	if m.visTracer != nil {
		m.server.RegisterVisTracer(m.visTracer)
	}

	if m.traceDBPath != "" {
		m.server.SetTraceDBPath(m.traceDBPath)
	}

	for _, c := range m.components {
		m.server.RegisterComponent(c)
	}
	m.components = nil

	m.server.StartServer()
}

// StopServer gracefully shuts down the monitoring server.
func (m *Monitor) StopServer() {
	if m.server != nil {
		m.server.StopServer()
	}
}

// GetServer returns the underlying Daisen server. This can be used to access
// advanced server functionality or to pass to components that require a
// *daisen.Server directly.
func (m *Monitor) GetServer() *daisen.Server {
	return m.server
}
