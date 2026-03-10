package wiring

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/simulation"
)

// samplePayload is a simple payload type for testing
type samplePayload struct {
	sendTime sim.VTimeInSec
}

// wireTestComponent is a component that can send and receive messages
type wireTestComponent struct {
	*sim.TickingComponent

	port         *Port
	msgsToSend   []*sim.GenericMsg
	msgsReceived []*sim.GenericMsg
}

func newWireTestComponent(engine sim.Engine, name string) *wireTestComponent {
	c := &wireTestComponent{
		msgsToSend:   make([]*sim.GenericMsg, 0),
		msgsReceived: make([]*sim.GenericMsg, 0),
	}

	c.TickingComponent =
		sim.NewTickingComponent(name, engine, 1, c)

	c.port = NewPort(c, name+".Port", engine)

	return c
}

func (c *wireTestComponent) Tick() bool {
	madeProgress := false

	// Try to receive messages
	msg := c.port.RetrieveIncoming()
	if msg != nil {
		c.msgsReceived = append(c.msgsReceived, msg)
		madeProgress = true

		now := c.CurrentTime()
		payload := msg.Payload.(*samplePayload)
		Expect(payload.sendTime).To(Equal(now - 1))
	}

	// Try to send messages
	if len(c.msgsToSend) > 0 {
		msg := c.msgsToSend[0]
		msg.Payload.(*samplePayload).sendTime = c.CurrentTime()

		err := c.port.Send(c.msgsToSend[0])
		if err == nil {
			c.msgsToSend = c.msgsToSend[1:]
			madeProgress = true
		}
	}

	return madeProgress
}

func (c *wireTestComponent) Ports() []sim.Port {
	return []sim.Port{c.port}
}

var _ = Describe("Wire Integration", func() {
	var (
		s     *simulation.Simulation
		comp1 *wireTestComponent
		comp2 *wireTestComponent
	)

	BeforeEach(func() {
		s = simulation.MakeBuilder().WithoutMonitoring().Build()
		comp1 = newWireTestComponent(s.GetEngine(), "Comp1")
		comp2 = newWireTestComponent(s.GetEngine(), "Comp2")
		ConnectWithWire(comp1.port, comp2.port)
		s.RegisterComponent(comp1)
		s.RegisterComponent(comp2)
	})

	AfterEach(func() {
		s.Terminate()
		os.RemoveAll("*.sqlite3")
	})

	It("should deliver messages one cycle after they are sent", func() {
		// Create 10 messages to send
		for i := 0; i < 10; i++ {
			msg := &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:  sim.GetIDGenerator().Generate(),
					Src: comp1.port.AsRemote(),
					Dst: comp2.port.AsRemote(),
				},
				Payload: &samplePayload{},
			}
			comp1.msgsToSend = append(comp1.msgsToSend, msg)
		}

		tick := sim.MakeTickEvent(comp1, 1)
		s.GetEngine().Schedule(tick)

		s.GetEngine().Run()

		Expect(comp2.msgsReceived).To(HaveLen(10))
		Expect(comp1.msgsToSend).To(HaveLen(0))
	})

	It("should handle bidirectional message passing", func() {
		// Create messages in both directions
		for i := 0; i < 5; i++ {
			msg1 := &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:  sim.GetIDGenerator().Generate(),
					Src: comp1.port.AsRemote(),
					Dst: comp2.port.AsRemote(),
				},
				Payload: &samplePayload{},
			}
			comp1.msgsToSend = append(comp1.msgsToSend, msg1)

			msg2 := &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:  sim.GetIDGenerator().Generate(),
					Src: comp2.port.AsRemote(),
					Dst: comp1.port.AsRemote(),
				},
				Payload: &samplePayload{},
			}
			comp2.msgsToSend = append(comp2.msgsToSend, msg2)
		}

		tick1 := sim.MakeTickEvent(comp1, 1)
		tick2 := sim.MakeTickEvent(comp2, 1)
		s.GetEngine().Schedule(tick1)
		s.GetEngine().Schedule(tick2)

		// Run until all messages are processed
		s.GetEngine().Run()

		// Verify all messages were received
		Expect(comp1.msgsReceived).To(HaveLen(5))
		Expect(comp2.msgsReceived).To(HaveLen(5))
		Expect(comp1.msgsToSend).To(HaveLen(0))
		Expect(comp2.msgsToSend).To(HaveLen(0))
	})
})
