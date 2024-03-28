package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	// Enable profiling
	_ "net/http/pprof"

	"github.com/google/pprof/profile"
	"github.com/gorilla/mux"
	"github.com/sarchlab/akita/v3/analysis"
	"github.com/sarchlab/akita/v3/monitoring/web"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/shirou/gopsutil/process"
	"github.com/syifan/goseth"
)

// Monitor can turn a simulation into a server and allows external monitoring
// controlling of the simulation.
type Monitor struct {
	engine       sim.Engine
	components   []sim.Component
	buffers      []sim.Buffer
	portNumber   int
	perfAnalyzer *analysis.PerfAnalyzer

	progressBarsLock sync.Mutex
	progressBars     []*ProgressBar
}

// NewMonitor creates a new Monitor
func NewMonitor() *Monitor {
	return &Monitor{}
}

// WithPortNumber sets the port number of the monitor.
func (m *Monitor) WithPortNumber(portNumber int) *Monitor {
	if portNumber < 1000 {
		fmt.Fprintf(os.Stderr,
			"Port number %d is assigned to the monitoring server, "+
				"which is not allowed. Using a random port instead.\n", portNumber)
		portNumber = 0
	}

	m.portNumber = portNumber

	return m
}

// RegisterEngine registers the engine that is used in the simulation.
func (m *Monitor) RegisterEngine(e sim.Engine) {
	m.engine = e
}

// RegisterPerfAnalyzer sets the performance analyzer to be used in the monitor.
func (m *Monitor) RegisterPerfAnalyzer(pa *analysis.PerfAnalyzer) {
	m.perfAnalyzer = pa
}

// RegisterComponent register a component to be monitored.
func (m *Monitor) RegisterComponent(c sim.Component) {
	m.components = append(m.components, c)

	m.registerBuffers(c)
}

func (m *Monitor) registerBuffers(c sim.Component) {
	m.registerComponentOrPortBuffers(c)

	for _, p := range c.Ports() {
		m.registerComponentOrPortBuffers(p)
	}
}

func (m *Monitor) registerComponentOrPortBuffers(c any) {
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
			m.buffers = append(m.buffers, fieledRef)
		}
	}
}

// CreateProgressBar creates a new progress bar.
func (m *Monitor) CreateProgressBar(name string, total uint64) *ProgressBar {
	bar := &ProgressBar{
		ID:    sim.GetIDGenerator().Generate(),
		Name:  name,
		Total: total,
	}

	m.progressBarsLock.Lock()
	defer m.progressBarsLock.Unlock()

	m.progressBars = append(m.progressBars, bar)

	return bar
}

// CompleteProgressBar removes a bar to be shown on the webpage.
func (m *Monitor) CompleteProgressBar(pb *ProgressBar) {
	m.progressBarsLock.Lock()
	defer m.progressBarsLock.Unlock()

	newBars := make([]*ProgressBar, 0, len(m.progressBars)-1)
	for _, b := range m.progressBars {
		if b != pb {
			newBars = append(newBars, b)
		}
	}

	m.progressBars = newBars
}

// StartServer starts the monitor as a web server with a custom port if wanted.
func (m *Monitor) StartServer() {
	r := mux.NewRouter()

	fs := web.GetAssets()
	fServer := http.FileServer(fs)
	r.HandleFunc("/api/pause", m.pauseEngine)
	r.HandleFunc("/api/continue", m.continueEngine)
	r.HandleFunc("/api/now", m.now)
	r.HandleFunc("/api/run", m.run)
	r.HandleFunc("/api/tick/{name}", m.tick)
	r.HandleFunc("/api/list_components", m.listComponents)
	r.HandleFunc("/api/component/{name}", m.listComponentDetails)
	r.HandleFunc("/api/field/{json}", m.listFieldValue)
	r.HandleFunc("/api/hangdetector/buffers", m.hangDetectorBuffers)
	r.HandleFunc("/api/progress", m.listProgressBars)
	r.HandleFunc("/api/resource", m.listResources)
	r.HandleFunc("/api/profile", m.collectProfile)
	r.HandleFunc("/api/traffic/{name}", m.reportTraffic)
	r.PathPrefix("/").Handler(fServer)
	http.Handle("/", r)

	actualPort := ":0"
	if m.portNumber > 1000 {
		actualPort = ":" + strconv.Itoa(m.portNumber)
	}

	listener, err := net.Listen("tcp", actualPort)
	dieOnErr(err)

	fmt.Fprintf(
		os.Stderr,
		"Monitoring simulation with http://localhost:%d\n",
		listener.Addr().(*net.TCPAddr).Port)

	go func() {
		err = http.Serve(listener, nil)
		dieOnErr(err)
	}()
}

func (m *Monitor) pauseEngine(w http.ResponseWriter, _ *http.Request) {
	m.engine.Pause()
	_, err := w.Write(nil)
	dieOnErr(err)
}

func (m *Monitor) continueEngine(w http.ResponseWriter, _ *http.Request) {
	m.engine.Continue()
	_, err := w.Write(nil)
	dieOnErr(err)
}

func (m *Monitor) now(w http.ResponseWriter, _ *http.Request) {
	now := m.engine.CurrentTime()
	fmt.Fprintf(w, "{\"now\":%.10f}", now)
}

func (m *Monitor) run(_ http.ResponseWriter, _ *http.Request) {
	go func() {
		err := m.engine.Run()
		if err != nil {
			panic(err)
		}
	}()
}

func (m *Monitor) listComponents(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprint(w, "[")
	for i, c := range m.components {
		if i > 0 {
			fmt.Fprint(w, ",")
		}

		fmt.Fprintf(w, "\"%s\"", c.Name())
	}
	fmt.Fprint(w, "]")
}

type tickingComponent interface {
	TickLater(now sim.VTimeInSec)
}

func (m *Monitor) tick(w http.ResponseWriter, r *http.Request) {
	compName := mux.Vars(r)["name"]

	comp := m.findComponentOr404(w, compName)
	if comp == nil {
		return
	}

	tickingComp, ok := comp.(tickingComponent)
	if !ok {
		w.WriteHeader(405)
	}

	tickingComp.TickLater(m.engine.CurrentTime())
	w.WriteHeader(200)
}

func (m *Monitor) listComponentDetails(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	component := m.findComponentOr404(w, name)
	if component == nil {
		return
	}

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)
	err := serializer.Serialize(w)

	dieOnErr(err)
}

type fieldReq struct {
	CompName  string `json:"comp_name,omitempty"`
	FieldName string `json:"field_name,omitempty"`
}

func (m *Monitor) listFieldValue(w http.ResponseWriter, r *http.Request) {
	jsonString := mux.Vars(r)["json"]
	req := fieldReq{}

	err := json.Unmarshal([]byte(jsonString), &req)
	if err != nil {
		dieOnErr(err)
	}

	name := req.CompName
	fields := strings.Split(req.FieldName, ".")

	component := m.findComponentOr404(w, name)
	if component == nil {
		return
	}

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)

	err = serializer.SetEntryPoint(fields)
	dieOnErr(err)

	err = serializer.Serialize(w)
	dieOnErr(err)
}

func (m *Monitor) hangDetectorBuffers(w http.ResponseWriter, r *http.Request) {
	sortMethod, limit, offset, err := m.buffersParseParams(r, w)
	if err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, "Error: %s", err)
		return
	}

	sortedBuffers := m.sortAndSelectBuffers(sortMethod, limit, offset)

	fmt.Fprintf(w, "[")
	for i, b := range sortedBuffers {
		if i > 0 {
			fmt.Fprint(w, ",")
		}

		fmt.Fprintf(w, "{\"buffer\":\"%s\",\"level\":%d,\"cap\":%d}",
			b.Name(), b.Size(), b.Capacity())
	}

	fmt.Fprint(w, "]")
}

func (*Monitor) buffersParseParams(
	r *http.Request,
	_ http.ResponseWriter,
) (sort string, limit, offset int, err error) {
	sortMethod := r.URL.Query().Get("sort")
	if sortMethod == "" {
		sortMethod = "percent"
	}
	if sortMethod != "level" && sortMethod != "percent" {
		errStr := fmt.Sprintf(
			"Invalid sort method: %s. Allowed values are `level` and `percent`",
			sortMethod)
		return "", 0, 0, errors.New(errStr)
	}

	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "0"
	}
	limitNumber, err := strconv.Atoi(limitStr)
	if err != nil {
		return sortMethod, 0, 0, err
	}

	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = "0"
	}
	offsetNumber, err := strconv.Atoi(offsetStr)
	if err != nil {
		return sortMethod, limitNumber, 0, err
	}

	return sortMethod, limitNumber, offsetNumber, nil
}

func bufferPercent(b sim.Buffer) float64 {
	return float64(b.Size()) / float64(b.Capacity())
}

func (m *Monitor) sortAndSelectBuffers(
	sortMethod string,
	limit, offset int,
) []sim.Buffer {
	sortedBuffers := make([]sim.Buffer, len(m.buffers))
	copy(sortedBuffers, m.buffers)

	if sortMethod == "level" {
		sort.Slice(sortedBuffers, func(i, j int) bool {
			sizeI := sortedBuffers[i].Size()
			sizeJ := sortedBuffers[j].Size()
			percentI := bufferPercent(sortedBuffers[i])
			percentJ := bufferPercent(sortedBuffers[j])

			if sizeI > sizeJ {
				return true
			} else if sizeI < sizeJ {
				return false
			} else {
				return percentI > percentJ
			}
		})
	} else if sortMethod == "percent" {
		sort.Slice(sortedBuffers, func(i, j int) bool {
			sizeI := sortedBuffers[i].Size()
			sizeJ := sortedBuffers[j].Size()
			percentI := bufferPercent(sortedBuffers[i])
			percentJ := bufferPercent(sortedBuffers[j])

			if percentI > percentJ {
				return true
			} else if percentI < percentJ {
				return false
			} else {
				return sizeI > sizeJ
			}
		})
	} else {
		panic("Invalid sort method " + sortMethod)
	}

	sortedBuffers = sortedBuffers[offset : offset+limit]

	return sortedBuffers
}

type fieldFormatError struct {
}

func (e fieldFormatError) Error() string {
	return "fieldFormatError"
}

func (m *Monitor) walkFields(
	comp interface{},
	fields string,
) (reflect.Value, error) {
	elem := reflect.ValueOf(comp)

	fieldNames := strings.Split(fields, ".")

	for len(fieldNames) > 0 {
		switch elem.Kind() {
		case reflect.Ptr, reflect.Interface:
			elem = elem.Elem()
		case reflect.Struct:
			elem = elem.FieldByName(fieldNames[0])
			fieldNames = fieldNames[1:]
		case reflect.Slice:
			index, err := strconv.Atoi(fieldNames[0])
			if err != nil {
				return elem, fieldFormatError{}
			}

			elem = elem.Index(index)
			fieldNames = fieldNames[1:]
		default:
			panic(fmt.Sprintf("kind %d not supported", elem.Kind()))
		}
	}

	if elem.Kind() == reflect.Ptr {
		elem = elem.Elem()
	}

	return elem, nil
}

func (m *Monitor) findComponentOr404(
	w http.ResponseWriter,
	name string,
) sim.Component {
	var component sim.Component
	for _, c := range m.components {
		if c.Name() == name {
			component = c
		}
	}

	if component == nil {
		w.WriteHeader(http.StatusNotFound)
		_, err := w.Write([]byte("Component not found"))
		dieOnErr(err)
	}

	return component
}

func (m *Monitor) listProgressBars(w http.ResponseWriter, _ *http.Request) {
	bytes, err := json.Marshal(m.progressBars)
	dieOnErr(err)

	_, err = w.Write(bytes)
	dieOnErr(err)
}

type resourceRsp struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemorySize uint64  `json:"memory_size"`
}

func (m *Monitor) listResources(w http.ResponseWriter, _ *http.Request) {
	pid := os.Getpid()
	process, err := process.NewProcess(int32(pid))
	dieOnErr(err)

	cpuPercent, err := process.CPUPercent()
	dieOnErr(err)

	memorySize, err := process.MemoryInfo()
	dieOnErr(err)

	rsp := resourceRsp{
		CPUPercent: cpuPercent,
		MemorySize: memorySize.RSS,
	}

	bytes, err := json.Marshal(rsp)
	dieOnErr(err)

	_, err = w.Write(bytes)
	dieOnErr(err)
}

func (m *Monitor) collectProfile(w http.ResponseWriter, _ *http.Request) {
	buf := bytes.NewBuffer(nil)

	err := pprof.StartCPUProfile(buf)
	dieOnErr(err)

	time.Sleep(time.Second)

	pprof.StopCPUProfile()

	prof, err := profile.ParseData(buf.Bytes())
	dieOnErr(err)

	bytes, err := json.Marshal(prof)
	dieOnErr(err)

	_, err = w.Write(bytes)
	dieOnErr(err)
}

func dieOnErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) reportTraffic(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	// component := m.findComponentOr404(w, name)
	// if component == nil {
	// 	return
	// }

	backend := m.perfAnalyzer.GetCurrentTraffic(name)

	_, err := w.Write([]byte(backend))
	dieOnErr(err)
}
