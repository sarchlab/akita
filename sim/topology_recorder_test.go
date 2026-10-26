package sim

import (
	"os"
	"testing"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
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
	p := twowaybuffered.MakeBuilder().WithSimulation(MakeBuilder().Build()).Build()
	p.BindOwner(owner, name)
	if conn != nil {
		p.BindConnection(conn)
	}

	return p
}

func TestTopologyRecorderRecordsComponentSpecs(t *testing.T) {
	path := "test_topology_recorder_specs"
	dbFile := path + ".sqlite3"
	os.Remove(dbFile)
	defer os.Remove(dbFile)

	recorder := datarecording.NewDataRecorder(path + ".sqlite3")
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
		t.Fatalf("expected 2 akita$component_spec rows, got %d", len(specs))
	}

	l1 := specs["L1"]
	if l1.Type != "sim.fakeSpec" {
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

	recorder := datarecording.NewDataRecorder(path + ".sqlite3")
	r := newTopologyRecorder(recorder)

	connA := &namedConnection{name: "ConnA"}
	l1, l2 := &portOwner{name: "L1"}, &portOwner{name: "L2"}
	ports := []messaging.Port{
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

func readComponentSpecs(t *testing.T, dbFile string) map[string]componentSpecEntry {
	t.Helper()
	rows, err := openRecordingDB(t, dbFile).Query("SELECT Name, Type, Spec FROM akita$component_spec")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	specs := make(map[string]componentSpecEntry)
	for rows.Next() {
		var entry componentSpecEntry
		if err := rows.Scan(&entry.Name, &entry.Type, &entry.Spec); err != nil {
			t.Fatal(err)
		}
		specs[entry.Name] = entry
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return specs
}

func readPorts(t *testing.T, dbFile string) []portEntry {
	t.Helper()
	rows, err := openRecordingDB(t, dbFile).Query("SELECT Port, Component, Connection FROM akita$port")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var entries []portEntry
	for rows.Next() {
		var entry portEntry
		if err := rows.Scan(&entry.Port, &entry.Component, &entry.Connection); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func (o *portOwner) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
