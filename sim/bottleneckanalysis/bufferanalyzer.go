package bottleneckanalysis

import (
	"fmt"
	"log"
	"math"

	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v2/sim"
)

// BufferAnalyzer can use buffer levels to analyze the bottleneck of the system.
type BufferAnalyzer struct {
	timeTeller sim.TimeTeller
	logger     *log.Logger
	lastTime   float64
	period     float64
	usePeriod  bool

	buffers map[string]*bufferInfo // map of buffer name to buffer
}

type bufferInfo struct {
	buf                          sim.Buffer
	lastBufLevel                 int
	lastTime                     float64
	bufLevelToDuration           map[int]float64
	lastPeriodBufLevelToDuration map[int]float64
}

func (b bufferInfo) getAverageBufLevel() float64 {
	sum := 0.0
	durationSum := 0.0
	for level, duration := range b.bufLevelToDuration {
		sum += float64(level) * duration
		durationSum += duration
	}

	if durationSum == 0.0 {
		return 0.0
	}

	return sum / durationSum
}

func (b bufferInfo) getAverageBufLevelInPeriod() float64 {
	sum := 0.0
	durationSum := 0.0
	for level, duration := range b.lastPeriodBufLevelToDuration {
		sum += float64(level) * duration
		durationSum += duration
	}

	if durationSum == 0.0 {
		return 0.0
	}

	return sum / durationSum
}

// NewBufferAnalyzer creates a new BufferAnalyzer.
func NewBufferAnalyzer(timeTeller sim.TimeTeller) *BufferAnalyzer {
	ba := &BufferAnalyzer{
		timeTeller: timeTeller,
		buffers:    make(map[string]*bufferInfo),
		logger:     log.Default(),
		lastTime:   0,
	}

	atexit.Register(ba.Report)

	return ba
}

// NewBufferAnalyzerWithPeriod creates a new BufferAnalyzer that reports buffer
// levels periodically.
func NewBufferAnalyzerWithPeriod(
	timeTeller sim.TimeTeller,
	period float64,
) *BufferAnalyzer {
	ba := NewBufferAnalyzer(timeTeller)
	ba.period = period
	ba.usePeriod = true

	return ba
}

// CreateBuffer creates a buffer to be analyzed
func (b *BufferAnalyzer) CreateBuffer(name string, capacity int) sim.Buffer {
	buf := sim.NewBuffer(name, capacity)

	b.buffers[name] = &bufferInfo{
		buf:                          buf,
		lastBufLevel:                 0,
		lastTime:                     0,
		bufLevelToDuration:           make(map[int]float64),
		lastPeriodBufLevelToDuration: make(map[int]float64),
	}

	buf.AcceptHook(b)

	return buf
}

// CreateBufferedPort creates a port with an incoming buffer that is monitored
// by the BufferAnalyzer.
func (b *BufferAnalyzer) CreateBufferedPort(
	comp sim.Component,
	capacity int,
	name string,
) *sim.LimitNumMsgPort {
	buf := b.CreateBuffer(name+".Buffer", capacity)
	port := sim.NewLimitNumMsgPortWithExternalBuffer(comp, buf, name)
	return port
}

// Func is a function that records buffer level change.
func (b *BufferAnalyzer) Func(ctx sim.HookCtx) {
	buf := ctx.Domain.(sim.Buffer)
	now := float64(b.timeTeller.CurrentTime())

	if b.usePeriod {
		if math.Floor(now/b.period) != math.Floor(b.lastTime/b.period) {
			b.resetPeriod()
			b.reportPeriod()
		}
	}

	bufInfo, ok := b.buffers[buf.Name()]
	if !ok {
		panic("buffer not created by BufferAnalyzer")
	}

	duration := now - bufInfo.lastTime
	currentLevel := buf.Size()

	bufInfo.bufLevelToDuration[bufInfo.lastBufLevel] += duration

	if b.usePeriod {
		periodStartTime := math.Floor(now/b.period) * b.period
		durationInPeriod := now - periodStartTime

		if duration > durationInPeriod {
			duration = durationInPeriod
		}

		bufInfo.lastPeriodBufLevelToDuration[bufInfo.lastBufLevel] += duration
	}

	bufInfo.lastTime = now
	bufInfo.lastBufLevel = currentLevel
}

func (b *BufferAnalyzer) getBufAverageLevel(name string) float64 {
	return b.buffers[name].getAverageBufLevel()
}

func (b *BufferAnalyzer) getInPeriodBufAverageLevel(name string) float64 {
	if !b.usePeriod {
		panic("period mode not enabled")
	}

	return b.buffers[name].getAverageBufLevelInPeriod()
}

func (b *BufferAnalyzer) reportPeriod() {
	b.Report()
}

func (b *BufferAnalyzer) resetPeriod() {
	for _, bufInfo := range b.buffers {
		bufInfo.lastPeriodBufLevelToDuration = make(map[int]float64)
	}
}

// Report will dump the buffer level information.
func (b *BufferAnalyzer) Report() {
	// fmt.Fprintln(b.logger.Writer(),
	// 	"name, time, current, average, period average, capacity")

	for name, bufInfo := range b.buffers {
		fmt.Fprintf(b.logger.Writer(),
			"%s, %.10f, %d, %.10f, %.10f, %d\n",
			name,
			b.timeTeller.CurrentTime(),
			bufInfo.lastBufLevel,
			bufInfo.getAverageBufLevel(),
			bufInfo.getAverageBufLevelInPeriod(),
			bufInfo.buf.Size())
	}
}
