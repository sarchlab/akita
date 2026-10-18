package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestTLBCtrlMiddleware(t *testing.T) {
	var (
		engine timing.Engine
		sim    timing.Simulation
		comp   *Comp
		ctrlMW *ctrlMiddleware
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		routingSpec1 := DefaultSpec()

		routingSpec1.Bottom.Destinations = []messaging.RemotePort{"RemotePort"}
		comp = modeling.MakeBuilder[*Comp](routingSpec1).
			WithSimulation(sim).
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
	}

	t.Run("should do nothing if there is no req in ctrlPort", func(t *testing.T) {
		setup1(t)

		madeProgress := ctrlMW.Handle(modelingtest.TickEvent(comp))

		require.False(t, madeProgress)
	})
}
