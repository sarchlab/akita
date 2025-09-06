package idealmemcontrollerv5

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
    "github.com/sarchlab/akita/v4/simv5"
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
        simx := simv5.NewSimulation(engine)
        // Register storage emulation state
        store := mem.NewStorage(1 * mem.MB)
        Expect(simx.RegisterState("dram0", store)).To(Succeed())

        spec := defaults()
        spec.Freq = 1000 * sim.MHz
        spec.LatencyCycles = 5
        spec.StorageRef = "dram0"
        ctrl := MakeBuilder().
            WithSimulation(simx).
            WithSpec(spec).
            Build("MemCtrlV5")

        // Inject ports and attach dummy conn to avoid nil deref on send
        top := sim.NewPort(ctrl, 4, 4, "MemCtrlV5.Top")
        ctrl.AddPort("Top", top)
        dc := &dummyConn{}
        dc.PlugIn(top)

        // Deliver a read req
        read := mem.ReadReqBuilder{}.
            WithSrc(sim.RemotePort("Agent.Port")).
            WithDst(top.AsRemote()).
            WithAddress(0).
            WithByteSize(4).
            Build()
        _ = top.Deliver(read)

        // Tick until response available
        for i := 0; i < 6; i++ { _ = ctrl.Tick() }

        // Retrieve outgoing response
        msg := top.RetrieveOutgoing()
        Expect(msg).NotTo(BeNil())
        _, isDataReady := msg.(*mem.DataReadyRsp)
        Expect(isDataReady).To(BeTrue())
    })

    It("processes a write request and updates storage", func() {
        engine := sim.NewSerialEngine()
        simx := simv5.NewSimulation(engine)
        store := mem.NewStorage(1 * mem.MB)
        Expect(simx.RegisterState("dram0", store)).To(Succeed())

        spec := defaults()
        spec.Freq = 1 * sim.GHz
        spec.LatencyCycles = 3
        spec.StorageRef = "dram0"
        ctrl := MakeBuilder().
            WithSimulation(simx).
            WithSpec(spec).
            Build("MemCtrlV5")

        top := sim.NewPort(ctrl, 4, 4, "MemCtrlV5.Top")
        ctrl.AddPort("Top", top)
        dc := &dummyConn{}
        dc.PlugIn(top)

        data := []byte{1,2,3,4}
        write := mem.WriteReqBuilder{}.
            WithSrc(sim.RemotePort("Agent.Port")).
            WithDst(top.AsRemote()).
            WithAddress(0).
            WithData(data).
            Build()
        _ = top.Deliver(write)

        for i := 0; i < 4; i++ { _ = ctrl.Tick() }

        // Verify response
        msg := top.RetrieveOutgoing()
        Expect(msg).NotTo(BeNil())
        _, isWriteDone := msg.(*mem.WriteDoneRsp)
        Expect(isWriteDone).To(BeTrue())

        // Verify storage content
        stored, err := store.Read(0, 4)
        Expect(err).To(BeNil())
        Expect(stored).To(Equal(data))
    })

    It("drains inflight then responds to control", func() {
        engine := sim.NewSerialEngine()
        simx := simv5.NewSimulation(engine)
        store := mem.NewStorage(1 * mem.MB)
        _ = simx.RegisterState("dram0", store)

        spec := defaults()
        spec.LatencyCycles = 2
        spec.StorageRef = "dram0"
        ctrl := MakeBuilder().
            WithSimulation(simx).
            WithSpec(spec).
            Build("MemCtrlV5")

        top := sim.NewPort(ctrl, 4, 4, "MemCtrlV5.Top")
        ctrl.AddPort("Top", top)
        control := sim.NewPort(ctrl, 2, 2, "MemCtrlV5.Control")
        ctrl.AddPort("Control", control)
        dc := &dummyConn{}
        dc.PlugIn(top)
        dc.PlugIn(control)

        // One inflight read
        read := mem.ReadReqBuilder{}.
            WithSrc(sim.RemotePort("Agent.Port")).
            WithDst(top.AsRemote()).
            WithAddress(16).
            WithByteSize(4).
            Build()
        _ = top.Deliver(read)
        _ = ctrl.Tick()

        // Send drain control
        ctrlMsg := mem.ControlMsgBuilder{}.
            WithSrc(sim.RemotePort("Agent.Ctrl")).
            WithDst(control.AsRemote()).
            WithCtrlInfo(false, true, false, false, false).
            Build()
        _ = control.Deliver(ctrlMsg)

        // Progress a few ticks: first complete read, then drain rsp
        for i := 0; i < 5; i++ { _ = ctrl.Tick() }

        // First, data ready should have been sent
        _ = top.RetrieveOutgoing()

        // Then, control response
        rsp := control.RetrieveOutgoing()
        Expect(rsp).NotTo(BeNil())
        _, isGeneral := rsp.(*sim.GeneralRsp)
        Expect(isGeneral).To(BeTrue())
    })
})
