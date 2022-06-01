package bottleneckanalysis

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"unsafe"

	"gitlab.com/akita/akita/v3/sim"
)

// BufferAnalyzer can use buffer levels to analyze the bottleneck of the system.
type BufferAnalyzer struct {
	dirPath    string
	timeTeller sim.TimeTeller
	lastTime   float64
	period     float64
	usePeriod  bool

	buffers map[string]*bufferInfo // map of buffer name to buffer

	bufferListFileOpen   bool
	bufferListFile       *os.File
	currentTraceFileOpen bool
	currentTraceFile     *os.File
}

type bufferInfo struct {
	id                           int
	buf                          sim.Buffer
	lastBufLevel                 int
	lastRecordTime               float64
	bufLevelToDuration           map[int]float64
	lastPeriodBufLevelToDuration map[int]float64
	activeInLastPeriod           bool
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

// CreateBuffer creates a buffer to be analyzed
func (b *BufferAnalyzer) CreateBuffer(name string, capacity int) sim.Buffer {
	buf := sim.NewBuffer(name, capacity)

	b.AddBuffer(buf)

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

// AddBuffer adds a buffer to be analyzed.
func (b *BufferAnalyzer) AddBuffer(buf sim.Buffer) {
	buf.AcceptHook(b)
	bufInfo := &bufferInfo{
		id:                           len(b.buffers),
		buf:                          buf,
		lastBufLevel:                 buf.Size(),
		lastRecordTime:               float64(b.timeTeller.CurrentTime()),
		bufLevelToDuration:           make(map[int]float64),
		lastPeriodBufLevelToDuration: make(map[int]float64),
	}
	b.buffers[buf.Name()] = bufInfo

	fmt.Fprintf(b.bufferListFile, "%d,%s,%d\n",
		bufInfo.id, buf.Name(), buf.Capacity())
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

// Func is a function that records buffer level change.
func (b *BufferAnalyzer) Func(ctx sim.HookCtx) {
	if b.bufferListFileOpen {
		err := b.bufferListFile.Close()
		if err != nil {
			panic(err)
		}

		b.bufferListFileOpen = false
	}

	buf := ctx.Domain.(sim.Buffer)
	now := float64(b.timeTeller.CurrentTime())

	if b.usePeriod {
		if math.Floor(now/b.period) != math.Floor(b.lastTime/b.period) {
			b.reportPeriod()
			b.resetPeriod()
		}
	}

	bufInfo, ok := b.buffers[buf.Name()]
	if !ok {
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

func (b *BufferAnalyzer) getBufAverageLevel(name string) float64 {
	return b.buffers[name].getAverageBufLevel()
}

func (b *BufferAnalyzer) getInPeriodBufAverageLevel(name string) float64 {
	if !b.usePeriod {
		panic("period mode not enabled")
	}

	return b.buffers[name].getAverageBufLevelInPeriod()
}

func (b *BufferAnalyzer) lastPeriodEndTime(now float64) float64 {
	return math.Floor(now/b.period) * b.period
}

func (b *BufferAnalyzer) reportPeriod() {
	now := float64(b.timeTeller.CurrentTime())
	lastPeriodEndTime := b.lastPeriodEndTime(now)

	for _, bufInfo := range b.buffers {
		b.terminatePeriod(bufInfo, lastPeriodEndTime)
	}

	b.dumpPeriodReport(lastPeriodEndTime)
}

func (b *BufferAnalyzer) terminatePeriod(
	bufInfo *bufferInfo,
	periodEndTime float64,
) {
	bufInfo.bufLevelToDuration[bufInfo.lastBufLevel] +=
		periodEndTime - bufInfo.lastRecordTime
	bufInfo.lastRecordTime = periodEndTime
}

func (b *BufferAnalyzer) resetPeriod() {
	now := float64(b.timeTeller.CurrentTime())
	for _, bufInfo := range b.buffers {
		bufInfo.lastPeriodBufLevelToDuration = make(map[int]float64)
		bufInfo.lastBufLevel = bufInfo.buf.Size()
		bufInfo.lastRecordTime = now
		bufInfo.activeInLastPeriod = false
	}
}

func (b *BufferAnalyzer) dumpPeriodReport(time float64) {
	b.changeTraceFile(time)

	for _, bufInfo := range b.buffers {
		if !bufInfo.activeInLastPeriod {
			continue
		}

		b.reportBufInfo(bufInfo)
	}
}

func (b *BufferAnalyzer) changeTraceFile(time float64) {
	var err error

	if b.currentTraceFileOpen {
		err = b.currentTraceFile.Close()
		if err != nil {
			panic(err)
		}
	}

	b.currentTraceFile, err = os.OpenFile(
		fmt.Sprintf("%s/buffer_level_%.10f.csv", b.dirPath, time),
		os.O_CREATE|os.O_WRONLY,
		0644,
	)

	if err != nil {
		panic(err)
	}

	b.currentTraceFileOpen = true
}

func (b *BufferAnalyzer) reportWithTime(time float64) {
	b.changeTraceFile(time)

	for _, bufInfo := range b.buffers {
		b.reportBufInfo(bufInfo)
	}
}

func (b *BufferAnalyzer) reportBufInfo(bufInfo *bufferInfo) {
	// fmt.Fprintln(b.logger.Writer(),
	// 	"name, time, current, average, period average, capacity")

	fmt.Fprintf(b.currentTraceFile,
		"%d, %d, %.10f, %.10f\n",
		bufInfo.id,
		bufInfo.lastBufLevel,
		bufInfo.getAverageBufLevel(),
		bufInfo.getAverageBufLevelInPeriod())
}

// Report will dump the buffer level information.
func (b *BufferAnalyzer) Report() {
	now := float64(b.timeTeller.CurrentTime())
	b.reportWithTime(now)
}

// BufferAnalyzerBuilder can build an buffer analyzer
type BufferAnalyzerBuilder struct {
	dirPath    string
	timeTeller sim.TimeTeller
	usePeriod  bool
	period     float64
}

// MakeBufferAnalyzerBuilder returns a new BufferAnalyzerBuilder with default
// parameters.
func MakeBufferAnalyzerBuilder() BufferAnalyzerBuilder {
	return BufferAnalyzerBuilder{
		dirPath:    ".",
		timeTeller: nil,
		usePeriod:  false,
		period:     0.0,
	}
}

// WithTimeTeller sets the time teller to be used by the buffer analyzer.
func (b BufferAnalyzerBuilder) WithTimeTeller(
	timeTeller sim.TimeTeller,
) BufferAnalyzerBuilder {
	b.timeTeller = timeTeller
	return b
}

// WithDirectoryPath sets the directory path that stores the
func (b BufferAnalyzerBuilder) WithDirectoryPath(
	path string,
) BufferAnalyzerBuilder {
	b.dirPath = path
	return b
}

// WithPeriod sets the period to be used by the buffer analyzer. Sets the period
// to 0 or negative will disable periodic reporting.
func (b BufferAnalyzerBuilder) WithPeriod(
	period float64,
) BufferAnalyzerBuilder {
	if period > 0 {
		b.usePeriod = true
		b.period = period
	} else {
		b.usePeriod = false
	}

	return b
}

// Build with build the buffer analyzer.
func (b BufferAnalyzerBuilder) Build() *BufferAnalyzer {
	var err error

	if !filepath.IsAbs(b.dirPath) {
		b.dirPath, err = filepath.Abs(b.dirPath)
		if err != nil {
			panic(err)
		}
	}

	b.prepareDirectory(err)

	ba := &BufferAnalyzer{
		dirPath:    b.dirPath,
		timeTeller: b.timeTeller,
		usePeriod:  b.usePeriod,
		period:     b.period,
		lastTime:   0.0,
		buffers:    make(map[string]*bufferInfo),
	}

	ba.bufferListFile, err = os.OpenFile(
		ba.dirPath+"/buffer_list.csv",
		os.O_CREATE|os.O_WRONLY,
		0644)
	if err != nil {
		panic(err)
	}
	ba.bufferListFileOpen = true

	return ba
}

func (b BufferAnalyzerBuilder) prepareDirectory(err error) {
	err = os.MkdirAll(b.dirPath, 0755)
	if err != nil {
		panic(err)
	}

	file, err := filepath.Glob("buffer*.csv")
	if err != nil {
		panic(err)
	}
	for _, f := range file {
		err = os.Remove(f)
		if err != nil {
			panic(err)
		}
	}
}
