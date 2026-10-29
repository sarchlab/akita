package accessworkload_test

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/internal/accessworkload"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func hardware() *sim.Simulation {
	s := sim.MakeBuilder().Build()
	storage := mem.MakeStorageBuilder().WithSimulation(s).WithCapacity(16384).Build("Storage")
	memory := modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).WithSimulation(s).
		WithResources(idealmemcontroller.Resources{Storage: storage}).Build("Memory")
	spec := memaccessagent.DefaultSpec()
	spec.LowModule = "Memory.Top"
	spec.MaxAddress = 32 // Explicit workload must override this legacy range in State, not Spec.
	agent := modeling.MakeBuilder[*memaccessagent.Comp](spec).WithSimulation(s).Build("MemAccessAgent")
	buffer := twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
	agent.BindPort("Mem", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(buffer).Build())
	memory.BindPort("Top", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(buffer).Build())
	memory.BindPort("Control", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(buffer).Build())
	connection := direct.NewConnection("Wire", s, timing.GHz)
	connection.BindPort(agent.Ports.Mem)
	connection.BindPort(memory.Ports.Top)
	return s
}

func TestWorkloadStateAndCheckpoint(t *testing.T) {
	s := hardware()
	defer s.Terminate()
	work := accessworkload.DefaultSpec()
	work.ReadCount, work.WriteCount = 30, 50
	work.AddressRange, work.AddressOffset, work.Seed = 4096, 4096, 42
	w := accessworkload.New(work)
	require.Panics(t, func() { w.Execute(s) })
	require.NoError(t, s.Initialize())
	agent := s.Lookup("MemAccessAgent").(*memaccessagent.Comp)
	oldSpec := agent.Spec
	require.Equal(t, 1000, agent.State.ReadLeft)
	w.Execute(s)
	require.Equal(t, oldSpec, agent.Spec)
	require.EqualValues(t, 4096, agent.State.MaxAddress)
	require.EqualValues(t, 4096, agent.State.AddressOffset)
	require.Equal(t, 30, agent.State.ReadLeft)
	require.Equal(t, 50, agent.State.WriteLeft)
	require.Panics(t, func() { w.Execute(s) })
	require.NoError(t, s.Engine().(*timing.SerialEngine).RunUntil(10000))
	require.NotEmpty(t, agent.State.PendingWriteReq)
	path := filepath.Join(t.TempDir(), "workload.checkpoint")
	require.NoError(t, s.SaveCheckpoint(path))
	require.NoError(t, s.Run())
	require.Zero(t, agent.State.ReadLeft)
	require.Zero(t, agent.State.WriteLeft)
	for address := range agent.State.KnownMemValue {
		require.GreaterOrEqual(t, address, uint64(4096))
		require.Less(t, address, uint64(8192))
	}
	restored := hardware()
	defer restored.Terminate()
	require.NoError(t, restored.Initialize())
	require.NoError(t, restored.LoadCheckpoint(path))
	require.NoError(t, restored.Run())
	require.Equal(t, agent.State, restored.Lookup("MemAccessAgent").(*memaccessagent.Comp).State)
	require.Equal(t, s.Engine().CurrentTime(), restored.Engine().CurrentTime())
	require.Equal(t, s.Lookup("Storage").(*mem.Storage).Read(0, 16384),
		restored.Lookup("Storage").(*mem.Storage).Read(0, 16384))
	t.Logf("workload: 30 reads/50 writes in [4096,8192), restored state/data/time match at %d ps",
		s.Engine().CurrentTime())
}

func TestWorkloadNameAndTypeContract(t *testing.T) {
	s := hardware()
	defer s.Terminate()
	require.NoError(t, s.Initialize())
	for _, name := range []string{"Missing", "Storage"} {
		spec := accessworkload.DefaultSpec()
		spec.AgentName = name
		require.Panics(t, func() { accessworkload.New(spec).Execute(s) })
	}
}
