package analysis

import (
	"math"

	"github.com/sarchlab/akita/v3/sim"
)

// BufferAnalyzer can periodically record the buffer level of a buffer.
type BufferAnalyzer struct {
	PerfLogger
	timeTeller sim.TimeTeller

	buf                sim.Buffer
	period             sim.VTimeInSec
	lastTime           sim.VTimeInSec
	lastBufLevel       int
	bufLevelToDuration map[int]sim.VTimeInSec
}

// NewBufferAnalyzer creates a buffer analyzer.
func NewBufferAnalyzer(
	buffer sim.Buffer,
	tt sim.TimeTeller,
	perfLogger PerfLogger,
	period sim.VTimeInSec,
) *BufferAnalyzer {
	b := &BufferAnalyzer{
		buf:                buffer,
		PerfLogger:         perfLogger,
		timeTeller:         tt,
		period:             period,
		lastTime:           0.0,
		bufLevelToDuration: make(map[int]sim.VTimeInSec),
	}

	return b
}

// Func is a function that records buffer level change.
func (b *BufferAnalyzer) Func(ctx sim.HookCtx) {
	now := b.timeTeller.CurrentTime()
	buf := ctx.Domain.(sim.Buffer)
	currLevel := buf.Size()

	lastPeriodEndTime := b.periodEndTime(b.lastTime)

	if now > lastPeriodEndTime {
		b.summarize()
		b.resetPeriod()
	}

	b.bufLevelToDuration[b.lastBufLevel] += now - b.lastTime
	b.lastBufLevel = currLevel
	b.lastTime = now
}

func (b *BufferAnalyzer) summarize() {
	now := b.timeTeller.CurrentTime()
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

	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start: periodStartTime,
		End:   periodEndTime,
		Where: b.buf.Name(),
		What:  "BufferLevel",
		Value: avgLevel,
		Unit:  "",
	})
}

func (b *BufferAnalyzer) resetPeriod() {
	now := b.timeTeller.CurrentTime()

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
