package simulation

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Global state manager", func() {
	var sim *Simulation

	BeforeEach(func() {
		sim = MakeBuilder().WithoutMonitoring().Build()
	})

	AfterEach(func() {
		sim.Terminate()
		os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
	})

	Describe("GetStateByName", func() {
		It("should resolve a registered component to its live object", func() {
			comp := testComponent{name: "comp", ports: []Port{testPort{name: "p"}}}
			sim.RegisterComponent(comp)

			obj, found := sim.GetStateByName("comp")
			Expect(found).To(BeTrue())
			Expect(obj).To(Equal(comp))
		})

		It("should resolve a registered port to its live object", func() {
			port := testPort{name: "p"}
			sim.RegisterComponent(testComponent{name: "comp", ports: []Port{port}})

			obj, found := sim.GetStateByName("p")
			Expect(found).To(BeTrue())
			Expect(obj).To(Equal(port))
		})

		It("should resolve a registered resource to its live object", func() {
			resource := testResource{
				name:     "Program.Memory",
				kind:     "test.Resource",
				format:   "json",
				ext:      ".json",
				identity: "resource-1",
			}
			sim.RegisterResource(resource)

			obj, found := sim.GetStateByName("Program.Memory")
			Expect(found).To(BeTrue())
			Expect(obj).To(Equal(resource))
		})

		It("should report not found for unknown names", func() {
			obj, found := sim.GetStateByName("missing")
			Expect(found).To(BeFalse())
			Expect(obj).To(BeNil())
		})

		It("should keep handles valid across the same object pointer", func() {
			comp := &resourceComponent{
				testComponent: testComponent{name: "comp"},
			}
			sim.RegisterComponent(comp)

			obj, found := sim.GetStateByName("comp")
			Expect(found).To(BeTrue())
			Expect(obj).To(BeIdenticalTo(comp))
		})
	})

	Describe("GetState typed helper", func() {
		It("should return the typed object when the type matches", func() {
			conn := newTestConnection("conn")
			sim.RegisterConnection(conn)

			got, ok := GetState[*testConnection](sim, "conn")
			Expect(ok).To(BeTrue())
			Expect(got).To(BeIdenticalTo(conn))
		})

		It("should return false when the type does not match", func() {
			sim.RegisterConnection(newTestConnection("conn"))

			_, ok := GetState[*Simulation](sim, "conn")
			Expect(ok).To(BeFalse())
		})

		It("should return false for unknown names", func() {
			_, ok := GetState[*testConnection](sim, "missing")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("Singleton runtime entities", func() {
		It("should register the engine as an entity resolving to the engine", func() {
			obj, found := sim.GetStateByName("engine")
			Expect(found).To(BeTrue())
			Expect(obj).To(BeIdenticalTo(sim.GetEngine()))

			entity, ok := sim.GetEntityByName("engine")
			Expect(ok).To(BeTrue())
			Expect(entity.Kind).To(Equal(EntityKindEngine))
		})

		It("should register the ID generator as an entity", func() {
			obj, found := sim.GetStateByName("id-generator")
			Expect(found).To(BeTrue())
			Expect(obj).To(BeAssignableToTypeOf(IDGeneratorHandle{}))

			entity, ok := sim.GetEntityByName("id-generator")
			Expect(ok).To(BeTrue())
			Expect(entity.Kind).To(Equal(EntityKindIDGenerator))
		})

		It("should round-trip the ID generator next-ID through the handle", func() {
			handle, ok := GetState[IDGeneratorHandle](sim, "id-generator")
			Expect(ok).To(BeTrue())

			original := handle.NextID()
			defer handle.SetNextID(original)

			handle.SetNextID(42)
			Expect(handle.NextID()).To(Equal(uint64(42)))
		})
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

		It("should reject a user entity that reuses a reserved singleton name", func() {
			Expect(func() {
				sim.RegisterComponent(testComponent{name: "engine"})
			}).To(PanicWith(ContainSubstring("already registered")))
		})
	})

	Describe("Deterministic entity inventory", func() {
		It("should list entities in stable registration order across rebuilds", func() {
			build := func() []Entity {
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

				return s.Entities()
			}

			Expect(build()).To(Equal(build()))
		})
	})
})
