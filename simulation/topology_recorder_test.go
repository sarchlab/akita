package simulation

import (
	"context"
	"os"
	"testing"

	"github.com/sarchlab/akita/v5/simulation/datarecording"
	"github.com/sarchlab/akita/v5/simulation/messaging"
)

// fakeSpec is a stand-in component spec that serializes to JSON, mirroring the
// real component specs the recorder marshals.
type fakeSpec struct {
	Freq int    `json:"freq"`
	Mode string `json:"mode"`
}

// specComponent is a Component with a Spec field, like every component built
// from a component model.
type specComponent struct {
	name string
	Spec fakeSpec
}

func (c *specComponent) Name() string { return c.name }

// plainComponent is a Component without a Spec field, exercising the
// graceful path where only the name is recorded.
type plainComponent struct {
	name string
}

func (c *plainComponent) Name() string { return c.name }

// portOwner is a named owner of real ports.
type portOwner struct {
	name string
}

func (o *portOwner) Name() string                  { return o.name }
func (o *portOwner) NotifyRecv(messaging.Port)     {}
func (o *portOwner) NotifyPortFree(messaging.Port) {}

// namedConnection is a connection that only has a name; the recorder needs
// nothing else.
type namedConnection struct {
	messaging.Connection
	name string
}

func (c *namedConnection) Name() string { return c.name }

// newOwnedPort creates a real port owned by owner and, unless conn is nil,
// plugged into conn.
func newOwnedPort(name string, owner *portOwner, conn messaging.Connection) messaging.Port {
	p := messaging.NewPort(name, 1, 1)
	p.SetOwner(owner)
	if conn != nil {
		p.SetConnection(conn)
	}

	return p
}

func TestTopologyRecorderRecordsComponentSpecs(t *testing.T) {
	path := "test_topology_recorder_specs"
	dbFile := path + ".sqlite3"
	os.Remove(dbFile)
	defer os.Remove(dbFile)

	recorder := datarecording.NewDataRecorder(path)
	r := newTopologyRecorder(recorder)

	components := []Component{
		&specComponent{name: "L1", Spec: fakeSpec{Freq: 1000, Mode: "write-through"}},
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
		t.Fatalf("expected empty spec for component without a Spec field, got %+v", agent)
	}
}

func TestTopologyRecorderRecordsPorts(t *testing.T) {
	path := "test_topology_recorder_ports"
	dbFile := path + ".sqlite3"
	os.Remove(dbFile)
	defer os.Remove(dbFile)

	recorder := datarecording.NewDataRecorder(path)
	r := newTopologyRecorder(recorder)

	connA := &namedConnection{name: "ConnA"}
	l1, l2 := &portOwner{name: "L1"}, &portOwner{name: "L2"}
	ports := []Port{
		newOwnedPort("L1.Top", l1, connA),
		newOwnedPort("L2.Bottom", l2, connA),
		newOwnedPort("L1.Ctrl", l1, nil),
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
