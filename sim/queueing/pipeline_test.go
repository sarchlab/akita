package queueing

import (
	"bytes"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"go.uber.org/mock/gomock"
)

func init() {
	serialization.RegisterType(reflect.TypeOf(&pipelineItem{}))
}

type pipelineItem struct {
	taskID string
}

func (p pipelineItem) TaskID() string {
	return p.taskID
}

func (p *pipelineItem) Name() string {
	return p.taskID
}

func (p *pipelineItem) Serialize() (map[string]any, error) {
	return map[string]any{
		"taskID": p.taskID,
	}, nil
}

func (p *pipelineItem) Deserialize(data map[string]any) error {
	p.taskID = data["taskID"].(string)
	return nil
}

var _ = Describe("Pipeline", func() {
	var (
		mockCtrl           *gomock.Controller
		sim                *MockSimulation
		postPipelineBuffer *bufferImpl
		pipeline           Pipeline
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

		postPipelineBuffer = BufferBuilder{}.
			WithSimulation(sim).
			WithCapacity(1).
			Build("PostPipelineBuffer").(*bufferImpl)
		pipeline = MakePipelineBuilder().
			WithSimulation(sim).
			WithPipelineWidth(1).
			WithNumStage(100).
			WithCyclePerStage(2).
			WithPostPipelineBuffer(postPipelineBuffer).
			Build("Pipeline")
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should process items in pipeline", func() {
		item1 := &pipelineItem{taskID: "1"}
		item2 := &pipelineItem{taskID: "2"}

		// Cycle 0, inject item1
		canAccept1 := pipeline.CanAccept()
		Expect(canAccept1).To(BeTrue())
		pipeline.Accept(item1)
		canAccept2 := pipeline.CanAccept()
		Expect(canAccept2).To(BeFalse())

		// Cycle 1, process item1
		madeProgress1 := pipeline.Tick()
		Expect(madeProgress1).To(BeTrue())
		canAccept3 := pipeline.CanAccept()
		Expect(canAccept3).To(BeFalse())

		// Cycle 3, process item1, inject item2
		madeProgress2 := pipeline.Tick()
		Expect(madeProgress2).To(BeTrue())
		canAccept4 := pipeline.CanAccept()
		Expect(canAccept4).To(BeTrue())
		pipeline.Accept(item2)

		// Cycle 4-198, process item1 and item2
		for i := 2; i < 199; i++ {
			madeProgress := pipeline.Tick()
			Expect(madeProgress).To(BeTrue())
			Expect(postPipelineBuffer.Size()).To(Equal(0))
		}

		// Cycle 199, pop item1
		madeProgress3 := pipeline.Tick()
		Expect(madeProgress3).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))

		// Cycle 200, process item2
		madeProgress4 := pipeline.Tick()
		Expect(madeProgress4).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))

		// Cycle 201, pop item 2 failed
		madeProgress5 := pipeline.Tick()
		Expect(madeProgress5).To(BeFalse())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))

		// Cycle 202, remove item 1 and pop item 2
		postPipelineBuffer.Pop()
		madeProgress6 := pipeline.Tick()
		Expect(madeProgress6).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item2))
	})

	It("should be serializable", func() {
		var err error

		strBuf := bytes.NewBuffer(nil)
		jsonCodec := serialization.NewJSONCodec()
		sManager := serialization.NewManager(jsonCodec)

		// Insert some items into the pipeline and run a few ticks.
		item1 := &pipelineItem{taskID: "1"}
		item2 := &pipelineItem{taskID: "2"}
		pipeline.Accept(item1)
		pipeline.Tick()
		pipeline.Tick()
		pipeline.Accept(item2)

		// Advance the pipeline enough so that it has items in-flight.
		for i := 0; i < 10; i++ {
			pipeline.Tick()
		}

		// Serialize the pipeline state.
		sManager.StartSerialization()
		_, err = sManager.Serialize(pipeline.State())
		Expect(err).To(BeNil())
		sManager.FinalizeSerialization(strBuf)

		// Build a second pipeline instance to deserialize into.
		postPipelineBuffer2 := BufferBuilder{}.
			WithSimulation(sim).
			WithCapacity(1).
			Build("PostPipelineBuffer2").(*bufferImpl)
		pipeline2 := MakePipelineBuilder().
			WithSimulation(sim).
			WithPipelineWidth(1).
			WithNumStage(100).
			WithCyclePerStage(2).
			WithPostPipelineBuffer(postPipelineBuffer2).
			Build("Pipeline2")

		// Deserialize the saved state into pipeline2.
		sManager.StartDeserialization(strBuf)
		state, err := sManager.Deserialize(
			serialization.IDToDeserialize("Pipeline"))
		Expect(err).To(BeNil())
		pipeline2.SetState(state.(*pipelineState))
		sManager.FinalizeDeserialization()

		// After deserialization, pipeline2 should resume as if it had run the
		// same sequence. Run enough ticks so items should appear in the post
		// pipeline buffer.
		for i := 0; i < 200; i++ {
			pipeline2.Tick()
		}

		// Check that the items eventually emerge in the correct order.
		Expect(postPipelineBuffer2.Size()).To(Equal(1))
		Expect(postPipelineBuffer2.Pop()).To(Equal(item1))

		pipeline2.Tick()
		Expect(postPipelineBuffer2.Pop()).To(Equal(item2))
	})
})

var _ = Describe("Zero-Stage Pipeline", func() {
	var (
		mockCtrl           *gomock.Controller
		sim                *MockSimulation
		postPipelineBuffer *bufferImpl
		pipeline           Pipeline
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		sim = NewMockSimulation(mockCtrl)

		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

		postPipelineBuffer = BufferBuilder{}.
			WithSimulation(sim).
			WithCapacity(1).
			Build("PostPipelineBuffer").(*bufferImpl)
		pipeline = MakePipelineBuilder().
			WithSimulation(sim).
			WithPipelineWidth(1).
			WithNumStage(0).
			WithCyclePerStage(2).
			WithPostPipelineBuffer(postPipelineBuffer).
			Build("Pipeline")

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should not accept if post buffer is full", func() {
		item1 := pipelineItem{taskID: "1"}
		postPipelineBuffer.Push(item1)

		canAccept := pipeline.CanAccept()

		Expect(canAccept).To(BeFalse())
	})

	It("should forward to post buffer directory", func() {
		item1 := &pipelineItem{taskID: "1"}

		canAccept := pipeline.CanAccept()
		pipeline.Accept(item1)

		Expect(canAccept).To(BeTrue())
		Expect(postPipelineBuffer.Size()).To(Equal(1))
		Expect(postPipelineBuffer.Peek()).To(Equal(item1))
	})
})
