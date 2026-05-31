package simulation

import (
	"io"
	"os"

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

		entities := simulation.Entities()
		names := make([]string, 0, len(entities))
		for _, e := range entities {
			names = append(names, e.Name())
		}

		Expect(names).To(ConsistOf(
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
})
