package simulation

import (
	"io"
	"os"
	"path/filepath"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

type testPort struct {
	name     string
	incoming int
	outgoing int
}

func (p testPort) Name() string {
	return p.name
}

func (p testPort) NumIncoming() int {
	return p.incoming
}

func (p testPort) NumOutgoing() int {
	return p.outgoing
}

type testComponent struct {
	name  string
	ports []Port
}

func (c testComponent) Name() string {
	return c.name
}

func (c testComponent) Ports() []Port {
	return c.ports
}

type compatiblePort interface {
	Port
	compatiblePort()
}

type compatibleTestPort struct {
	testPort
}

func (p compatibleTestPort) compatiblePort() {
}

type compatiblePortComponent struct {
	name  string
	ports []compatiblePort
}

func (c compatiblePortComponent) Name() string {
	return c.name
}

func (c compatiblePortComponent) Ports() []compatiblePort {
	return c.ports
}

var _ = Describe("Simulation", func() {
	var (
		simulation *Simulation
		comp       testComponent
		port       testPort
	)

	BeforeEach(func() {
		simulation = MakeBuilder().WithoutMonitoring().Build()
		port = testPort{name: "port"}
		comp = testComponent{name: "comp", ports: []Port{port}}
	})

	AfterEach(func() {
		simulation.Terminate()

		os.Remove("akita_sim_" + simulation.ID() + ".sqlite3")
	})

	It("should register a component", func() {
		simulation.RegisterComponent(comp)

		Expect(simulation.GetComponentByName("comp")).To(Equal(comp))
		Expect(simulation.GetPortByName("port")).To(Equal(port))
	})

	It("should reject duplicate component names", func() {
		comp.ports = nil
		simulation.RegisterComponent(comp)

		dup := testComponent{name: "comp"}

		Expect(func() {
			simulation.RegisterComponent(dup)
		}).To(PanicWith(ContainSubstring("already registered")))
	})

	It("should reject duplicate port names", func() {
		simulation.RegisterComponent(comp)

		dupPort := testPort{name: "port"}
		dupComp := testComponent{name: "other", ports: []Port{dupPort}}

		Expect(func() {
			simulation.RegisterComponent(dupComp)
		}).To(PanicWith(ContainSubstring("already registered")))
	})

	It("should return all registered components", func() {
		simulation.RegisterComponent(comp)

		comps := simulation.Components()
		Expect(comps).To(HaveLen(1))
		Expect(comps[0]).To(Equal(comp))
	})

	It("should register ports from compatible port slices", func() {
		port := compatibleTestPort{testPort: testPort{name: "port"}}
		comp := compatiblePortComponent{
			name:  "comp",
			ports: []compatiblePort{port},
		}

		simulation.RegisterComponent(comp)

		Expect(simulation.GetPortByName("port")).To(Equal(port))
	})

	It("should register shared state resources directly", func() {
		resource := testResource{
			name:     "Program.Memory",
			kind:     "test.Resource",
			format:   "json",
			ext:      ".json",
			identity: "resource-1",
		}

		simulation.RegisterResource(resource)

		resources := simulation.Resources()
		Expect(resources).To(HaveLen(1))
		Expect(resources[0].Name()).To(Equal("Program.Memory"))
	})

	It("should reject duplicate shared state names with different identities", func() {
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

		Expect(func() {
			simulation.RegisterResource(duplicate)
		}).To(PanicWith(ContainSubstring("already registered")))
	})

	It("should return all registered entities", func() {
		resource := testResource{
			name:     "Program.Memory",
			kind:     "test.Resource",
			format:   "json",
			ext:      ".json",
			identity: "resource-1",
		}
		connection := newTestConnection("conn")

		simulation.RegisterComponent(comp)
		simulation.RegisterConnection(connection)
		simulation.RegisterResource(resource)

		entities := simulation.entities
		names := make([]string, 0, len(entities))
		for _, e := range entities {
			names = append(names, e.Name())
		}

		Expect(names).To(ConsistOf(
			"Engine", "IDGenerator",
			"comp", "port", "conn", "Program.Memory",
		))
	})

	Context("Builder with custom output file", func() {
		var customSim *Simulation

		AfterEach(func() {
			if customSim != nil {
				customSim.Terminate()
				os.Remove("test_custom_output.sqlite3")
				customSim = nil
			}
		})

		It("should allow custom output file to be set", func() {
			builder := MakeBuilder().
				WithoutMonitoring().
				WithOutputFileName("test_custom_output")
			customSim = builder.Build()

			Expect(customSim).ToNot(BeNil())
			Expect(customSim.GetDataRecorder()).ToNot(BeNil())
		})
	})

	Context("checkpoint foundation", func() {
		It("should reject an entity that has no serializer", func() {
			// A bare component/port has no checkpoint serializer yet, so save
			// must fail loudly and not leave an archive behind.
			noSerializerSim := MakeBuilder().WithoutMonitoring().Build()
			defer func() {
				noSerializerSim.Terminate()
				os.Remove("akita_sim_" + noSerializerSim.ID() + ".sqlite3")
			}()
			noSerializerSim.RegisterComponent(testComponent{
				name:  "comp",
				ports: []Port{testPort{name: "comp.Port"}},
			})

			path := filepath.Join(GinkgoT().TempDir(), "checkpoint.tar.gz")

			err := noSerializerSim.SaveCheckpoint(path, "test-build")

			Expect(err).To(MatchError(ContainSubstring(
				"has no checkpoint serializer")))
			_, statErr := os.Stat(path)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("should reject checkpoints for parallel engines", func() {
			parallelSim := MakeBuilder().
				WithoutMonitoring().
				WithParallelEngine().
				Build()
			defer func() {
				parallelSim.Terminate()
				os.Remove("akita_sim_" + parallelSim.ID() + ".sqlite3")
			}()

			path := filepath.Join(GinkgoT().TempDir(), "checkpoint.tar.gz")

			err := parallelSim.SaveCheckpoint(path, "test-build")

			Expect(err).To(MatchError(ContainSubstring(
				"only timing.SerialEngine is supported")))
		})

		It("should reject a checkpoint from a different build", func() {
			path := filepath.Join(GinkgoT().TempDir(), "checkpoint.tar.gz")

			err := writeArchive(path, "other-build", dummyPayloads(simulation))
			Expect(err).ToNot(HaveOccurred())

			err = simulation.LoadCheckpoint(path, "test-build")

			Expect(err).To(MatchError(ContainSubstring("build ID mismatch")))
		})

		It("should reject when a rebuilt entity is missing from the checkpoint", func() {
			path := filepath.Join(GinkgoT().TempDir(), "checkpoint.tar.gz")

			entries := []archiveEntry{}
			for _, entity := range simulation.entities {
				if entity.Name() == "IDGenerator" {
					continue
				}
				entries = append(entries,
					archiveEntry{name: entity.Name(), data: []byte("{}")})
			}

			err := writeArchive(path, "test-build", entries)
			Expect(err).ToNot(HaveOccurred())

			err = simulation.LoadCheckpoint(path, "test-build")

			Expect(err).To(MatchError(ContainSubstring(
				"rebuilt entity \"IDGenerator\" is missing from checkpoint")))
		})

	})
})

var _ = Describe("Global state manager", func() {
	var sim *Simulation

	BeforeEach(func() {
		sim = MakeBuilder().WithoutMonitoring().Build()
	})

	AfterEach(func() {
		sim.Terminate()
		os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
	})

	Describe("Global name uniqueness", func() {
		It("should reject a connection whose name collides with a component", func() {
			sim.RegisterComponent(testComponent{name: "shared"})

			Expect(func() {
				sim.RegisterConnection(newTestConnection("shared"))
			}).To(PanicWith(ContainSubstring("already registered")))
		})

		It("should reject a resource whose name collides with a component", func() {
			sim.RegisterComponent(testComponent{name: "Program.Memory"})

			Expect(func() {
				sim.RegisterResource(testResource{
					name:     "Program.Memory",
					kind:     "test.Resource",
					format:   "json",
					ext:      ".json",
					identity: "resource-1",
				})
			}).To(PanicWith(ContainSubstring("already registered")))
		})
	})

	Describe("Deterministic entity inventory", func() {
		It("should list entities in stable registration order across rebuilds", func() {
			build := func() []string {
				s := MakeBuilder().WithoutMonitoring().Build()
				defer func() {
					s.Terminate()
					os.Remove("akita_sim_" + s.ID() + ".sqlite3")
				}()

				s.RegisterComponent(testComponent{
					name:  "GPU[1]",
					ports: []Port{testPort{name: "GPU[1].Port"}},
				})
				s.RegisterComponent(testComponent{
					name:  "GPU[2]",
					ports: []Port{testPort{name: "GPU[2].Port"}},
				})
				s.RegisterConnection(newTestConnection("GPU[1].GPU[2].Conn"))

				entities := s.entities
				names := make([]string, 0, len(entities))
				for _, e := range entities {
					names = append(names, e.Name())
				}

				return names
			}

			Expect(build()).To(Equal(build()))
		})
	})

	Describe("Complete state inventory", func() {
		It("registers the engine and ID generator as entities", func() {
			entities := sim.entities
			names := make([]string, 0, len(entities))
			for _, e := range entities {
				names = append(names, e.Name())
			}

			Expect(names).To(ContainElements("Engine", "IDGenerator"))
		})
	})
})

type roundTripSpec struct {
	Latency int `json:"latency"`
}

type roundTripState struct {
	Count int `json:"count"`
}

var _ = Describe("Checkpoint round trip", func() {
	It("restores component state, storage, ID counter, and engine time", func() {
		sim := MakeBuilder().WithoutMonitoring().Build()
		defer func() {
			sim.Terminate()
			os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
		}()

		// A port-less component plus a storage resource: every entity
		// (Engine, IDGenerator, Comp, Mem) is checkpointable, so no port or
		// connection serializers are needed yet.
		engine := sim.GetEngine().(*timing.SerialEngine)
		comp := modeling.NewBuilder[roundTripSpec, roundTripState, modeling.None]().
			WithEngine(engine).
			WithFreq(1 * timing.GHz).
			WithSpec(roundTripSpec{Latency: 5}).
			Build("Comp")
		sim.RegisterComponent(comp)
		storage := mem.MakeStorageBuilder().
			WithCapacity(4 * mem.KB).
			WithSimulation(sim).
			Build("Mem")

		// Establish runtime state across all four entity kinds.
		comp.State = roundTripState{Count: 7}
		Expect(storage.Write(0, []byte{1, 2, 3, 4})).To(Succeed())
		for i := 0; i < 5; i++ {
			timing.GetIDGenerator().Generate()
		}
		savedCounter := timing.GetIDGeneratorNextID()
		engine.SetCurrentTime(100)

		path := filepath.Join(GinkgoT().TempDir(), "checkpoint.tar.gz")
		Expect(sim.SaveCheckpoint(path, "test-build")).To(Succeed())

		// Mutate every piece of runtime state away from the checkpoint.
		comp.State = roundTripState{Count: 999}
		Expect(storage.Write(0, []byte{0, 0, 0, 0})).To(Succeed())
		timing.GetIDGenerator().Generate()
		timing.GetIDGenerator().Generate()
		engine.SetCurrentTime(500)

		// Restore and confirm every piece came back.
		Expect(sim.LoadCheckpoint(path, "test-build")).To(Succeed())

		Expect(comp.State.Count).To(Equal(7))
		data, err := storage.Read(0, 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
		Expect(timing.GetIDGeneratorNextID()).To(Equal(savedCounter))
		Expect(engine.CurrentTime()).To(Equal(timing.VTimeInSec(100)))
	})
})

type resumeSpec struct {
	N int `json:"n"`
}

type resumeState struct {
	Pending  int    `json:"pending"`
	Done     int    `json:"done"`
	Checksum uint64 `json:"checksum"`
}

type resumeWorkerMW struct {
	comp *modeling.Component[resumeSpec, resumeState, modeling.None]
}

func (m *resumeWorkerMW) Tick() bool {
	if m.comp.State.Pending <= 0 {
		return false
	}
	m.comp.State.Done++
	m.comp.State.Checksum = m.comp.State.Checksum*1000003 + uint64(m.comp.State.Done)
	m.comp.State.Pending--
	return true
}

func buildResumeSim() (*Simulation, *modeling.Component[resumeSpec, resumeState, modeling.None]) {
	sim := MakeBuilder().WithoutMonitoring().Build()
	engine := sim.GetEngine().(*timing.SerialEngine)
	w := modeling.NewBuilder[resumeSpec, resumeState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(resumeSpec{N: 1}).
		Build("Worker")
	w.AddMiddleware(&resumeWorkerMW{comp: w})
	sim.RegisterComponent(w)
	return sim, w
}

var _ = Describe("Mid-transaction resume", func() {
	It("resumes from a checkpoint with a pending tick identically to running uninterrupted", func() {
		path := filepath.Join(GinkgoT().TempDir(), "ck.tar.gz")
		const buildID = "test-build"

		// Reference run: set up mid-transaction state (work pending, one tick
		// already scheduled), checkpoint there, then continue to completion.
		refSim, refW := buildResumeSim()
		defer func() {
			refSim.Terminate()
			os.Remove("akita_sim_" + refSim.ID() + ".sqlite3")
		}()
		refW.State = resumeState{Pending: 5}
		refW.TickLater() // schedule the first tick -> non-empty engine queue

		Expect(refSim.SaveCheckpoint(path, buildID)).To(Succeed())

		refEngine := refSim.GetEngine().(*timing.SerialEngine)
		Expect(refEngine.Run()).To(Succeed())
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
		Expect(resSim.LoadCheckpoint(path, buildID)).To(Succeed())

		resEngine := resSim.GetEngine().(*timing.SerialEngine)
		Expect(resEngine.Run()).To(Succeed())

		Expect(resW.State.Done).To(Equal(wantDone))
		Expect(resW.State.Checksum).To(Equal(wantChecksum))
		Expect(resEngine.CurrentTime()).To(Equal(wantTime))
		Expect(wantDone).To(Equal(5)) // sanity: work actually happened
	})
})
