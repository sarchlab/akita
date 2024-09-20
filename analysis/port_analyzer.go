package analysis

import (
	"math"
	"regexp"
	"strconv"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

type portAnalyzerEntry struct {
	remotePortName string
	OutTrafficByte int64
	OutTrafficMsg  int64
	InTrafficByte  int64
	InTrafficMsg   int64
	Hops           int
}

// PortAnalyzer is a hook for the amount of traffic that passes through a Port.
type PortAnalyzer struct {
	PerfLogger
	sim.TimeTeller

	usePeriod bool
	period    sim.VTimeInSec
	port      sim.Port

	lastTime           sim.VTimeInSec
	remoteToTrafficMap map[string]portAnalyzerEntry
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
		h.remoteToTrafficMap = make(map[string]portAnalyzerEntry)
	}

	remotePortName := msg.Meta().Dst.Name()
	if h.isIncoming(msg) {
		remotePortName = msg.Meta().Src.Name()
	}

	entry, ok := h.remoteToTrafficMap[remotePortName]
	if !ok {
		h.remoteToTrafficMap[remotePortName] = portAnalyzerEntry{
			remotePortName: remotePortName,
		}
	}

	entry = h.remoteToTrafficMap[remotePortName]

	if h.isIncoming(msg) {
		entry.InTrafficByte += int64(msg.Meta().TrafficBytes)
		entry.InTrafficMsg++
	} else {
		entry.OutTrafficByte += int64(msg.Meta().TrafficBytes)
		entry.OutTrafficMsg++
		hops := manhattanDistance(msg)
		entry.Hops += hops
	}

	h.remoteToTrafficMap[remotePortName] = entry

	h.lastTime = now
}

func (h *PortAnalyzer) isIncoming(msg sim.Msg) bool {
	return msg.Meta().Dst == h.port
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
			WhereRemote: entry.remotePortName,
			EntryType:   "Traffic",
		}

		if entry.InTrafficMsg != 0 {
			perfEntry.What = "Incoming"

			perfEntry.Value = float64(entry.InTrafficByte)
			perfEntry.Unit = "Byte"
			perfEntry.Hops = 0
			h.PerfLogger.AddDataEntry(perfEntry)

			perfEntry.Value = float64(entry.InTrafficMsg)
			perfEntry.Unit = "Msg"
			perfEntry.Hops = 0
			h.PerfLogger.AddDataEntry(perfEntry)
		}

		if entry.OutTrafficMsg != 0 {
			perfEntry.What = "Outgoing"

			perfEntry.Value = float64(entry.OutTrafficByte)
			perfEntry.Unit = "Byte"
			perfEntry.Hops = 0
			h.PerfLogger.AddDataEntry(perfEntry)

			perfEntry.Value = float64(entry.OutTrafficMsg)
			perfEntry.Unit = "Msg"
			perfEntry.Hops = entry.Hops
			h.PerfLogger.AddDataEntry(perfEntry)
		}
	}

	h.remoteToTrafficMap = make(map[string]portAnalyzerEntry)
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

// Get Manhattan Distance
func manhattanDistance(msg sim.Msg) int {
	form := msg.Meta().Src.Name()
	to := msg.Meta().Dst.Name()

	formID := getGPUIDFromName(form)
	toID := getGPUIDFromName(to)

	formX, formY := convToCoord(formID)
	toX, toY := convToCoord(toID)
	distance := int(math.Abs(float64(formX-toX)) + math.Abs(float64(formY-toY)))
	// fmt.Println("fromID: ", form, "toID", to, "formX: ", formX, " formY: ", formY, " toX: ", toX, " toY: ", toY, "distance: ", distance)
	return distance
}

func getGPUIDFromName(name string) int {
	re := regexp.MustCompile(`GPU\[(\d+)\]`)

	// 查找所有匹配项
	matches := re.FindAllStringSubmatch(name, -1)

	// fmt.Println("name: ", name)
	// 遍历所有匹配项，提取数字
	for _, match := range matches {
		if len(match) > 1 {
			// fmt.Println(match[1]) // 输出匹配的数字
			gpuid, err := strconv.Atoi(match[1])
			if err == nil {
				// fmt.Println("GPUID: %d \n", gpuid)
				return gpuid
			} else {
				return 0
			}
		}
	}
	return 0
}

func convToCoord(id int) (int, int) {
	if id >= 24 {
		id += 1
	}
	// } else {
	// 	id = id + 1
	// }

	x := id % 7
	y := id / 7
	return x, y
}
