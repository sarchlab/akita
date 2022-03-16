package monitoring

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/gorilla/mux"
	"github.com/syifan/goseth"
	"gitlab.com/akita/akita/v3/monitoring/web"
	"gitlab.com/akita/akita/v3/sim"
)

// Monitor can turn a simulation into a server and allows external monitoring
// controlling of the simulation.
type Monitor struct {
	engine     sim.Engine
	components []sim.Component
	buffers    []sim.Buffer

	progressBarsLock sync.Mutex
	progressBars     []*ProgressBar
}

// NewMonitor creates a new Monitor
func NewMonitor() *Monitor {
	return &Monitor{}
}

// RegisterEngine registers the engine that is used in the simulation.
func (m *Monitor) RegisterEngine(e sim.Engine) {
	m.engine = e
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

// StartServer starts the monitor as a web server.
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
	r.HandleFunc("/api/hangdetector/buffers", m.hangdetectorBuffers)
	r.HandleFunc("/api/progress", m.listProgressBars)
	r.PathPrefix("/").Handler(fServer)
	http.Handle("/", r)

	listener, err := net.Listen("tcp", ":0")
	dieOnErr(err)

	fmt.Printf("Monitoring simulation with http://localhost:%d\n",
		listener.Addr().(*net.TCPAddr).Port)

	go func() {
		err = http.Serve(listener, nil)
		dieOnErr(err)
	}()
}

func (m *Monitor) pauseEngine(w http.ResponseWriter, r *http.Request) {
	m.engine.Pause()
	_, err := w.Write(nil)
	dieOnErr(err)
}

func (m *Monitor) continueEngine(w http.ResponseWriter, r *http.Request) {
	m.engine.Continue()
	_, err := w.Write(nil)
	dieOnErr(err)
}

func (m *Monitor) now(w http.ResponseWriter, r *http.Request) {
	now := m.engine.CurrentTime()
	fmt.Fprintf(w, "{\"now\":%.10f}", now)
}

func (m *Monitor) run(w http.ResponseWriter, r *http.Request) {
	go func() {
		err := m.engine.Run()
		if err != nil {
			panic(err)
		}
	}()
}

func (m *Monitor) listComponents(w http.ResponseWriter, r *http.Request) {
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

	serializer := goseth.NewInteractiveSerializer()
	err := serializer.Serialize(component, w)
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
	fields := req.FieldName

	component := m.findComponentOr404(w, name)
	if component == nil {
		return
	}

	elem, err := m.walkFields(component, fields)
	dieOnErr(err)

	serializer := goseth.NewInteractiveSerializer()
	elemCopy := reflect.NewAt(
		elem.Type(), unsafe.Pointer(elem.UnsafeAddr())).Elem()
	err = serializer.Serialize(elemCopy.Interface(), w)
	dieOnErr(err)
}

func (m *Monitor) hangdetectorBuffers(w http.ResponseWriter, r *http.Request) {
	sortedBuffers := make([]sim.Buffer, len(m.buffers))
	copy(sortedBuffers, m.buffers)
	sort.Slice(sortedBuffers, func(i, j int) bool {
		return sortedBuffers[i].Size() > sortedBuffers[j].Size()
	})

	fmt.Fprintf(w, "[")
	for i, b := range sortedBuffers {
		if i > 0 {
			fmt.Fprint(w, ",")
		}

		fmt.Fprintf(w, "{\"buffer\":\"%s\",\"level\":%d,\"cap\":%d}",
			b.Name(), b.Size(), b.Capacity())

		if i > 50 {
			break
		}
	}

	fmt.Fprint(w, "]")
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

func (m *Monitor) listProgressBars(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.Marshal(m.progressBars)
	dieOnErr(err)

	_, err = w.Write(bytes)
	dieOnErr(err)
}

func dieOnErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
