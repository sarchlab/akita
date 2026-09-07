package tracing

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/stretchr/testify/require"
)

type isolatedDomain struct {
	sim timing.Simulation
	hooking.HookableBase
}

func (*isolatedDomain) Name() string                       { return "same-component-name" }
func (*isolatedDomain) CurrentTime() timing.VTimeInPicoSec { return 0 }

func TestTaskRegistriesIsolateSimulationNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lookup func(messaging.Msg, NamedHookable) uint64
		forget func(uint64, NamedHookable)
	}{
		{"receiver", lookupOrCreateReceiverTaskID, forgetReceiverTaskIDByMsgID},
		{"incoming", lookupOrCreateIncomingBufferTaskID, forgetIncomingBufferTaskIDByMsgID},
		{"outgoing", lookupOrCreateOutgoingBufferTaskID, forgetOutgoingBufferTaskIDByMsgID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &isolatedDomain{sim: modeling.NewStandaloneSimulation(timing.NewSerialEngine())}
			b := &isolatedDomain{sim: modeling.NewStandaloneSimulation(timing.NewSerialEngine())}
			msg := &messaging.MsgMeta{ID: a.Simulation().NewID()}
			require.Equal(t, msg.ID, b.Simulation().NewID())
			aTask := tc.lookup(msg, a)
			require.Equal(t, uint64(2), aTask)
			b.Simulation().NewID()
			bTask := tc.lookup(msg, b)
			require.Equal(t, uint64(3), bTask)
			require.Equal(t, aTask, tc.lookup(msg, a))
			tc.forget(msg.ID, a)
			require.Equal(t, bTask, tc.lookup(msg, b))
			require.Equal(t, uint64(3), tc.lookup(msg, a))
			tc.forget(msg.ID, a)
			tc.forget(msg.ID, b)
		})
	}
}

func (c *isolatedDomain) Simulation() timing.Simulation { return c.sim }
