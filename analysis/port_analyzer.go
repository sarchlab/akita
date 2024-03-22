package analysis

import (
	"math"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

type PortAnalyzerEntry struct {
	Linker         string
	OutTrafficByte int64
	OutTrafficMsg  int64
	InTrafficByte  int64
	InTrafficMsg   int64
}

// PortAnalyzer is a hook for the amount of traffic that passes through a Port.
type PortAnalyzer struct {
	PerfLogger
	sim.TimeTeller

	usePeriod bool
	period    sim.VTimeInSec
	port      sim.Port

	lastTime          sim.VTimeInSec
	PortAnalyzerTable map[string]PortAnalyzerEntry
}

// Func writes the message information into the logger
func (h *PortAnalyzer) Func(ctx sim.HookCtx) {

	now := h.CurrentTime()
	msg, ok := ctx.Item.(sim.Msg)
	if !ok {
		return
	}

	if h.usePeriod {
		lastPeriodEndTime := h.periodEndTime(h.lastTime)
		if now > lastPeriodEndTime {
			h.summarize()
		}
	}

	if h.PortAnalyzerTable == nil {
		h.PortAnalyzerTable = make(map[string]PortAnalyzerEntry)
	}

	// comp := h.port.Name()
	linker := msg.Meta().Dst.Name()
	entry, ok := h.PortAnalyzerTable[linker]
	if !ok {
		h.PortAnalyzerTable[linker] = PortAnalyzerEntry{Linker: linker}
	}
	entry = h.PortAnalyzerTable[linker]

	h.lastTime = now
	if msg.Meta().Dst == h.port {
		entry.Linker = msg.Meta().Src.Name()
		entry.InTrafficByte += int64(msg.Meta().TrafficBytes)
		entry.InTrafficMsg++
	} else {
		entry.Linker = msg.Meta().Dst.Name()
		entry.OutTrafficByte += int64(msg.Meta().TrafficBytes)
		entry.OutTrafficMsg++
	}
	h.PortAnalyzerTable[linker] = entry
}

func (h *PortAnalyzer) summarize() {
	now := h.CurrentTime()

	startTime := sim.VTimeInSec(0)
	endTime := now

	if h.usePeriod {
		startTime = h.periodStartTime(h.lastTime)
		endTime = h.periodEndTime(h.lastTime)

		if endTime > now {
			endTime = now
		}
	}

	for dst, entry := range h.PortAnalyzerTable {

		if entry.InTrafficMsg != 0 {
			h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
				Start:  startTime,
				End:    endTime,
				Src:    h.port.Name(),
				Linker: entry.Linker,
				Dir:    "Incoming",
				Value:  float64(entry.InTrafficByte),
				Unit:   "Byte",
			})

			h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
				Start:  startTime,
				End:    endTime,
				Src:    h.port.Name(),
				Linker: entry.Linker,
				Dir:    "Incoming",
				Value:  float64(entry.InTrafficMsg),
				Unit:   "Msg",
			})
		} else {
			h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
				Start:  startTime,
				End:    endTime,
				Src:    h.port.Name(),
				Linker: entry.Linker,
				Dir:    "Outgoing",
				Value:  float64(entry.OutTrafficByte),
				Unit:   "Byte",
			})

			h.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
				Start:  startTime,
				End:    endTime,
				Src:    h.port.Name(),
				Linker: entry.Linker,
				Dir:    "Outgoing",
				Value:  float64(entry.OutTrafficMsg),
				Unit:   "Msg",
			})
		}
		delete(h.PortAnalyzerTable, dst)
	}
}

func (h *PortAnalyzer) periodStartTime(t sim.VTimeInSec) sim.VTimeInSec {
	return sim.VTimeInSec(math.Floor(float64(t/h.period))) * h.period
}

func (h *PortAnalyzer) periodEndTime(t sim.VTimeInSec) sim.VTimeInSec {
	return h.periodStartTime(t) + h.period
}

// PortAnalyzerBuilder can build a PortAnalyzer.
type PortAnalyzerBuilder struct {
	perfLogger PerfLogger
	timeTeller sim.TimeTeller
	usePeriod  bool
	period     sim.VTimeInSec
	port       sim.Port
}

// MakePortAnalyzerBuilder creates a PortAnalyzerBuilder.
func MakePortAnalyzerBuilder() PortAnalyzerBuilder {
	return PortAnalyzerBuilder{}
}

// WithPerfLogger sets the logger to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithPerfLogger(l PerfLogger) PortAnalyzerBuilder {
	b.perfLogger = l
	return b
}

// WithTimeTeller sets the TimeTeller to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithTimeTeller(
	t sim.TimeTeller,
) PortAnalyzerBuilder {
	b.timeTeller = t
	return b
}

// WithPeriod sets the period to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithPeriod(p sim.VTimeInSec) PortAnalyzerBuilder {
	b.usePeriod = true
	b.period = p
	return b
}

// WithPort sets the port to be used by the PortAnalyzer.
func (b PortAnalyzerBuilder) WithPort(p sim.Port) PortAnalyzerBuilder {
	b.port = p
	return b
}

// Build creates a PortAnalyzer.
func (b PortAnalyzerBuilder) Build() *PortAnalyzer {
	if b.perfLogger == nil {
		panic("PortAnalyzer requires a PerfLogger")
	}

	if b.timeTeller == nil {
		panic("PortAnalyzer requires a TimeTeller")
	}

	if b.port == nil {
		panic("PortAnalyzer requires a Port")
	}

	a := &PortAnalyzer{
		PerfLogger: b.perfLogger,
		TimeTeller: b.timeTeller,
		usePeriod:  b.usePeriod,
		period:     b.period,
		port:       b.port,
	}

	atexit.Register(func() { a.summarize() })

	return a
}
