package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("TLB CtrlMiddleware", func() {

	var (
		engine timing.Engine
		comp   *Comp
		ctrlMW *ctrlMiddleware
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		reg := modeling.NewStandaloneRegistrar(engine)
		comp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(DefaultSpec()).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: "RemotePort",
				},
			}).
			Build("TLB")

		assignDefaultPorts(reg, comp)
		plugNoopConn(comp)

		ctrlMW = comp.Middlewares()[0].(*ctrlMiddleware)
	})

	It("should do nothing if there is no req in ctrlPort", func() {
		madeProgress := ctrlMW.Tick()

		Expect(madeProgress).To(BeFalse())
	})
})
