package simulation

import (
	"context"
	"os"
	"testing"

	"github.com/sarchlab/akita/v5/datarecording"
)

// fakeSpec is a stand-in component spec that serializes to JSON, mirroring the
// real component specs the recorder marshals.
type fakeSpec struct {
	Freq int    `json:"freq"`
	Mode string `json:"mode"`
}

// specComponent is a Component that exposes a Spec accessor, like every
// modeling.Component does.
type specComponent struct {
	name string
	spec fakeSpec
}

func (c *specComponent) Name() string   { return c.name }
func (c *specComponent) Spec() fakeSpec { return c.spec }

// plainComponent is a Component without a Spec accessor, exercising the
// graceful path where only the name is recorded.
type plainComponent struct {
	name string
}

func (c *plainComponent) Name() string { return c.name }

// namedEntity is a minimal named value used as a port's component/connection.
type namedEntity struct {
	name string
}

func (n *namedEntity) Name() string { return n.name }

// fakePort is a Port whose Connection and Component accessors mirror the
// concrete *defaultPort the recorder reflects over. A nil conn models an
// unconnected port.
type fakePort struct {
	name string
	conn *namedEntity
	comp *namedEntity
}

func (p *fakePort) Name() string            { return p.name }
func (p *fakePort) NumIncoming() int        { return 0 }
func (p *fakePort) NumOutgoing() int        { return 0 }
func (p *fakePort) Component() *namedEntity { return p.comp }

// Connection returns the plugged-in connection, or nil when unconnected. The
// nil is returned through the same interface the real port uses, so the
// recorder's nil check is exercised.
func (p *fakePort) Connection() named {
	if p.conn == nil {
		return nil
	}
	return p.conn
}

func TestTopologyRecorderRecordsComponentSpecs(t *testing.T) {
	path := "test_topology_recorder_specs"
	dbFile := path + ".sqlite3"
	os.Remove(dbFile)
	defer os.Remove(dbFile)

	recorder := datarecording.NewDataRecorder(path)
	r := newTopologyRecorder(recorder)

	components := []Component{
		&specComponent{name: "L1", spec: fakeSpec{Freq: 1000, Mode: "write-through"}},
		&plainComponent{name: "Agent"},
	}
	r.Record(components, nil)
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	specs := readComponentSpecs(t, dbFile)
	if len(specs) != 2 {
		t.Fatalf("expected 2 component_spec rows, got %d", len(specs))
	}

	l1 := specs["L1"]
	if l1.Type != "simulation.fakeSpec" {
		t.Fatalf("unexpected spec type %q", l1.Type)
	}
	if l1.Spec != `{"freq":1000,"mode":"write-through"}` {
		t.Fatalf("unexpected spec json %q", l1.Spec)
	}

	agent := specs["Agent"]
	if agent.Type != "" || agent.Spec != "" {
		t.Fatalf("expected empty spec for component without Spec(), got %+v", agent)
	}
}

func TestTopologyRecorderRecordsPorts(t *testing.T) {
	path := "test_topology_recorder_ports"
	dbFile := path + ".sqlite3"
	os.Remove(dbFile)
	defer os.Remove(dbFile)

	recorder := datarecording.NewDataRecorder(path)
	r := newTopologyRecorder(recorder)

	connA := &namedEntity{name: "ConnA"}
	ports := []Port{
		&fakePort{name: "L1.Top", conn: connA, comp: &namedEntity{name: "L1"}},
		&fakePort{name: "L2.Bottom", conn: connA, comp: &namedEntity{name: "L2"}},
		&fakePort{name: "L1.Ctrl", conn: nil, comp: &namedEntity{name: "L1"}},
	}
	r.Record(nil, ports)
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	rows := readPorts(t, dbFile)
	if len(rows) != 3 {
		t.Fatalf("expected 3 port rows (incl. unconnected), got %d", len(rows))
	}

	byPort := make(map[string]portEntry)
	for _, row := range rows {
		byPort[row.Port] = row
	}

	if got := byPort["L1.Top"]; got.Component != "L1" || got.Connection != "ConnA" {
		t.Fatalf("unexpected row for L1.Top: %+v", got)
	}
	if got := byPort["L2.Bottom"]; got.Component != "L2" || got.Connection != "ConnA" {
		t.Fatalf("unexpected row for L2.Bottom: %+v", got)
	}

	// The unconnected port must still be recorded, with an empty Connection, so
	// the table is a complete port inventory rather than just the edge list.
	ctrl, ok := byPort["L1.Ctrl"]
	if !ok {
		t.Fatal("unconnected port should still be recorded")
	}
	if ctrl.Component != "L1" || ctrl.Connection != "" {
		t.Fatalf("unconnected port should have empty Connection: %+v", ctrl)
	}
}

func readComponentSpecs(
	t *testing.T,
	dbFile string,
) map[string]componentSpecEntry {
	t.Helper()

	reader := datarecording.NewReader(dbFile)
	defer reader.Close()
	reader.MapTable(componentSpecTableName, componentSpecEntry{})

	results, _, err := reader.Query(
		context.Background(), componentSpecTableName, datarecording.QueryParams{})
	if err != nil {
		t.Fatalf("query component_spec: %v", err)
	}

	specs := make(map[string]componentSpecEntry)
	for _, result := range results {
		entry, ok := result.(*componentSpecEntry)
		if !ok {
			t.Fatalf("unexpected result type %T", result)
		}
		specs[entry.Name] = *entry
	}

	return specs
}

func readPorts(
	t *testing.T,
	dbFile string,
) []portEntry {
	t.Helper()

	reader := datarecording.NewReader(dbFile)
	defer reader.Close()
	reader.MapTable(portTableName, portEntry{})

	results, _, err := reader.Query(
		context.Background(), portTableName, datarecording.QueryParams{})
	if err != nil {
		t.Fatalf("query port: %v", err)
	}

	rows := make([]portEntry, 0, len(results))
	for _, result := range results {
		entry, ok := result.(*portEntry)
		if !ok {
			t.Fatalf("unexpected result type %T", result)
		}
		rows = append(rows, *entry)
	}

	return rows
}
