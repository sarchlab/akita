package analysis

import (
	"math"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

// BufferAnalyzer can periodically record the buffer level of a buffer.
type BufferAnalyzer struct {
	PerfLogger
	sim.TimeTeller

	buf       sim.Buffer
	usePeriod bool
	period    sim.VTimeInSec

	lastTime           sim.VTimeInSec
	lastBufLevel       int
	bufLevelToDuration map[int]sim.VTimeInSec
}

// Func is a function that records buffer level change.
func (b *BufferAnalyzer) Func(ctx sim.HookCtx) {
	now := b.CurrentTime()
	buf := ctx.Domain.(sim.Buffer)
	currLevel := buf.Size()

	if b.usePeriod {
		lastPeriodEndTime := b.periodEndTime(b.lastTime)

		if now > lastPeriodEndTime {
			b.summarize()
			b.resetPeriod()
		}
	}

	b.bufLevelToDuration[b.lastBufLevel] += now - b.lastTime
	b.lastBufLevel = currLevel
	b.lastTime = now
}

func (b *BufferAnalyzer) summarize() {
	now := b.CurrentTime()

	if !b.usePeriod {
		b.summarizePeriod(now, 0, now)
		return
	}

	periodStartTime := b.periodStartTime(b.lastTime)
	periodEndTime := b.periodEndTime(b.lastTime)

	for periodEndTime < now {
		b.summarizePeriod(now, periodStartTime, periodEndTime)

		b.bufLevelToDuration = make(map[int]sim.VTimeInSec)
		b.lastTime = periodEndTime
		periodStartTime = periodEndTime
		periodEndTime = periodStartTime + b.period
	}
}

func (b *BufferAnalyzer) summarizePeriod(
	now, periodStartTime, periodEndTime sim.VTimeInSec,
) {
	sumLevel := 0.0
	sumDuration := 0.0
	for level, duration := range b.bufLevelToDuration {
		sumLevel += float64(level) * float64(duration)
		sumDuration += float64(duration)
	}

	summarizeEndTime := minTime(periodEndTime, now)
	if summarizeEndTime > b.lastTime {
		remainingTime := summarizeEndTime - b.lastTime
		sumLevel += float64(b.lastBufLevel) * float64(remainingTime)
		sumDuration += float64(summarizeEndTime - b.lastTime)
	}

	avgLevel := sumLevel / sumDuration

	if avgLevel == 0 {
		return
	}

	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start:     periodStartTime,
		End:       periodEndTime,
		Where:     b.buf.Name(),
		What:      "Level",
		EntryType: "Buffer",
		Value:     avgLevel,
		Unit:      "",
	})
}

func (b *BufferAnalyzer) resetPeriod() {
	now := b.CurrentTime()

	b.bufLevelToDuration = make(map[int]sim.VTimeInSec)

	b.lastTime = b.periodStartTime(now)
}

func (b *BufferAnalyzer) periodStartTime(t sim.VTimeInSec) sim.VTimeInSec {
	return sim.VTimeInSec(math.Floor(float64(t/b.period))) * b.period
}

func (b *BufferAnalyzer) periodEndTime(t sim.VTimeInSec) sim.VTimeInSec {
	return b.periodStartTime(t) + b.period
}

func minTime(a, b sim.VTimeInSec) sim.VTimeInSec {
	if a < b {
		return a
	}

	return b
}

// BufferAnalyzerBuilder can build a BufferAnalyzer.
type BufferAnalyzerBuilder struct {
	perfLogger PerfLogger
	timeTeller sim.TimeTeller
	usePeriod  bool
	period     sim.VTimeInSec
	buffer     sim.Buffer
}

// MakeBufferAnalyzerBuilder creates a BufferAnalyzerBuilder.
func MakeBufferAnalyzerBuilder() BufferAnalyzerBuilder {
	return BufferAnalyzerBuilder{
		perfLogger: nil,
		timeTeller: nil,
		usePeriod:  false,
		period:     0.0,
	}
}

// WithPerfLogger sets the PerfLogger to use.
func (b BufferAnalyzerBuilder) WithPerfLogger(
	perfLogger PerfLogger,
) BufferAnalyzerBuilder {
	b.perfLogger = perfLogger
	return b
}

// WithTimeTeller sets the TimeTeller to use.
func (b BufferAnalyzerBuilder) WithTimeTeller(
	timeTeller sim.TimeTeller,
) BufferAnalyzerBuilder {
	b.timeTeller = timeTeller
	return b
}

// WithPeriod sets the period to use.
func (b BufferAnalyzerBuilder) WithPeriod(
	period sim.VTimeInSec,
) BufferAnalyzerBuilder {
	b.usePeriod = true
	b.period = period
	return b
}

// WithBuffer sets the buffer to use.
func (b BufferAnalyzerBuilder) WithBuffer(
	buffer sim.Buffer,
) BufferAnalyzerBuilder {
	b.buffer = buffer
	return b
}

// Build creates a BufferAnalyzer.
func (b BufferAnalyzerBuilder) Build() *BufferAnalyzer {
	if b.perfLogger == nil {
		panic("perfLogger is not set")
	}

	if b.timeTeller == nil {
		panic("timeTeller is not set")
	}

	if b.buffer == nil {
		panic("buffer is not set")
	}

	analyzer := &BufferAnalyzer{
		PerfLogger:         b.perfLogger,
		TimeTeller:         b.timeTeller,
		buf:                b.buffer,
		usePeriod:          b.usePeriod,
		period:             b.period,
		lastTime:           0.0,
		lastBufLevel:       0,
		bufLevelToDuration: make(map[int]sim.VTimeInSec),
	}

	atexit.Register(func() {
		analyzer.summarize()
	})

	return analyzer
}
