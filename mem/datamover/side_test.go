package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestInvalidSidesRejectedBeforeAdmission(t *testing.T) {
	for _, sides := range [][2]datamoverprotocol.Side{
		{"unknown", datamoverprotocol.SideInside},
		{datamoverprotocol.SideInside, "unknown"},
		{"", datamoverprotocol.SideOutside},
	} {
		c := buildWithMappers("Inside.Top", "Outside.Top")
		req := messaging.Msg{ID: c.NewID(), Payload: datamoverprotocol.Req{SrcSide: sides[0], DstSide: sides[1]}}
		c.Ports.Top.Deliver(req)
		require.Panics(t, func() { c.Middlewares.CtrlParse.parseFromCP() })
		require.False(t, c.State.CurrentTransaction.Active)
		_, present := c.Ports.Top.PeekIncoming()
		require.True(t, present)
	}
}
