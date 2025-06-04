package analysis

import (
	"math"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/tebeka/atexit"
)

type portAnalyzerEntry struct {
<<<<<<< HEAD
	remotePort     sim.RemotePort
=======
	remotePortName string
>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
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

	lastTime           sim.VTimeInSec
<<<<<<< HEAD
	remoteToTrafficMap map[sim.RemotePort]portAnalyzerEntry
=======
	remoteToTrafficMap map[string]portAnalyzerEntry
>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
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

	if h.remoteToTrafficMap == nil {
<<<<<<< HEAD
		h.remoteToTrafficMap = make(map[sim.RemotePort]portAnalyzerEntry)
	}

	remotePortName := msg.Meta().Dst
	if h.isIncoming(msg) {
		remotePortName = msg.Meta().Src
=======
		h.remoteToTrafficMap = make(map[string]portAnalyzerEntry)
	}

	remotePortName := msg.Meta().Dst.Name()
	if h.isIncoming(msg) {
		remotePortName = msg.Meta().Src.Name()
>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
	}

	entry, ok := h.remoteToTrafficMap[remotePortName]
	if !ok {
		h.remoteToTrafficMap[remotePortName] = portAnalyzerEntry{
<<<<<<< HEAD
			remotePort: remotePortName,
=======
			remotePortName: remotePortName,
>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
		}
	}

	entry = h.remoteToTrafficMap[remotePortName]

	if h.isIncoming(msg) {
		entry.InTrafficByte += int64(msg.Meta().TrafficBytes)
		entry.InTrafficMsg++
	} else {
		entry.OutTrafficByte += int64(msg.Meta().TrafficBytes)
		entry.OutTrafficMsg++
	}

	h.remoteToTrafficMap[remotePortName] = entry

	h.lastTime = now
}

func (h *PortAnalyzer) isIncoming(msg sim.Msg) bool {
<<<<<<< HEAD
	return msg.Meta().Dst == h.port.AsRemote()
=======
	return msg.Meta().Dst == h.port
>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
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

	for _, entry := range h.remoteToTrafficMap {
		perfEntry := PerfAnalyzerEntry{
			Start:       startTime,
			End:         endTime,
			Where:       h.port.Name(),
<<<<<<< HEAD
			WhereRemote: entry.remotePort,
=======
			WhereRemote: entry.remotePortName,
>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
			EntryType:   "Traffic",
		}

		if entry.InTrafficMsg != 0 {
			perfEntry.What = "Incoming"
<<<<<<< HEAD
			perfEntry.Value = float64(entry.InTrafficByte)
			perfEntry.Unit = "Byte"
			h.PerfLogger.AddDataEntry(perfEntry)

=======

			perfEntry.Value = float64(entry.InTrafficByte)
			perfEntry.Unit = "Byte"
			h.PerfLogger.AddDataEntry(perfEntry)

>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
			perfEntry.Value = float64(entry.InTrafficMsg)
			perfEntry.Unit = "Msg"
			h.PerfLogger.AddDataEntry(perfEntry)
		}

		if entry.OutTrafficMsg != 0 {
			perfEntry.What = "Outgoing"

			perfEntry.Value = float64(entry.OutTrafficByte)
			perfEntry.Unit = "Byte"
			h.PerfLogger.AddDataEntry(perfEntry)

			perfEntry.Value = float64(entry.OutTrafficMsg)
			perfEntry.Unit = "Msg"
			h.PerfLogger.AddDataEntry(perfEntry)
		}
	}

<<<<<<< HEAD
	h.remoteToTrafficMap = make(map[sim.RemotePort]portAnalyzerEntry)
=======
	h.remoteToTrafficMap = make(map[string]portAnalyzerEntry)
>>>>>>> 674be9ab7565412cd39847012b7c9cb857680930
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
