package analysis

import (
	"reflect"
	"unsafe"

	"github.com/sarchlab/akita/v3/sim"
)

// PerfAnalyzerEntry is a single entry in the performance database.
type PerfAnalyzerEntry struct {
	Start sim.VTimeInSec
	End   sim.VTimeInSec
	Where string
	What  string
	Value float64
	Unit  string
}

// PerfLogger is the interface that provide the service that can record
// performance data entries.
type PerfLogger interface {
	AddDataEntry(entry PerfAnalyzerEntry)
}

// PerfAnalyzer can report performance metrics during simulation.
type PerfAnalyzer struct {
	usePeriod bool
	period    sim.VTimeInSec
	engine    sim.Engine
	backend   PerfAnalyzerBackend
}

// RegisterEngine registers the engine that is used in the simulation.
func (p *PerfAnalyzer) RegisterEngine(e sim.Engine) {
	p.engine = e
}

// RegisterComponent register a component to be monitored.
func (p *PerfAnalyzer) RegisterComponent(c sim.Component) {
	p.registerComponentBuffers(c)
	p.registerComponentPorts(c)
}

func (p *PerfAnalyzer) registerComponentBuffers(c sim.Component) {
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
			fieldRef := reflect.NewAt(
				field.Type(),
				unsafe.Pointer(field.UnsafeAddr()),
			).Elem().Interface().(sim.Buffer)

			p.RegisterBuffer(fieldRef)
		}
	}
}

func (p *PerfAnalyzer) RegisterBuffer(buf sim.Buffer) {
	bufferAnalyzerBuilder := MakeBufferAnalyzerBuilder().
		WithTimeTeller(p.engine).
		WithPerfLogger(p).
		WithBuffer(buf)

	if p.usePeriod {
		bufferAnalyzerBuilder.WithPeriod(p.period)
	}

	bufferAnalyzer := bufferAnalyzerBuilder.Build()

	buf.AcceptHook(bufferAnalyzer)
}

func (p *PerfAnalyzer) registerComponentPorts(c sim.Component) {
	for _, port := range c.Ports() {
		p.RegisterPort(port)
	}
}

// RegisterPort registers a port to be monitored.
func (p *PerfAnalyzer) RegisterPort(port sim.Port) {
	portAnalyzerBuilder := MakePortAnalyzerBuilder().
		WithTimeTeller(p.engine).
		WithPerfLogger(p).
		WithPort(port)

	if p.usePeriod {
		portAnalyzerBuilder.WithPeriod(p.period)
	}

	portAnalyzer := portAnalyzerBuilder.Build()

	port.AcceptHook(portAnalyzer)
}

// AddDataEntry adds a data entry to the database. It directly writes into the
// CSV file.
func (p *PerfAnalyzer) AddDataEntry(entry PerfAnalyzerEntry) {
	p.backend.AddDataEntry(entry)
}

// PerfAnalyzerBuilder is a builder that can build a PerfAnalyzer.
type PerfAnalyzerBuilder struct {
	usePeriod   bool
	period      sim.VTimeInSec
	backendType string
	dbFilename  string
}

// MakePerfAnalyzerBuilder creates a new PerfAnalyzerBuilder.
func MakePerfAnalyzerBuilder() PerfAnalyzerBuilder {
	return PerfAnalyzerBuilder{
		usePeriod:   false,
		period:      0,
		backendType: "csv",
		dbFilename:  "perf",
	}
}

// WithPeriod sets the period of the PerfAnalyzer.
func (b PerfAnalyzerBuilder) WithPeriod(
	period sim.VTimeInSec,
) PerfAnalyzerBuilder {
	b.usePeriod = true
	b.period = period
	return b
}

// WithSQLiteBackend sets the backend of the PerfAnalyzer to be a SQLite.
func (b PerfAnalyzerBuilder) WithSQLiteBackend() PerfAnalyzerBuilder {
	b.backendType = "sqlite"
	return b
}

// WithDBFilename sets the filename of the database file.
func (b PerfAnalyzerBuilder) WithDBFilename(
	filename string,
) PerfAnalyzerBuilder {
	b.dbFilename = filename
	return b
}

// Build creates a PerfAnalyzer.
func (b PerfAnalyzerBuilder) Build() *PerfAnalyzer {
	var backend PerfAnalyzerBackend
	if b.backendType == "csv" {
		backend = NewCSVPerfAnalyzerBackend(b.dbFilename)
	} else if b.backendType == "sqlite" {
		backend = NewSQLitePerfAnalyzerBackend(b.dbFilename)
	} else {
		panic("Unknown backend type")
	}

	return &PerfAnalyzer{
		period:  b.period,
		backend: backend,
	}
}
