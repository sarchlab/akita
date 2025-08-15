package wiring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
)

// sampleMsg is a simple message type for testing
type sampleMsg struct {
	sim.MsgMeta

	sendTime sim.VTimeInSec
}

func newSampleMsg() *sampleMsg {
	m := &sampleMsg{}
	m.ID = sim.GetIDGenerator().Generate()

	return m
}

func (m *sampleMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *sampleMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// testComponent is a simple component for testing
type testComponent struct {
	*sim.ComponentBase

	notified bool
}

func newTestComponent(name string) *testComponent {
	c := &testComponent{
		ComponentBase: sim.NewComponentBase(name),
	}

	return c
}

func (c *testComponent) Handle(e sim.Event) error {
	return nil
}

func (c *testComponent) NotifyRecv(port sim.Port) {
	c.notified = true
}

func (c *testComponent) NotifyPortFree(port sim.Port) {
	// Do nothing in test component
}

func (c *testComponent) IsNotified() bool {
	return c.notified
}

// stubTimeTeller is a simple implementation of TimeTeller for testing
type stubTimeTeller struct {
	time sim.VTimeInSec
}

func (t *stubTimeTeller) CurrentTime() sim.VTimeInSec {
	return t.time
}

func (t *stubTimeTeller) AdvanceTime(delta sim.VTimeInSec) {
	t.time += delta
}

var _ = Describe("Wire and Port", func() {
	var (
		comp1 *testComponent
		comp2 *testComponent
		port1 *Port
		port2 *Port
		wire  *Wire
		time  *stubTimeTeller
	)

	BeforeEach(func() {
		time = &stubTimeTeller{}
		comp1 = newTestComponent("Comp1")
		comp2 = newTestComponent("Comp2")
		port1 = NewPort(comp1, "Port1", time)
		port2 = NewPort(comp2, "Port2", time)
		wire = ConnectWithWire(port1, port2)
	})

	It("should connect ports with wire", func() {
		Expect(wire.Name()).To(Equal("Port1-Port2"))
		Expect(port1.Component()).To(Equal(comp1))
		Expect(port2.Component()).To(Equal(comp2))
	})

	It("should not allow connecting more than two ports", func() {
		port3 := NewPort(comp1, "Port3", time)
		Expect(func() { wire.PlugIn(port3) }).
			To(PanicWith("wire already has two ports connected"))
	})

	It("should not allow connecting non-wiring.Port", func() {
		otherPort := sim.NewPort(comp1, 1, 1, "OtherPort")
		Expect(func() { wire.PlugIn(otherPort) }).
			To(PanicWith("wire can only connect to wiring.Port"))
	})

	It("should not allow sending when src is not the port", func() {
		msg := newSampleMsg()
		msg.Src = port2.AsRemote()
		msg.Dst = port1.AsRemote()

		Expect(func() { port1.Send(msg) }).
			To(PanicWith("message src must be the port"))
	})

	It("should not allow sending when port is busy", func() {
		msg1 := newSampleMsg()
		msg1.Src = port1.AsRemote()
		msg1.Dst = port2.AsRemote()

		err := port1.Send(msg1)
		Expect(err).To(BeNil())

		msg2 := newSampleMsg()
		msg2.Src = port1.AsRemote()
		msg2.Dst = port2.AsRemote()

		err = port1.Send(msg2)
		Expect(err).NotTo(BeNil())
	})

	It("should notify components when messages are available", func() {
		msg := newSampleMsg()
		msg.Src = port1.AsRemote()
		msg.Dst = port2.AsRemote()

		// Send message from port1
		err := port1.Send(msg)
		Expect(err).To(BeNil())

		// Component2 should be notified
		Expect(comp2.IsNotified()).To(BeTrue())
	})

	It("should allow peeking and retrieving messages across cycles", func() {
		msg := newSampleMsg()
		msg.Src = port1.AsRemote()
		msg.Dst = port2.AsRemote()

		// First cycle: comp1 sends message
		err := port1.Send(msg)
		Expect(err).To(BeNil())

		time.AdvanceTime(1)

		// Second cycle: comp2 can peek the message
		peekedMsg := port2.PeekIncoming()
		Expect(peekedMsg).To(Equal(msg))
		Expect(port2.PeekIncoming()).
			To(Equal(msg)) // Message should still be there after peeking

		// Third cycle: comp2 retrieves the message
		retrievedMsg := port2.RetrieveIncoming()
		Expect(retrievedMsg).To(Equal(msg))

		// Fourth cycle: message should be gone after retrieval
		Expect(port2.PeekIncoming()).To(BeNil())
		Expect(port2.RetrieveIncoming()).To(BeNil())
	})

	It("should not allow same-cycle access", func() {
		msg := newSampleMsg()
		msg.Src = port1.AsRemote()
		msg.Dst = port2.AsRemote()

		err := port1.Send(msg)
		Expect(err).To(BeNil())

		// Try to peek in the same cycle
		Expect(port2.PeekIncoming()).To(BeNil())

		// Advance time by one cycle
		time.AdvanceTime(1)

		// Now we should be able to peek
		Expect(port2.PeekIncoming()).To(Equal(msg))
	})
})
