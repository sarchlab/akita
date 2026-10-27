package sim

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type testResource struct {
	name     string
	kind     string
	format   string
	ext      string
	identity string
}

// dummyPayloads builds one placeholder archive entry per registered entity, so
// coverage passes and the foundation's "no serializer" path can be exercised.
func dummyPayloads(s *Simulation) []archiveEntry {
	entries := make([]archiveEntry, 0, len(s.entities))
	for _, entity := range s.entities {
		entries = append(entries,
			archiveEntry{name: entity.Name(), data: []byte("{}")})
	}
	return entries
}

func (r testResource) Name() string {
	return r.name
}

func (r testResource) Kind() string {
	return r.kind
}

func (r testResource) Format() string {
	return r.format
}

func (r testResource) FileExtension() string {
	return r.ext
}

func (r testResource) Identity() string {
	return r.identity
}

func (r testResource) Save(io.Writer) error {
	return nil
}

func (r testResource) Load(io.Reader) error {
	return nil
}

type testConnection struct {
	name string
}

func newTestConnection(name string) *testConnection {
	return &testConnection{name: name}
}

func (c *testConnection) Name() string {
	return c.name
}

func (c *testConnection) BindPort(p messaging.Port)    { p.BindConnection(c) }
func (*testConnection) NotifySend(messaging.Port)      {}
func (*testConnection) NotifyAvailable(messaging.Port) {}

type testPort struct {
	*twowaybuffered.Port
	name string
}

func (p testPort) Name() string { return p.name }

type testComponent struct {
	name string
}

func (c testComponent) Name() string {
	return c.name
}

func (c testComponent) BindPort(slot string, p messaging.Port) { p.BindOwner(c, c.name+"."+slot) }
func (testComponent) NotifyRecv(messaging.Port)                {}
func (testComponent) NotifyPortFree(messaging.Port)            {}

func TestSimulation(t *testing.T) {
	var (
		simulation *Simulation
		comp       testComponent
		port       testPort
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		simulation = MakeBuilder().Build()
		port = testPort{Port: twowaybuffered.MakeBuilder().WithSimulation(simulation).Build(), name: "port"}
		comp = testComponent{name: "comp"}
	}

	cleanup2 := func(t *testing.T) {
		t.Helper()

		simulation.Terminate()

		os.Remove("akita_sim_" + simulation.ID() + ".sqlite3")
	}

	t.Run("should register a component", func(t *testing.T) {
		t.Cleanup(func() { cleanup2(t) })
		setup1(t)

		simulation.RegisterComponent(comp)
		simulation.RegisterPort(port)

		require.Equal(t, []Component{comp}, simulation.components)
		require.Equal(t, []messaging.Port{port}, simulation.ports)
	})

	t.Run("should reject duplicate component names", func(t *testing.T) {
		t.Cleanup(func() { cleanup2(t) })
		setup1(t)

		simulation.RegisterComponent(comp)

		dup := testComponent{name: "comp"}

		requirePanicContains(t, "already registered", func() {
			simulation.RegisterComponent(dup)
		})
	})

	t.Run("should reject duplicate port names", func(t *testing.T) {
		t.Cleanup(func() { cleanup2(t) })
		setup1(t)

		simulation.RegisterPort(port)

		dupPort := testPort{Port: twowaybuffered.MakeBuilder().WithSimulation(simulation).Build(), name: "port"}

		requirePanicContains(t, "already registered", func() {
			simulation.RegisterPort(dupPort)
		})
	})

	t.Run("should register shared state resources directly", func(t *testing.T) {
		t.Cleanup(func() { cleanup2(t) })
		setup1(t)

		resource := testResource{
			name:     "Program.Memory",
			kind:     "test.Resource",
			format:   "json",
			ext:      ".json",
			identity: "resource-1",
		}

		simulation.RegisterResource(resource)

		require.Contains(t, simulation.entityByName, "Program.Memory")
	})

	t.Run("should reject duplicate shared state names with different identities", func(t *testing.T) {
		t.Cleanup(func() { cleanup2(t) })
		setup1(t)

		resource := testResource{
			name:     "Program.Memory",
			kind:     "test.Resource",
			format:   "json",
			ext:      ".json",
			identity: "resource-1",
		}
		duplicate := resource
		duplicate.identity = "resource-2"

		simulation.RegisterResource(resource)

		requirePanicContains(t, "already registered", func() {
			simulation.RegisterResource(duplicate)
		})
	})

	t.Run("should return all registered entities", func(t *testing.T) {
		t.Cleanup(func() { cleanup2(t) })
		setup1(t)

		resource := testResource{
			name:     "Program.Memory",
			kind:     "test.Resource",
			format:   "json",
			ext:      ".json",
			identity: "resource-1",
		}
		connection := newTestConnection("conn")

		simulation.RegisterComponent(comp)
		simulation.RegisterPort(port)
		simulation.RegisterConnection(connection)
		simulation.RegisterResource(resource)

		entities := simulation.entities
		names := make([]string, 0, len(entities))
		for _, e := range entities {
			names = append(names, e.Name())
		}

		require.ElementsMatch(t, []string{"Engine", "IDGenerator", "comp", "port", "conn", "Program.Memory"}, names)
	})

	t.Run("Builder with custom output file", func(t *testing.T) {
		var customSim *Simulation

		cleanup3 := func(t *testing.T) {
			t.Helper()

			if customSim != nil {
				customSim.Terminate()
				customSim = nil
			}
		}

		t.Run("should allow custom output file to be set", func(t *testing.T) {
			// TempDir comes before the cleanups, so the simulation is
			// terminated before the recording's directory is removed.
			path := filepath.Join(t.TempDir(), "custom_output.sqlite3")
			t.Cleanup(func() { cleanup2(t) })
			t.Cleanup(func() { cleanup3(t) })
			setup1(t)

			builder := MakeBuilder().
				WithOutputPath(path)
			customSim = builder.Build()

			require.NotNil(t, customSim)
			require.NotNil(t, customSim.DataRecorder())
		})
	})

	t.Run("checkpoint foundation", func(t *testing.T) {
		t.Run("should reject an entity that has no serializer", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

			// A bare component/port has no checkpoint serializer yet, so save
			// must fail loudly and not leave an archive behind.
			noSerializerSim := MakeBuilder().Build()
			defer func() {
				noSerializerSim.Terminate()
				os.Remove("akita_sim_" + noSerializerSim.ID() + ".sqlite3")
			}()
			noSerializerSim.RegisterComponent(testComponent{name: "comp"})
			require.NoError(t, noSerializerSim.Initialize())

			path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")

			err := noSerializerSim.SaveCheckpoint(path)

			require.ErrorContains(t, err, "has no checkpoint serializer")
			_, statErr := os.Stat(path)
			require.True(t, os.IsNotExist(statErr))
		})

		t.Run("should reject checkpoints for parallel engines", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

			parallelSim := MakeBuilder().
				WithParallelEngine().
				Build()
			defer func() {
				parallelSim.Terminate()
				os.Remove("akita_sim_" + parallelSim.ID() + ".sqlite3")
			}()

			path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")

			require.NoError(t, parallelSim.Initialize())
			err := parallelSim.SaveCheckpoint(path)

			require.ErrorContains(t, err, "only timing.SerialEngine is supported")
		})

		t.Run("should reject when a rebuilt entity is missing from the checkpoint", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

			path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")

			entries := []archiveEntry{}
			for _, entity := range simulation.entities {
				if entity.Name() == "IDGenerator" {
					continue
				}
				entries = append(entries,
					archiveEntry{name: entity.Name(), data: []byte("{}")})
			}

			err := writeArchive(path, entries)
			require.NoError(t, err)

			require.NoError(t, simulation.Initialize())
			err = simulation.LoadCheckpoint(path)

			require.ErrorContains(t, err, "rebuilt entity \"IDGenerator\" is missing from checkpoint")
		})

		t.Run("should reject a checkpoint that carries an entity the sim does not rebuild", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

			path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")

			entries := append(dummyPayloads(simulation),
				archiveEntry{name: "GhostEntity", data: []byte("{}")})

			err := writeArchive(path, entries)
			require.NoError(t, err)

			require.NoError(t, simulation.Initialize())
			err = simulation.LoadCheckpoint(path)

			require.ErrorContains(t, err, "saved entity \"GhostEntity\" is not rebuilt")
		})

		t.Run("should reject a corrupted archive", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

			path := filepath.Join(t.TempDir(), "corrupt.tar.gz")
			require.NoError(t, os.WriteFile(
				path, []byte("this is not a gzip archive"), 0o644))

			err := simulation.LoadCheckpoint(path)

			require.Error(t, err)
		})

		t.Run("should reject a truncated archive", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

			path := filepath.Join(t.TempDir(), "truncated.tar.gz")
			require.NoError(t, writeArchive(
				path, dummyPayloads(simulation)))

			full, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Greater(t, len(full), 4)
			require.NoError(t, os.WriteFile(path, full[:len(full)/2], 0o644))

			require.NoError(t, simulation.Initialize())
			err = simulation.LoadCheckpoint(path)

			require.Error(t, err)
		})
	})
}

func TestGlobalStateManager(t *testing.T) {
	var sim *Simulation

	setup4 := func(t *testing.T) {
		t.Helper()

		sim = MakeBuilder().Build()
	}

	cleanup5 := func(t *testing.T) {
		t.Helper()

		sim.Terminate()
		os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
	}

	t.Run("Global name uniqueness", func(t *testing.T) {
		t.Run("should reject a connection whose name collides with a component", func(t *testing.T) {
			t.Cleanup(func() { cleanup5(t) })
			setup4(t)

			sim.RegisterComponent(testComponent{name: "shared"})

			requirePanicContains(t, "already registered", func() {
				sim.RegisterConnection(newTestConnection("shared"))
			})
		})

		t.Run("should reject a resource whose name collides with a component", func(t *testing.T) {
			t.Cleanup(func() { cleanup5(t) })
			setup4(t)

			sim.RegisterComponent(testComponent{name: "Program.Memory"})

			requirePanicContains(t, "already registered", func() {
				sim.RegisterResource(testResource{
					name:     "Program.Memory",
					kind:     "test.Resource",
					format:   "json",
					ext:      ".json",
					identity: "resource-1",
				})
			})
		})
	})

	t.Run("Deterministic entity inventory", func(t *testing.T) {
		t.Run("should list entities in stable registration order across rebuilds", func(t *testing.T) {
			t.Cleanup(func() { cleanup5(t) })
			setup4(t)

			build := func() []string {
				s := MakeBuilder().Build()
				defer func() {
					s.Terminate()
					os.Remove("akita_sim_" + s.ID() + ".sqlite3")
				}()

				s.RegisterComponent(testComponent{name: "GPU[1]"})
				s.RegisterComponent(testComponent{name: "GPU[2]"})
				s.RegisterConnection(newTestConnection("GPU[1].GPU[2].Conn"))

				entities := s.entities
				names := make([]string, 0, len(entities))
				for _, e := range entities {
					names = append(names, e.Name())
				}

				return names
			}

			require.Equal(t, build(), build())
		})
	})

	t.Run("Complete state inventory", func(t *testing.T) {
		t.Run("registers the engine and ID generator as entities", func(t *testing.T) {
			t.Cleanup(func() { cleanup5(t) })
			setup4(t)

			entities := sim.entities
			names := make([]string, 0, len(entities))
			for _, e := range entities {
				names = append(names, e.Name())
			}

			require.Subset(t, names, []string{"Engine", "IDGenerator"})
		})
	})
}

type roundTripSpec struct {
	Freq    timing.Freq `json:"freq"`
	Latency int         `json:"latency"`
}

type roundTripState struct {
	Count int `json:"count"`
}

// noPorts and noMiddlewares shape the test components that need neither.
type (
	noPorts       struct{}
	noMiddlewares struct{}
)

// roundTripDefaultSpec returns the component's default configuration.
func roundTripDefaultSpec() roundTripSpec {
	return roundTripSpec{Freq: 1 * timing.GHz, Latency: 5}
}

// timeSetter handles an event by doing nothing, so running an event for it
// only moves the engine's clock to the event's time.
type timeSetter struct{}

func (timeSetter) Handle(timing.Event) {}

// advanceTo moves the engine's clock forward to at with an empty event.
func advanceTo(t *testing.T, engine *timing.SerialEngine, at timing.VTimeInPicoSec) {
	t.Helper()

	engine.Schedule(timing.MakeEventBase(0, at, "TimeSetter"))
	require.NoError(t, engine.Run())
}

func TestCheckpointRoundTrip(t *testing.T) {
	t.Run("restores component state, storage, ID counter, and engine time", func(t *testing.T) {
		sim := MakeBuilder().Build()
		defer func() {
			sim.Terminate()
			os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
		}()

		// A port-less component plus a storage resource: every entity
		// (Engine, IDGenerator, Comp, Mem) is checkpointable, so no port or
		// connection serializers are needed yet.
		engine := sim.Engine().(*timing.SerialEngine)
		engine.RegisterHandler("TimeSetter", timeSetter{})
		comp := modeling.MakeBuilder[*ticking.Component[
			roundTripSpec, roundTripState, modeling.None, noPorts, noMiddlewares]](roundTripDefaultSpec()).
			WithSimulation(sim).Build("Comp")
		storage := mem.MakeStorageBuilder().
			WithCapacity(4 * mem.KB).
			WithSimulation(sim).
			Build("Mem")
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		// Establish runtime state across all four entity kinds.
		comp.State = roundTripState{Count: 7}
		storage.Write(0, []byte{1, 2, 3, 4})
		var savedCounter uint64
		for i := 0; i < 5; i++ {
			savedCounter = sim.NewID()
		}
		advanceTo(t, engine, 100)

		path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
		require.NoError(t, sim.SaveCheckpoint(path))

		// Mutate every piece of runtime state away from the checkpoint.
		comp.State = roundTripState{Count: 999}
		storage.Write(0, []byte{0, 0, 0, 0})
		sim.NewID()
		sim.NewID()
		advanceTo(t, engine, 500)

		// Restore and confirm every piece came back.
		require.NoError(t, sim.LoadCheckpoint(path))

		require.Equal(t, 7, comp.State.Count)
		data := storage.Read(0, 4)
		require.Equal(t, []byte{1, 2, 3, 4}, data)
		require.Equal(t, savedCounter+1, sim.NewID())
		require.Equal(t, timing.VTimeInPicoSec(100), engine.CurrentTime())
	})
}

type resumeSpec struct {
	Freq timing.Freq `json:"freq"`
	N    int         `json:"n"`
}

type resumeState struct {
	Pending  int    `json:"pending"`
	Done     int    `json:"done"`
	Checksum uint64 `json:"checksum"`
}

type resumeMWs struct {
	Worker *resumeWorkerMW
}

type resumeComp = ticking.Component[
	resumeSpec, resumeState, modeling.None, noPorts, resumeMWs]

type resumeWorkerMW struct {
	comp *resumeComp
}

func (m *resumeWorkerMW) Handle(timing.Event) bool {
	if m.comp.State.Pending <= 0 {
		return false
	}
	m.comp.State.Done++
	m.comp.State.Checksum = m.comp.State.Checksum*1000003 + uint64(m.comp.State.Done)
	m.comp.State.Pending--
	return true
}

// resumeDefaultSpec returns the component's default configuration.
func resumeDefaultSpec() resumeSpec {
	return resumeSpec{Freq: 1 * timing.GHz, N: 1}
}

func buildResumeSim() (*Simulation, *resumeComp) {
	sim := MakeBuilder().Build()
	c := modeling.MakeBuilder[*resumeComp](resumeDefaultSpec()).WithSimulation(sim).Build("Worker")
	if err := sim.Initialize(); err != nil {
		panic(err)
	}
	return sim, c
}

func TestMidTransactionResume(t *testing.T) {
	t.Run("resumes from a checkpoint with a pending tick identically to running uninterrupted", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "ck.tar.gz")

		// Reference run: set up mid-transaction state (work pending, one tick
		// already scheduled), checkpoint there, then continue to completion.
		refSim, refW := buildResumeSim()
		defer func() {
			refSim.Terminate()
			os.Remove("akita_sim_" + refSim.ID() + ".sqlite3")
		}()
		refW.State = resumeState{Pending: 5}
		refW.TickLater() // schedule the first tick -> non-empty engine queue

		require.NoError(t, refSim.SaveCheckpoint(path))

		refEngine := refSim.Engine().(*timing.SerialEngine)
		require.NoError(t, refEngine.Run())
		wantDone := refW.State.Done
		wantChecksum := refW.State.Checksum
		wantTime := refEngine.CurrentTime()

		// Resumed run: fresh sim, load the mid-transaction checkpoint, run to
		// completion. The pending tick comes from the restored queue.
		resSim, resW := buildResumeSim()
		defer func() {
			resSim.Terminate()
			os.Remove("akita_sim_" + resSim.ID() + ".sqlite3")
		}()
		require.NoError(t, resSim.LoadCheckpoint(path))

		resEngine := resSim.Engine().(*timing.SerialEngine)
		require.NoError(t, resEngine.Run())

		require.Equal(t, wantDone, resW.State.Done)
		require.Equal(t, wantChecksum, resW.State.Checksum)
		require.Equal(t, wantTime, resEngine.CurrentTime())
		require.Equal(t, 5, wantDone) // sanity: work actually happened
	})
}

type tickCountSpec struct {
	Freq timing.Freq `json:"freq"`
	Tag  int         `json:"tag"`
}

type tickCountState struct {
	Ticks int `json:"ticks"`
}

// tickCountMW counts every tick and never reports progress, so the component
// chains no follow-up ticks: exactly the ticks present in the engine queue fire.
type tickCountMWs struct {
	Counter *tickCountMW
}

type tickCountComp = ticking.Component[
	tickCountSpec, tickCountState, modeling.None, noPorts, tickCountMWs]

type tickCountMW struct {
	comp *tickCountComp
}

func (m *tickCountMW) Handle(timing.Event) bool {
	m.comp.State.Ticks++
	return false
}

// tickCountDefaultSpec returns the component's default configuration.
func tickCountDefaultSpec() tickCountSpec {
	return tickCountSpec{Freq: 1 * timing.GHz, Tag: 1}
}

func buildTickCountSim() (*Simulation, *tickCountComp) {
	sim := MakeBuilder().Build()
	c := modeling.MakeBuilder[*tickCountComp](tickCountDefaultSpec()).WithSimulation(sim).Build("Ticker")
	if err := sim.Initialize(); err != nil {
		panic(err)
	}
	return sim, c
}

func TestTickSchedulerGuardRestore(t *testing.T) {
	t.Run("restores the guard from the checkpoint so a stimulus before the "+
		"pending tick fires schedules no duplicate", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "ck.tar.gz")

		// Checkpoint with exactly one tick pending in the engine queue and the
		// engine still at time 0.
		srcSim, srcC := buildTickCountSim()
		defer func() {
			srcSim.Terminate()
			os.Remove("akita_sim_" + srcSim.ID() + ".sqlite3")
		}()
		srcC.TickLater()
		require.NoError(t, srcSim.SaveCheckpoint(path))

		// Restore into a fresh sim. The component's guard is restored directly
		// from the checkpoint, so it already agrees that a tick is scheduled.
		dstSim, dstC := buildTickCountSim()
		defer func() {
			dstSim.Terminate()
			os.Remove("akita_sim_" + dstSim.ID() + ".sqlite3")
		}()
		require.NoError(t, dstSim.LoadCheckpoint(path))

		// A stimulus (what NotifyRecv does) arrives before the pending tick
		// fires. With the guard restored, this is recognized as redundant and
		// schedules no second tick at the same cycle.
		dstC.TickLater()

		engine := dstSim.Engine().(*timing.SerialEngine)
		require.NoError(t, engine.Run())

		// Exactly one tick fired. Without the restored guard the stimulus would
		// have scheduled a duplicate (Ticks == 2); a dropped pending tick
		// would be 0.
		require.Equal(t, 1, dstC.State.Ticks)
	})
}

func TestTickSchedulerRestoresAFutureTick(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const tickTime = timing.VTimeInPicoSec(5000)

	// Checkpoint with a tick pending several cycles ahead.
	srcSim, srcC := buildTickCountSim()
	defer func() {
		srcSim.Terminate()
		os.Remove("akita_sim_" + srcSim.ID() + ".sqlite3")
	}()
	srcC.TickAt(tickTime)
	require.NoError(t, srcSim.SaveCheckpoint(path))

	dstSim, dstC := buildTickCountSim()
	defer func() {
		dstSim.Terminate()
		os.Remove("akita_sim_" + dstSim.ID() + ".sqlite3")
	}()
	require.NoError(t, dstSim.LoadCheckpoint(path))

	// The restored guard knows the future tick is pending, so asking for it
	// again schedules nothing, while a tick next cycle is still scheduled.
	dstC.TickAt(tickTime)
	dstC.TickLater()

	engine := dstSim.Engine().(*timing.SerialEngine)
	require.NoError(t, engine.Run())

	// The next-cycle tick and the restored tick fired, the latter at the
	// saved time. A duplicate would make three ticks.
	require.Equal(t, 2, dstC.State.Ticks)
	require.Equal(t, tickTime, engine.CurrentTime())
}

type wakeSpec struct {
	Tag int `json:"tag"`
}

type wakeState struct {
	Wakeups int `json:"wakeups"`
}

type wakeMWs struct {
	Counter *wakeMW
}

type wakeComp = wakeup.Component[wakeSpec, wakeState, modeling.None, noPorts, wakeMWs]

// wakeMW counts every wakeup and makes no progress, so the component asks for
// no follow-up and exactly the wakeups present in the engine queue fire.
type wakeMW struct {
	comp *wakeComp
}

func (m *wakeMW) Handle(timing.Event) bool {
	m.comp.State.Wakeups++
	return false
}

// wakeDefaultSpec returns the component's default configuration.
func wakeDefaultSpec() wakeSpec {
	return wakeSpec{Tag: 1}
}

func buildWakeSim() (*Simulation, *wakeComp) {
	sim := MakeBuilder().Build()
	c := modeling.MakeBuilder[*wakeComp](wakeDefaultSpec()).WithSimulation(sim).Build("Waker")
	if err := sim.Initialize(); err != nil {
		panic(err)
	}
	return sim, c
}

func TestEventDrivenWakeupGuardRestore(t *testing.T) {
	t.Run("restores the wakeup guard from the checkpoint so a redundant "+
		"wakeup request after restore schedules no duplicate", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "ck.tar.gz")
		const wakeTime = timing.VTimeInPicoSec(1000)

		// Checkpoint with one wakeup pending in the engine queue.
		srcSim, srcC := buildWakeSim()
		defer func() {
			srcSim.Terminate()
			os.Remove("akita_sim_" + srcSim.ID() + ".sqlite3")
		}()
		srcC.WakeAt(wakeTime)
		require.NoError(t, srcSim.SaveCheckpoint(path))

		// Restore into a fresh sim, whose wakeup guard is restored directly from
		// the checkpoint.
		dstSim, dstC := buildWakeSim()
		defer func() {
			dstSim.Terminate()
			os.Remove("akita_sim_" + dstSim.ID() + ".sqlite3")
		}()
		require.NoError(t, dstSim.LoadCheckpoint(path))

		// A redundant request for a wakeup at the already-pending time. With the
		// guard restored, it is recognized as redundant and queues no duplicate.
		dstC.WakeAt(wakeTime)

		engine := dstSim.Engine().(*timing.SerialEngine)
		require.NoError(t, engine.Run())

		// Exactly one wakeup fired. Without the restored guard the redundant
		// request would have queued a duplicate (Wakeups == 2).
		require.Equal(t, 1, dstC.State.Wakeups)
	})
}

func (m *resumeMWs) Initialize(c *resumeComp) {
	*m = resumeMWs{Worker: &resumeWorkerMW{comp: c}}
}

func (m *tickCountMWs) Initialize(c *tickCountComp) {
	*m = tickCountMWs{Counter: &tickCountMW{comp: c}}
}

func (m *wakeMWs) Initialize(c *wakeComp) {
	*m = wakeMWs{Counter: &wakeMW{comp: c}}
}

func requirePanicContains(t *testing.T, substring string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		require.NotNil(t, recovered)
		require.Contains(t, fmt.Sprint(recovered), substring)
	}()
	fn()
}
