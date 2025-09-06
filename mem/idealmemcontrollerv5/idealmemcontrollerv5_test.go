package idealmemcontrollerv5

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
)

// dummyConn is a minimal Connection to avoid nil panics from Port.Send.
type dummyConn struct{ sim.HookableBase }

func (d *dummyConn) Name() string              { return "dummy" }
func (d *dummyConn) PlugIn(p sim.Port)         { p.SetConnection(d) }
func (d *dummyConn) Unplug(_ sim.Port)         {}
func (d *dummyConn) NotifyAvailable(_ sim.Port) {}
func (d *dummyConn) NotifySend()               {}

var _ = Describe("IdealMemControllerV5", func() {
    It("processes a read request after latency cycles", func() {
        engine := sim.NewSerialEngine()
        ctrl := MakeBuilder().
            WithEngine(engine).
            WithFreq(1000 * sim.MHz).
            WithLatency(5).
            WithTopBufSize(4).
            Build("MemCtrlV5")

        // Attach dummy conn to avoid nil deref on send
        dc := &dummyConn{}
        dc.PlugIn(ctrl.IO.Top)

        // Deliver a read req
        read := mem.ReadReqBuilder{}.
            WithSrc(sim.RemotePort("Agent.Port")).
            WithDst(ctrl.IO.Top.AsRemote()).
            WithAddress(0).
            WithByteSize(4).
            Build()
        _ = ctrl.IO.Top.Deliver(read)

        // Tick until response available
        for i := 0; i < 6; i++ { _ = ctrl.Tick() }

        // Retrieve outgoing response
        msg := ctrl.IO.Top.RetrieveOutgoing()
        Expect(msg).NotTo(BeNil())
        _, isDataReady := msg.(*mem.DataReadyRsp)
        Expect(isDataReady).To(BeTrue())
    })

    It("processes a write request and updates storage", func() {
        engine := sim.NewSerialEngine()
        ctrl := MakeBuilder().
            WithEngine(engine).
            WithFreq(1 * sim.GHz).
            WithLatency(3).
            Build("MemCtrlV5")

        dc := &dummyConn{}
        dc.PlugIn(ctrl.IO.Top)

        data := []byte{1,2,3,4}
        write := mem.WriteReqBuilder{}.
            WithSrc(sim.RemotePort("Agent.Port")).
            WithDst(ctrl.IO.Top.AsRemote()).
            WithAddress(0).
            WithData(data).
            Build()
        _ = ctrl.IO.Top.Deliver(write)

        for i := 0; i < 4; i++ { _ = ctrl.Tick() }

        // Verify response
        msg := ctrl.IO.Top.RetrieveOutgoing()
        Expect(msg).NotTo(BeNil())
        _, isWriteDone := msg.(*mem.WriteDoneRsp)
        Expect(isWriteDone).To(BeTrue())

        // Verify storage content
        stored, err := ctrl.Storage.Read(0, 4)
        Expect(err).To(BeNil())
        Expect(stored).To(Equal(data))
    })

    It("drains inflight then responds to control", func() {
        engine := sim.NewSerialEngine()
        ctrl := MakeBuilder().
            WithEngine(engine).
            WithLatency(2).
            WithTopBufSize(4).
            WithCtrlBufSize(2).
            Build("MemCtrlV5")

        dc := &dummyConn{}
        dc.PlugIn(ctrl.IO.Top)
        dc.PlugIn(ctrl.IO.Control)

        // One inflight read
        read := mem.ReadReqBuilder{}.
            WithSrc(sim.RemotePort("Agent.Port")).
            WithDst(ctrl.IO.Top.AsRemote()).
            WithAddress(16).
            WithByteSize(4).
            Build()
        _ = ctrl.IO.Top.Deliver(read)
        _ = ctrl.Tick()

        // Send drain control
        ctrlMsg := mem.ControlMsgBuilder{}.
            WithSrc(sim.RemotePort("Agent.Ctrl")).
            WithDst(ctrl.IO.Control.AsRemote()).
            WithCtrlInfo(false, true, false, false, false).
            Build()
        _ = ctrl.IO.Control.Deliver(ctrlMsg)

        // Progress a few ticks: first complete read, then drain rsp
        for i := 0; i < 5; i++ { _ = ctrl.Tick() }

        // First, data ready should have been sent
        _ = ctrl.IO.Top.RetrieveOutgoing()

        // Then, control response
        rsp := ctrl.IO.Control.RetrieveOutgoing()
        Expect(rsp).NotTo(BeNil())
        _, isGeneral := rsp.(*sim.GeneralRsp)
        Expect(isGeneral).To(BeTrue())
    })
})
