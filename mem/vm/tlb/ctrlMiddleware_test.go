package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/simulation/timing"
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
			WithPorts(defaultPorts("TLB")).
			Build("TLB")

		plugNoopConn(comp)

		ctrlMW = comp.Middlewares.Ctrl
	})

	It("should do nothing if there is no req in ctrlPort", func() {
		madeProgress := ctrlMW.Handle(modelingtest.TickEvent(comp))

		Expect(madeProgress).To(BeFalse())
	})
})
