package writethroughcache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/noc/directconnection"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("Cache", func() {
	var (
		engine              timing.Engine
		connection          messaging.Connection
		addressToPortMapper mem.AddressToPortMapper
		dram                *idealmemcontroller.Comp
		dramStorage         *mem.Storage
		cuPort              messaging.Port
		c                   *modeling.Component[Spec, State, Resources]
	)

	// drainResponses retrieves every message that has been delivered to cuPort.
	drainResponses := func() []messaging.Msg {
		msgs := []messaging.Msg{}
		for {
			msg := cuPort.RetrieveIncoming()
			if msg == nil {
				break
			}
			msgs = append(msgs, msg)
		}
		return msgs
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		connection = directconnection.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Conn")

		// cuPort is a real, component-less port that stands in for the compute
		// unit. It is plugged into the connection so the cache's responses land
		// in its incoming buffer, which the tests then drain and inspect.
		cuPort = messaging.NewPort(nil, 16, 16, "cuPort")

		dramStorage = mem.NewStorage(4 * mem.GB)
		dram = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(idealmemcontroller.Resources{Storage: dramStorage}).
			Build("DRAM")
		dram.AssignPort("Top",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Top"))
		dram.AssignPort("Control",
			messaging.NewPort(dram, 16, 16, dram.Name()+".Control"))
		addressToPortMapper = &mem.SinglePortMapper{
			Port: dram.GetPortByName("Top").AsRemote(),
		}

		c = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{
				AddressMapper: addressToPortMapper,
			}).
			Build("Cache")

		connection.PlugIn(dram.GetPortByName("Top"))
		connection.PlugIn(c.GetPortByName("Top"))
		connection.PlugIn(c.GetPortByName("Bottom"))
		connection.PlugIn(cuPort)
	})

	It("should do read miss", func() {
		dramStorage.Write(0x100, []byte{1, 2, 3, 4})
		read := mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = cuPort.AsRemote()
		read.Dst = c.GetPortByName("Top").AsRemote()
		read.Address = 0x100
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(read)

		engine.Run()

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		dr := rsps[0].(mem.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should do read miss coalesce", func() {
		dramStorage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := mem.ReadReq{}
		read1.ID = timing.GetIDGenerator().Generate()
		read1.Src = cuPort.AsRemote()
		read1.Dst = c.GetPortByName("Top").AsRemote()
		read1.Address = 0x100
		read1.AccessByteSize = 4
		read1.TrafficBytes = 12
		read1.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(read1)

		read2 := mem.ReadReq{}
		read2.ID = timing.GetIDGenerator().Generate()
		read2.Src = cuPort.AsRemote()
		read2.Dst = c.GetPortByName("Top").AsRemote()
		read2.Address = 0x104
		read2.AccessByteSize = 4
		read2.TrafficBytes = 12
		read2.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(read2)

		engine.Run()

		// Without coalescing, the MSHR-hit transaction (read2) may be
		// finalized before the fetcher (read1). Accept responses in
		// any order as long as both data values are received.
		received := make(map[string]bool)
		for _, msg := range drainResponses() {
			dr := msg.(mem.DataReadyRsp)
			if string(dr.Data) == string([]byte{1, 2, 3, 4}) {
				received["1234"] = true
			} else if string(dr.Data) == string([]byte{5, 6, 7, 8}) {
				received["5678"] = true
			}
		}

		Expect(received["1234"]).To(BeTrue())
		Expect(received["5678"]).To(BeTrue())
	})

	It("should do read hit", func() {
		dramStorage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := mem.ReadReq{}
		read1.ID = timing.GetIDGenerator().Generate()
		read1.Src = cuPort.AsRemote()
		read1.Dst = c.GetPortByName("Top").AsRemote()
		read1.Address = 0x100
		read1.AccessByteSize = 4
		read1.TrafficBytes = 12
		read1.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(read1)
		engine.Run()
		t1 := engine.CurrentTime()

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].(mem.DataReadyRsp).Data).To(Equal([]byte{1, 2, 3, 4}))

		read2 := mem.ReadReq{}
		read2.ID = timing.GetIDGenerator().Generate()
		read2.Src = cuPort.AsRemote()
		read2.Dst = c.GetPortByName("Top").AsRemote()
		read2.Address = 0x104
		read2.AccessByteSize = 4
		read2.TrafficBytes = 12
		read2.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(read2)
		engine.Run()
		t2 := engine.CurrentTime()

		rsps = drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].(mem.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))

		Expect(t2 - t1).To(BeNumerically("<", t1))
	})

	It("should write partial line", func() {
		write := mem.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Src = cuPort.AsRemote()
		write.Dst = c.GetPortByName("Top").AsRemote()
		write.Address = 0x100
		write.Data = []byte{1, 2, 3, 4}
		write.TrafficBytes = 4 + 12
		write.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(write)

		engine.Run()

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].Meta().RspTo).To(Equal(write.ID))

		data, _ := dramStorage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should write full line", func() {
		write := mem.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Src = cuPort.AsRemote()
		write.Dst = c.GetPortByName("Top").AsRemote()
		write.Address = 0x100
		write.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		write.TrafficBytes = 64 + 12
		write.TrafficClass = "req"
		c.GetPortByName("Top").Deliver(write)
		engine.Run()

		rsps := drainResponses()
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].Meta().RspTo).To(Equal(write.ID))

		data, _ := dramStorage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

})
