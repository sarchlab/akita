package modeling_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type boolMiddleware struct{}

func (boolMiddleware) Handle(timing.Event) bool { return false }

type voidMiddleware struct{}

func (voidMiddleware) Handle(timing.Event) {}

type boolPipeline struct{ Work boolMiddleware }
type voidPipeline struct{ Work voidMiddleware }
type middlewareSpec struct{ Freq timing.Freq }

func TestMiddlewareContractMatchesModel(t *testing.T) {
	for _, model := range []string{"event", "ticking", "wakeup"} {
		t.Run(model, func(t *testing.T) {
			s := sim.MakeBuilder().Build()
			switch model {
			case "event":
				modeling.MakeBuilder[*event.Component[
					modeling.None, modeling.None, modeling.None, modeling.None, boolPipeline]](modeling.None{}).
					WithSimulation(s).Build("C")
			case "ticking":
				modeling.MakeBuilder[*ticking.Component[
					middlewareSpec, modeling.None, modeling.None, modeling.None, voidPipeline]](middlewareSpec{Freq: timing.GHz}).
					WithSimulation(s).Build("C")
			case "wakeup":
				modeling.MakeBuilder[*wakeup.Component[
					modeling.None, modeling.None, modeling.None, modeling.None, voidPipeline]](modeling.None{}).
					WithSimulation(s).Build("C")
			}
			field := "voidPipeline"
			if model == "event" {
				field = "boolPipeline"
			}
			require.PanicsWithValue(t, "modeling: Middlewares field modeling_test."+field+
				".Work must be exported and implement "+model+".Middleware",
				func() { _ = s.Initialize() })
			require.NoError(t, s.Terminate())
		})
	}
}
