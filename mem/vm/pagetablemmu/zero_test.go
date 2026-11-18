package pagetablemmu

import (
	"encoding/binary"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestNodePublicationWaitsForEveryZeroACK(t *testing.T) {
	spec := baseSpec()
	spec.Banks, spec.VABits = spec.Banks[:1], 30
	spec.MemoryWidth = 16
	// The two test ports stand in for 32-byte interleaved memory banks.
	spec.Memory.Destinations = []messaging.RemotePort{"Client.Policy", "Client.Requests"}
	spec.Memory.InterleavingSize = 32
	r := newRig(t, spec, nil, false)
	root := spec.PoolBase
	rootID := r.send(r.ctrl, "Local.Control", pt.Req{Command: pt.RegisterRoot, PID: 7, Root: root}, 0)
	lastACK := r.holdFirstZeroWrite(root)
	require.NotContains(t, r.mmu.State.Roots, vm.PID(7))
	r.send(r.policy, lastACK.Src, memprotocol.WriteDoneRsp{}, lastACK.ID)
	r.run()
	require.Equal(t, rootID, r.recv(r.ctrl).RspTo)
	require.Equal(t, root, r.mmu.State.Roots[7])

	page := vm.Page{PAddr: 0x80000}
	mapID := r.send(r.ctrl, "Local.Control", pt.Req{Command: pt.Map, PID: 7, VAddr: 0, Mapping: page}, 0)
	r.run()
	probe := r.recv(r.policy)
	require.Equal(t, memprotocol.ReadReq{PID: 7, Address: root, AccessByteSize: pteBytes}, probe.Payload)
	r.send(r.policy, probe.Src, memprotocol.DataReadyRsp{Data: make([]byte, pteBytes)}, probe.ID)
	child := root + nodeBytes
	lastACK = r.holdFirstZeroWrite(child)
	r.send(r.policy, lastACK.Src, memprotocol.WriteDoneRsp{}, lastACK.ID)
	// Only the parent pointer may issue next. The leaf must wait for its ACK.
	for _, want := range []struct{ addr, word uint64 }{{root, child | validBit}, {child, spec.encodeLeaf(page)}} {
		r.run()
		require.Equal(t, 1, r.policy.IncomingBuf().Size())
		require.Zero(t, r.request.IncomingBuf().Size())
		require.Zero(t, r.ctrl.IncomingBuf().Size())
		write := r.recv(r.policy)
		require.Equal(t, memprotocol.WriteReq{PID: 7, Address: want.addr,
			Data: binary.LittleEndian.AppendUint64(nil, want.word)}, write.Payload)
		r.send(r.policy, write.Src, memprotocol.WriteDoneRsp{}, write.ID)
	}
	r.run()
	msg := r.recv(r.ctrl)
	require.Equal(t, mapID, msg.RspTo)
	require.Empty(t, msg.Payload.(pt.Rsp).Error)
}

// Retain the first write while acknowledging the rest of each window in
// reverse order. Check every address, payload, destination and window size.
func (r *rig) holdFirstZeroWrite(node uint64) messaging.Msg {
	r.t.Helper()
	const width = uint64(32)
	var held messaging.Msg
	for offset := uint64(0); offset < nodeBytes; {
		r.run()
		count := min(uint64(7), (nodeBytes-offset)/width)
		if offset == 0 {
			count = 8
		}
		require.Equal(r.t, int(count), r.policy.IncomingBuf().Size()+r.request.IncomingBuf().Size())
		var batch []messaging.Msg
		for range count {
			port := r.policy
			if offset/width%2 == 1 {
				port = r.request
			}
			msg := r.recv(port)
			require.Equal(r.t, memprotocol.WriteReq{PID: 7, Address: node + offset, Data: make([]byte, width)}, msg.Payload)
			if offset == 0 {
				held = msg
			} else {
				batch = append(batch, msg)
			}
			offset += width
		}
		for i := len(batch) - 1; i >= 0; i-- {
			msg := batch[i]
			port := r.policy
			if msg.Dst == r.request.AsRemote() {
				port = r.request
			}
			r.send(port, msg.Src, memprotocol.WriteDoneRsp{}, msg.ID)
		}
	}
	r.run()
	require.Zero(r.t, r.policy.IncomingBuf().Size()+r.request.IncomingBuf().Size(), "a dependent write published early")
	require.Zero(r.t, r.ctrl.IncomingBuf().Size(), "command acknowledged before the held zero write")
	return held
}
