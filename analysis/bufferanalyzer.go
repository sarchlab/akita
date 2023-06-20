package analysis

import (
	"math"
	"reflect"
	"unsafe"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

// BufferAnalyzer can use buffer levels to analyze the bottleneck of the system.
type BufferAnalyzer struct {
	timeTeller sim.TimeTeller
	PerfLogger

	lastTime  float64
	period    float64
	usePeriod bool

	buffer *bufferInfo
}

type bufferInfo struct {
	// id                           int
	buf                          sim.Buffer
	lastBufLevel                 int
	lastRecordTime               float64
	bufLevelToDuration           map[int]float64
	lastPeriodBufLevelToDuration map[int]float64
	activeInLastPeriod           bool
}

// NewBufferAnalyzer
func NewBufferAnalyzer(
	tt sim.TimeTeller,
	usePeriod bool,
	perfLogger PerfLogger,
	period float64,
	lastTime float64,
) *BufferAnalyzer {
	bufferInfo := &bufferInfo{
		buf:                          nil,
		lastBufLevel:                 0,
		lastRecordTime:               0.0,
		bufLevelToDuration:           make(map[int]float64),
		lastPeriodBufLevelToDuration: make(map[int]float64),
		activeInLastPeriod:           false,
	}

	b := &BufferAnalyzer{
		PerfLogger: perfLogger,
		timeTeller: tt,
		usePeriod:  usePeriod,
		period:     period,
		lastTime:   0.0,
		buffer:     bufferInfo,
	}

	atexit.Register(func() {
		b.Report()
	})

	return b
}

// Func is a function that records buffer level change.
func (b *BufferAnalyzer) Func(ctx sim.HookCtx) {
	buf := ctx.Domain.(sim.Buffer)
	now := float64(b.timeTeller.CurrentTime())

	if b.usePeriod {
		if math.Floor(now/b.period) != math.Floor(b.lastTime/b.period) {
			b.reportPeriod()
			b.resetPeriod()
		}
	}

	bufInfo := b.buffer
	if bufInfo.buf == nil {
		panic("buffer not created by BufferAnalyzer")
	}

	duration := now - bufInfo.lastRecordTime
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

	bufInfo.activeInLastPeriod = true
	bufInfo.lastRecordTime = now
	bufInfo.lastBufLevel = currentLevel

	b.lastTime = now
}

func (b *BufferAnalyzer) reportPeriod() {
	now := float64(b.timeTeller.CurrentTime())
	lastPeriodEndTime := b.lastPeriodEndTime(now)

	b.terminatePeriod(b.buffer, lastPeriodEndTime)

	b.dumpPeriodReport(lastPeriodEndTime)
}

func (b *BufferAnalyzer) lastPeriodEndTime(now float64) float64 {
	return math.Floor(now/b.period) * b.period
}

func (b *BufferAnalyzer) terminatePeriod(
	bufInfo *bufferInfo,
	periodEndTime float64,
) {
	bufInfo.bufLevelToDuration[bufInfo.lastBufLevel] +=
		periodEndTime - bufInfo.lastRecordTime
	bufInfo.lastRecordTime = periodEndTime
}

func (b *BufferAnalyzer) dumpPeriodReport(time float64) {
	if b.buffer.activeInLastPeriod {
		b.reportBufferInfoPeriodly(b.buffer, time)
	}
}

func (b *BufferAnalyzer) reportBufferInfoPeriodly(
	buffer *bufferInfo,
	period float64,
) {
	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start: sim.VTimeInSec(period),
		End:   sim.VTimeInSec(period),
		Where: b.buffer.buf.Name(),
		What:  "lastBufLevel",
		Value: float64(b.buffer.lastBufLevel),
		Unit:  "BuffInfo",
	})

	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start: sim.VTimeInSec(period),
		End:   sim.VTimeInSec(period),
		Where: b.buffer.buf.Name(),
		What:  "AverageBufLevel",
		Value: b.getAverageBufLevel(),
		Unit:  "BuffInfo",
	})

	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start: sim.VTimeInSec(period),
		End:   sim.VTimeInSec(period),
		Where: b.buffer.buf.Name(),
		What:  "AverageBufLevelInPeriod",
		Value: b.getAverageBufLevelInPeriod(),
		Unit:  "BuffInfo",
	})
}

func (b *BufferAnalyzer) resetPeriod() {
	now := float64(b.timeTeller.CurrentTime())
	b.buffer.lastPeriodBufLevelToDuration = make(map[int]float64)
	b.buffer.lastBufLevel = b.buffer.buf.Size()
	b.buffer.lastRecordTime = now
	b.buffer.activeInLastPeriod = false
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

// AddBuffer adds a buffer to be analyzed.
func (b *BufferAnalyzer) AddBuffer(buf sim.Buffer) {
	buf.AcceptHook(b)
	bufInfo := &bufferInfo{
		buf:                          buf,
		lastBufLevel:                 buf.Size(),
		lastRecordTime:               float64(b.timeTeller.CurrentTime()),
		bufLevelToDuration:           make(map[int]float64),
		lastPeriodBufLevelToDuration: make(map[int]float64),
	}
	b.buffer = bufInfo
}

// AddPort adds a port to be analyzed.
func (b *BufferAnalyzer) AddPort(port any) {
	b.addComponentOrPortBuffers(port)
}

// AddComponent adds all the ports of a component to analyze.
func (b *BufferAnalyzer) AddComponent(comp sim.Component) {
	b.addComponentOrPortBuffers(comp)

	for _, p := range comp.Ports() {
		b.addComponentOrPortBuffers(p)
	}
}

func (b *BufferAnalyzer) addComponentOrPortBuffers(c interface{}) {
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
			b.AddBuffer(fieledRef)
		}
	}
}

func (b *BufferAnalyzer) getBufAverageLevel(name string) float64 {
	return b.getAverageBufLevel()
}

func (b *BufferAnalyzer) getInPeriodBufAverageLevel(name string) float64 {
	if !b.usePeriod {
		panic("period mode not enabled")
	}

	return b.getAverageBufLevelInPeriod()
}

// func (b *BufferAnalyzer) reportBufInfo(bufInfo *bufferInfo) {
// 	// fmt.Fprintln(b.logger.Writer(),
// 	// 	"name, time, current, average, period average, capacity")

// 	fmt.Fprintf(
// 		"%s, %d, %.10f, %.10f\n",
// 		b.buffer.buf.Name(),
// 		b.buffer.lastBufLevel,
// 		b.getAverageBufLevel(),
// 		b.getAverageBufLevelInPeriod())
// }

// Report will dump the buffer level information.
func (b *BufferAnalyzer) Report() {
	now := float64(b.timeTeller.CurrentTime())
	b.reportWithTime(now)
}

func (b *BufferAnalyzer) reportWithTime(time float64) {
	vTime := sim.VTimeInSec(time)
	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start: sim.VTimeInSec(vTime),
		End:   sim.VTimeInSec(vTime),
		Where: b.buffer.buf.Name(),
		What:  "lastBufLevel",
		Value: float64(b.buffer.lastBufLevel),
		Unit:  "BuffInfo",
	})

	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start: sim.VTimeInSec(vTime),
		End:   sim.VTimeInSec(vTime),
		Where: b.buffer.buf.Name(),
		What:  "AverageBufLevel",
		Value: b.getAverageBufLevel(),
		Unit:  "BuffInfo",
	})

	b.PerfLogger.AddDataEntry(PerfAnalyzerEntry{
		Start: sim.VTimeInSec(vTime),
		End:   sim.VTimeInSec(vTime),
		Where: b.buffer.buf.Name(),
		What:  "AverageBufLevelInPeriod",
		Value: b.getAverageBufLevelInPeriod(),
		Unit:  "BuffInfo",
	})
}

// WithTimeTeller sets the time teller to be used by the buffer analyzer.
// func (b BufferAnalyzerBuilder) WithTimeTeller(
// 	timeTeller sim.TimeTeller,
// ) BufferAnalyzerBuilder {
// 	b.timeTeller = timeTeller
// 	return b
// }

// WithPeriod sets the period to be used by the buffer analyzer. Sets the period
// to 0 or negative will disable periodic reporting.
// func (b BufferAnalyzerBuilder) WithPeriod(
// 	period float64,
// ) BufferAnalyzerBuilder {
// 	if period > 0 {
// 		b.usePeriod = true
// 		b.period = period
// 	} else {
// 		b.usePeriod = false
// 	}

// 	return b
// }

func (b *BufferAnalyzer) getAverageBufLevelInPeriod() float64 {
	sum := 0.0
	durationSum := 0.0
	for level, duration := range b.buffer.lastPeriodBufLevelToDuration {
		sum += float64(level) * duration
		durationSum += duration
	}

	if durationSum == 0.0 {
		return 0.0
	}

	return sum / durationSum
}

func (b *BufferAnalyzer) getAverageBufLevel() float64 {
	sum := 0.0
	durationSum := 0.0
	for level, duration := range b.buffer.bufLevelToDuration {
		sum += float64(level) * duration
		durationSum += duration
	}

	if durationSum == 0.0 {
		return 0.0
	}

	return sum / durationSum
}

// CreateBuffer creates a buffer to be analyzed
func (b *BufferAnalyzer) CreateBuffer(name string, capacity int) sim.Buffer {
	buf := sim.NewBuffer(name, capacity)

	b.AddBuffer(buf)

	return buf
}
