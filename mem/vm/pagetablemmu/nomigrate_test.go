package pagetablemmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	at "github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	pf "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/stretchr/testify/require"
)

func (r *rig) translateNoMigrate(va, device uint64, noMigrate bool) uint64 {
	return r.send(r.request, r.mmu.Ports.Top.AsRemote(),
		at.Req{PID: 7, VAddr: va, DeviceID: device, NoMigrate: noMigrate}, 0)
}

func TestNoMigrateUsesRemoteMappingWithoutConsulting(t *testing.T) {
	spec := baseSpec()
	spec.ConsultRemote = true
	r := newRig(t, spec, nil, false)
	r.root(r.mmu)
	r.install(r.mmu, 0x7000, 0x90000, 1)

	id := r.translateNoMigrate(0x7123, 2, true)
	r.run()

	msg := r.recv(r.request)
	require.Equal(t, id, msg.RspTo)
	rsp := msg.Payload.(at.Rsp)
	require.False(t, rsp.Fault)
	require.Equal(t, uint64(0x90000), rsp.Page.PAddr)
	require.Equal(t, uint64(1), rsp.Page.DeviceID)
	require.Zero(t, r.policy.IncomingBuf().Size())
}

// TestNoMigrateTravelsThroughFaultIn checks that a local MMU forwards
// NoMigrate to the authoritative MMU, which takes a remote mapping for it
// and forwards NoMigrate to the fault handler when the page is unmapped.
func TestNoMigrateTravelsThroughFaultIn(t *testing.T) {
	ioSpec := baseSpec()
	ioSpec.PoolBase = 0x38000
	ioSpec.ConsultRemote = true
	r := newRig(t, baseSpec(), &ioSpec, false)
	r.root(r.mmu)
	r.root(r.io)
	r.install(r.io, 0x7000, 0x90000, 1)

	id := r.translateNoMigrate(0x7123, 2, true)
	r.run()
	msg := r.recv(r.request)
	require.Equal(t, id, msg.RspTo)
	require.Equal(t, uint64(0x90000), msg.Payload.(at.Rsp).Page.PAddr)
	require.Zero(t, r.policy.IncomingBuf().Size())

	id = r.translateNoMigrate(0xb000, 2, true)
	r.run()
	f := r.recv(r.policy)
	req := f.Payload.(pf.Req)
	require.True(t, req.NoMigrate)
	require.False(t, req.HasMapping)
	require.Equal(t, uint64(2), req.DeviceID)
	r.send(r.policy, f.Src, pf.Rsp{Result: pf.Blocked}, f.ID)
	r.run()
	msg = r.recv(r.request)
	require.Equal(t, id, msg.RspTo)
	require.True(t, msg.Payload.(at.Rsp).Fault)
}

// TestNoMigrateDecisionsStaySeparate checks that NoMigrate and normal
// requests of one device that share a walk get separate decisions.
func TestNoMigrateDecisionsStaySeparate(t *testing.T) {
	r := newRig(t, baseSpec(), nil, false)
	r.root(r.mmu)

	normal1 := r.translateNoMigrate(0xb000, 2, false)
	noMigrate := r.translateNoMigrate(0xb040, 2, true)
	normal2 := r.translateNoMigrate(0xb080, 2, false)
	r.run()

	decisions := map[bool]uint64{}
	for range 2 {
		f := r.recv(r.policy)
		req := f.Payload.(pf.Req)
		require.Equal(t, uint64(2), req.DeviceID)
		require.False(t, req.HasMapping)
		decisions[req.NoMigrate] = f.ID
	}
	require.Len(t, decisions, 2)
	require.Zero(t, r.policy.IncomingBuf().Size())

	// Blocking the NoMigrate request leaves the normal requests waiting.
	r.send(r.policy, r.mmu.Ports.PageFault.AsRemote(), pf.Rsp{Result: pf.Blocked}, decisions[true])
	r.run()
	msg := r.recv(r.request)
	require.Equal(t, noMigrate, msg.RspTo)
	require.True(t, msg.Payload.(at.Rsp).Fault)
	require.Zero(t, r.request.IncomingBuf().Size())
	require.Zero(t, r.policy.IncomingBuf().Size())

	mapping := vm.Page{PID: 7, VAddr: 0xb000, PAddr: 0x81000,
		PageSize: r.mmu.Spec.pageSize(), DeviceID: 2, Valid: true}
	r.send(r.policy, r.mmu.Ports.PageFault.AsRemote(),
		pf.Rsp{Result: pf.Resolved, Mapping: mapping}, decisions[false])
	r.run()
	for _, id := range []uint64{normal1, normal2} {
		msg = r.recv(r.request)
		require.Equal(t, id, msg.RspTo)
		require.Equal(t, at.Rsp{Page: mapping}, msg.Payload)
	}
	require.Equal(t, free, r.mmu.State.Walks[0].Stage)
}
