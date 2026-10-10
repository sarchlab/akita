package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var _ = Describe("TLB CtrlMiddleware", func() {

	var (
		engine timing.Engine
		sim    timing.Simulation
		comp   *Comp
		ctrlMW *ctrlMiddleware
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		comp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(Definition.DefaultSpec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: "RemotePort",
				},
			}).
			Build("TLB")
		compPorts := defaultPorts("TLB")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)
		plugNoopConn(comp)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		ctrlMW = comp.Middlewares.Ctrl
	})

	It("should do nothing if there is no req in ctrlPort", func() {
		madeProgress := ctrlMW.Handle(modelingtest.TickEvent(comp))

		Expect(madeProgress).To(BeFalse())
	})
})
