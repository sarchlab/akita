package writethroughcache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"

	"github.com/sarchlab/akita/v5/sim/messaging/direct"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(name string, bufSize int) messaging.Port {
	p := twowaybuffered.NewPort(bufSize, bufSize)
	p.BindOwner(testDriver{}, name)

	return p
}

var _ = Describe("Cache", func() {
	var (
		engine              timing.Engine
		sim                 timing.Simulation
		connection          messaging.Connection
		addressToPortMapper mem.AddressToPortMapper
		dram                *idealmemcontroller.Comp
		dramStorage         *mem.Storage
		cuPort              messaging.Port
		c                   *Comp
	)

	// drainResponses retrieves every message that has been delivered to cuPort.
	drainResponses := func() []messaging.Msg {
		msgs := []messaging.Msg{}
		for {
			msg, ok := cuPort.RetrieveIncoming()
			if !ok {
				break
			}
			msgs = append(msgs, msg)
		}
		return msgs
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		connection = direct.NewConnection("Conn", sim, timing.GHz)

		// cuPort is a real, component-less port that stands in for the compute
		// unit. It is plugged into the connection so the cache's responses land
		// in its incoming buffer, which the tests then drain and inspect.
		cuPort = newDriverPort("CU.Top", 16)

		dramStorage = mem.NewStorage(4 * mem.GB)
		dram = idealmemcontroller.Definition.Builder().
			WithSimulation(sim).
			WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
			Build("DRAM")

		dram.BindPort("Top", twowaybuffered.NewPort(16, 16))
		dram.BindPort("Control", twowaybuffered.NewPort(16, 16))

		addressToPortMapper = &mem.SinglePortMapper{
			Port: dram.Ports.Top.AsRemote(),
		}

		// The system builder chooses the port buffer sizes. Control is
		// unused here but every port must still be given.
		c = Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{
				Storage:       mem.NewStorage(Definition.DefaultSpec.TotalByteSize),
				AddressMapper: addressToPortMapper,
			}).
			Build("Cache")

		c.BindPort("Top", twowaybuffered.NewPort(4, 4))
		c.BindPort("Bottom", twowaybuffered.NewPort(4, 4))
		c.BindPort("Control", twowaybuffered.NewPort(4, 4))

		connection.BindPort(dram.Ports.Top)
		connection.BindPort(c.Ports.Top)
		connection.BindPort(c.Ports.Bottom)
		connection.BindPort(cuPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

	})

	It("should do read miss", func() {
		dramStorage.Write(0x100, []byte{1, 2, 3, 4})
		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read)

		Expect(engine.Run()).To(Succeed())

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		dr := rsps[0]
		Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should do read miss coalesce", func() {
		dramStorage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read1)

		read2 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x104,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read2)

		Expect(engine.Run()).To(Succeed())

		// Without coalescing, the MSHR-hit transaction (read2) may be
		// finalized before the fetcher (read1). Accept responses in
		// any order as long as both data values are received.
		received := make(map[string]bool)
		for _, msg := range drainResponses() {
			dr := msg
			if string(dr.Payload.(memprotocol.DataReadyRsp).Data) == string([]byte{1, 2, 3, 4}) {
				received["1234"] = true
			} else if string(dr.Payload.(memprotocol.DataReadyRsp).Data) == string([]byte{5, 6, 7, 8}) {
				received["5678"] = true
			}
		}

		Expect(received["1234"]).To(BeTrue())
		Expect(received["5678"]).To(BeTrue())
	})

	It("should do read hit", func() {
		dramStorage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read1)
		Expect(engine.Run()).To(Succeed())
		t1 := engine.CurrentTime()

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{1, 2, 3, 4}))

		read2 := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x104,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(read2)
		Expect(engine.Run()).To(Succeed())
		t2 := engine.CurrentTime()

		rsps = drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))

		Expect(t2 - t1).To(BeNumerically("<", t1))
	})

	It("should write partial line", func() {
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100,
			Data:    []byte{1, 2, 3, 4}},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 4 + 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(write)

		Expect(engine.Run()).To(Succeed())

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].RspTo).To(Equal(write.ID))

		data := dramStorage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should write full line", func() {
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100,
			Data: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}},
			ID:  sim.NewID(),
			Src: cuPort.AsRemote(),
			Dst: c.Ports.Top.AsRemote(),

			TrafficBytes: 64 + 12,
			TrafficClass: "req"}

		c.Ports.Top.Deliver(write)
		Expect(engine.Run()).To(Succeed())

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].RspTo).To(Equal(write.ID))

		data := dramStorage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

})
