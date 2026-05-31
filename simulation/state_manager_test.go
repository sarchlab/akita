package simulation

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type componentRuntimeState struct {
	Counter int
}

// statefulComponent keeps its runtime state in a distinct sub-object and
// exposes it via StateRef, like modeling.Component does.
type statefulComponent struct {
	name  string
	state *componentRuntimeState
}

func (c *statefulComponent) Name() string {
	return c.name
}

func (c *statefulComponent) StateRef() any {
	return c.state
}

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
			comp := &testComponent{name: "comp"}
			sim.RegisterComponent(comp)

			obj, found := sim.GetStateByName("comp")
			Expect(found).To(BeTrue())
			Expect(obj).To(BeIdenticalTo(comp))
		})
	})

	Describe("StateHolder entities", func() {
		It("should return the StateRef for entities holding a distinct state", func() {
			comp := &statefulComponent{
				name:  "sc",
				state: &componentRuntimeState{Counter: 7},
			}
			sim.RegisterComponent(comp)

			obj, found := sim.GetStateByName("sc")
			Expect(found).To(BeTrue())
			Expect(obj).To(BeIdenticalTo(comp.state))
		})

		It("should return a live reference, so backdoor mutations are shared", func() {
			comp := &statefulComponent{
				name:  "sc",
				state: &componentRuntimeState{Counter: 7},
			}
			sim.RegisterComponent(comp)

			obj, ok := sim.GetStateByName("sc")
			Expect(ok).To(BeTrue())
			obj.(*componentRuntimeState).Counter = 99

			Expect(comp.state.Counter).To(Equal(99))
		})

		It("should fall back to the entity itself when it holds no distinct state", func() {
			comp := testComponent{name: "plain"}
			sim.RegisterComponent(comp)

			obj, found := sim.GetStateByName("plain")
			Expect(found).To(BeTrue())
			Expect(obj).To(Equal(comp))
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
