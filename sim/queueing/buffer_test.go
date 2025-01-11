package queueing

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"go.uber.org/mock/gomock"
)

var _ = Describe("BufferImpl", func() {
	var (
		mockCtrl *gomock.Controller
		sim      *MockSimulation
		buf      Buffer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

		buf = BufferBuilder{}.
			WithSimulation(sim).
			WithCapacity(2).
			Build("Buf")
	})

	It("should allow push and pop", func() {
		Expect(buf.Capacity()).To(Equal(2))
		Expect(buf.CanPush()).To(BeTrue())

		buf.Push(1)
		Expect(buf.CanPush()).To(BeTrue())
		Expect(buf.Size()).To(Equal(1))

		buf.Push(2)
		Expect(buf.CanPush()).To(BeFalse())
		Expect(buf.Size()).To(Equal(2))
		Expect(func() {
			buf.Push(3)
		}).To(Panic())

		Expect(buf.Peek()).To(Equal(1))
		Expect(buf.Pop()).To(Equal(1))
		Expect(buf.Size()).To(Equal(1))
		Expect(buf.Peek()).To(Equal(2))
		Expect(buf.Pop()).To(Equal(2))
		Expect(buf.Size()).To(Equal(0))
		Expect(buf.Peek()).To(BeNil())
		Expect(buf.Pop()).To(BeNil())
	})

	It("should clear", func() {
		buf.Push(2)
		Expect(buf.Size()).To(Equal(1))

		buf.Clear()

		Expect(buf.Size()).To(Equal(0))
		Expect(buf.Peek()).To(BeNil())
	})

	It("should be serializable", func() {
		var err error

		strBuf := bytes.NewBuffer(nil)
		jsonCodec := serialization.NewJSONCodec()
		sManager := serialization.NewManager(jsonCodec)

		buf.Push(1)
		buf.Push(2)

		sManager.StartSerialization()
		_, err = sManager.Serialize(buf.State())
		Expect(err).To(BeNil())
		sManager.FinalizeSerialization(strBuf)

		fmt.Println(strBuf.String())

		buf2 := BufferBuilder{}.
			WithSimulation(sim).
			WithCapacity(2).
			Build("Buf2")

		sManager.StartDeserialization(strBuf)
		state, err := sManager.Deserialize(
			serialization.IDToDeserialize("Buf"))
		Expect(err).To(BeNil())
		buf2.SetState(state.(*bufferState))
		sManager.FinalizeDeserialization()

		Expect(buf2.Size()).To(Equal(2))
		Expect(buf2.Peek()).To(Equal(1))
		Expect(buf2.Pop()).To(Equal(1))
		Expect(buf2.Peek()).To(Equal(2))
		Expect(buf2.Pop()).To(Equal(2))
	})
})
