package analysis

import (
	"math"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/queueing"
	"github.com/sarchlab/akita/v4/sim/timing"
	"github.com/tebeka/atexit"
)

// BufferAnalyzer can periodically record the buffer level of a buffer.
type BufferAnalyzer struct {
	PerfLogger
	timing.TimeTeller

	buf       queueing.Buffer
	usePeriod bool
	period    timing.VTimeInSec

	lastTime           timing.VTimeInSec
	lastBufLevel       int
	bufLevelToDuration map[int]timing.VTimeInSec
}

// Func is a function that records buffer level change.
func (b *BufferAnalyzer) Func(ctx hooking.HookCtx) {
	now := b.CurrentTime()
	buf := ctx.Domain.(queueing.Buffer)
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

		b.bufLevelToDuration = make(map[int]timing.VTimeInSec)
		b.lastTime = periodEndTime
		periodStartTime = periodEndTime
		periodEndTime = periodStartTime + b.period
	}
}

func (b *BufferAnalyzer) summarizePeriod(
	now, periodStartTime, periodEndTime timing.VTimeInSec,
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

	b.bufLevelToDuration = make(map[int]timing.VTimeInSec)

	b.lastTime = b.periodStartTime(now)
}

func (b *BufferAnalyzer) periodStartTime(
	t timing.VTimeInSec,
) timing.VTimeInSec {
	return timing.VTimeInSec(math.Floor(float64(t/b.period))) * b.period
}

func (b *BufferAnalyzer) periodEndTime(
	t timing.VTimeInSec,
) timing.VTimeInSec {
	return b.periodStartTime(t) + b.period
}

func minTime(a, b timing.VTimeInSec) timing.VTimeInSec {
	if a < b {
		return a
	}

	return b
}

// BufferAnalyzerBuilder can build a BufferAnalyzer.
type BufferAnalyzerBuilder struct {
	perfLogger PerfLogger
	timeTeller timing.TimeTeller
	usePeriod  bool
	period     timing.VTimeInSec
	buffer     queueing.Buffer
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
	timeTeller timing.TimeTeller,
) BufferAnalyzerBuilder {
	b.timeTeller = timeTeller
	return b
}

// WithPeriod sets the period to use.
func (b BufferAnalyzerBuilder) WithPeriod(
	period timing.VTimeInSec,
) BufferAnalyzerBuilder {
	b.usePeriod = true
	b.period = period

	return b
}

// WithBuffer sets the buffer to use.
func (b BufferAnalyzerBuilder) WithBuffer(
	buffer queueing.Buffer,
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
		bufLevelToDuration: make(map[int]timing.VTimeInSec),
	}

	atexit.Register(func() {
		analyzer.summarize()
	})

	return analyzer
}
