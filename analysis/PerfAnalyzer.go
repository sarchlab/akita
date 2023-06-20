package analysis

import (
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"unsafe"

	// "reflect"
	// "unsafe"

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
	period sim.VTimeInSec

	dbFile    *os.File
	csvWriter *csv.Writer

	engine sim.Engine
}

// NewPerfAnalyzer creates a new PerfAnalyzer with configuration parameters.
func NewPerfAnalyzer(dbFilename string, period sim.VTimeInSec, engine sim.Engine) *PerfAnalyzer {
	p := &PerfAnalyzer{
		period: period,
	}

	p.engine = engine

	var err error
	p.dbFile, err = os.Create(dbFilename)
	if err != nil {
		panic(err)
	}

	p.dbFile, err = os.OpenFile(dbFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	// defer p.dbFile.Close()

	p.csvWriter = csv.NewWriter(p.dbFile)

	header := []string{"Start", "End", "Where", "What", "Value", "Unit"}
	err = p.csvWriter.Write(header)
	if err != nil {
		panic(err)
	}
	p.csvWriter.Flush()
	return p
}

// RegisterEngine registers the engine that is used in the simulation.
func (p *PerfAnalyzer) RegisterEngine(e sim.Engine) {
	p.engine = e
}

// RegisterComponent register a component to be monitored.
func (p *PerfAnalyzer) RegisterComponent(c sim.Component) {
	// p.registerBuffers(c)
	p.registerPorts(c)
}

func (p *PerfAnalyzer) registerBuffers(c sim.Component) {
	p.registerComponentOrPortBuffers(c)

	for _, port := range c.Ports() {
		p.registerComponentOrPortBuffers(port)
	}
}

func (p *PerfAnalyzer) registerComponentOrPortBuffers(c any) {
	lastTime := 0.0
	usePeriod := false
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

			bufferAnalyzer := NewBufferAnalyzer(
				p.engine, usePeriod, p, float64(p.period), lastTime,
			)
			fieldRef.AcceptHook(bufferAnalyzer)
		}
	}
}

func (p *PerfAnalyzer) registerPorts(c sim.Component) {
	for _, port := range c.Ports() {
		portAnalyzer := NewPortAnalyzer(
			port, p.engine, p, p.period,
		)
		port.AcceptHook(portAnalyzer)
	}
}

// AddDataEntry adds a data entry to the database. It directly writes into the
// CSV file.
func (p *PerfAnalyzer) AddDataEntry(entry PerfAnalyzerEntry) {
	p.csvWriter.Write(
		[]string{
			fmt.Sprintf("%.10f", entry.Start),
			fmt.Sprintf("%.10f", entry.End),
			entry.Where,
			entry.What,
			fmt.Sprintf("%.10f", entry.Value),
			entry.Unit,
		})
	p.csvWriter.Flush()
}
