package analysis

import (
	"log"
	"reflect"
	"unsafe"

	"github.com/sarchlab/akita/v3/sim"
)

type PerfAnalyzer struct {
	dirPath string

	dbFilename string

	components []sim.Component
	buffers    []sim.Buffer
	ports      []sim.Port
	portNumber int

	engine sim.Engine
}

//This function creates a new PerfAnalyzer with configuration parameters. If we need many parameters, consider using a builder with With functions.
func NewPerfAnalyzer(dbFilename string, period sim.VTimeInSec) *PerfAnalyzer {
	p := &PerfAnalyzer{}

	return p
}

// RegisterEngine registers the engine that is used in the simulation.
func (p *PerfAnalyzer) RegisterEngine(e sim.Engine) {
	p.engine = e
}

// RegisterComponent register a component to be monitored.
func (p *PerfAnalyzer) RegisterComponent(c sim.Component) {
	p.components = append(p.components, c)

	p.registerBuffers(c)
	p.registerPorts(c)
}

func (p *PerfAnalyzer) registerBuffers(c sim.Component) {
	p.registerComponentOrPortBuffers(c)

	for _, port := range c.Ports() {
		p.registerComponentOrPortBuffers(port)
	}
}

func (p *PerfAnalyzer) registerComponentOrPortBuffers(c any) {
	v := reflect.ValueOf(c).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)

		fieldType := field.Type()
		bufferType := reflect.TypeOf((*sim.Buffer)(nil)).Elem()

		if fieldType == bufferType {
			fieledRef := reflect.NewAt(
				field.Type(),
				unsafe.Pointer(field.UnsafeAddr()),
			).Elem().Interface().(sim.Buffer)
			p.buffers = append(p.buffers, fieledRef)
		}
	}
}

func (p *PerfAnalyzer) registerPorts(c sim.Component) {
	p.registerPort(c)

	for _, port := range c.Ports() {
		p.registerPort(port)

		interval := float64(0.000001)
		p.collectHooks(port, interval)
	}
}

func (p *PerfAnalyzer) collectHooks(port sim.Port, interval float64) {
	name := port.Name()

	logger := log.New(p.portFile, "", 0)

	port.AcceptHook(sim.portDeFenXiQi(
		name,
		sim.VTimeInSec(interval),
		p.engine,
		logger))

}

func (p *PerfAnalyzer) registerPort(c any) {
	v := reflect.ValueOf(c).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)

		fieldType := field.Type()
		portType := reflect.TypeOf((*sim.Port)(nil)).Elem()

		if fieldType == portType && !field.IsNil() {
			fieledRef := reflect.NewAt(
				field.Type(),
				unsafe.Pointer(field.UnsafeAddr()),
			).Elem().Interface().(sim.Port)
			p.ports = append(p.ports, fieledRef)
		}
	}
}

//This method adds data to the SQLite database.
func AddDataEntry(time sim.VTimeInSec, where, what string, value float64) {

}

func (p PerfAnalyzer) WithPathDir(path string) PerfAnalyzer {
	p.dirPath = path
	return p
}

func (p PerfAnalyzer) WithDBFileName(name string) PerfAnalyzer {
	p.dbFilename = name
	return p
}

func (p PerfAnalyzer) WithEngine(engine sim.Engine) PerfAnalyzer {
	p.engine = engine
	return p
}
