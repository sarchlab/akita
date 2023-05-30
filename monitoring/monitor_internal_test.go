package monitoring

import (
	"reflect"

	"github.com/sarchlab/akita/v3/sim"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type sampleStruct struct {
	field1 int
	field2 string
	field3 *sampleStruct
	field4 []sampleStruct
}

type sampleComponent struct {
	*sim.ComponentBase

	buffer sim.Buffer
}

func (c *sampleComponent) Handle(_ sim.Event) error {
	return nil
}

func (c *sampleComponent) NotifyRecv(_ sim.VTimeInSec, _ sim.Port) {
	// Do nothing
}

func (c *sampleComponent) NotifyPortFree(_ sim.VTimeInSec, _ sim.Port) {
	// Do nothing
}

func newSampleComponent() *sampleComponent {
	c := &sampleComponent{
		ComponentBase: sim.NewComponentBase("Comp"),
		buffer:        sim.NewBuffer("Comp.Buf", 10),
	}

	c.AddPort("Port1", sim.NewLimitNumMsgPort(c, 2, "Comp.Port1"))

	return c
}

var _ = Describe("Monitor", func() {
	var (
		m *Monitor
	)

	BeforeEach(func() {
		m = &Monitor{}
	})

	It("should register components and internal buffers", func() {
		c := newSampleComponent()
		m.RegisterComponent(c)

		Expect(m.components).To(HaveLen(1))
		Expect(m.buffers).To(HaveLen(2))
	})

	It("should walk int fields", func() {
		s := &sampleStruct{
			field1: 1,
		}

		elem, err := m.walkFields(s, "field1")

		Expect(err).To(BeNil())
		Expect(elem.Kind()).To(Equal(reflect.Int))
		Expect(elem.Type().Name()).To(Equal("int"))
		Expect(elem.Int()).To(Equal(int64(1)))
	})

	It("should walk string fields", func() {
		s := &sampleStruct{
			field2: "abc",
		}

		elem, err := m.walkFields(s, "field2")

		Expect(err).To(BeNil())
		Expect(elem.Kind()).To(Equal(reflect.String))
		Expect(elem.Type().Name()).To(Equal("string"))
		Expect(elem.String()).To(Equal("abc"))
	})

	It("should walk struct", func() {
		s := &sampleStruct{
			field3: &sampleStruct{},
		}

		elem, err := m.walkFields(s, "field3")

		Expect(err).To(BeNil())

		Expect(elem.Kind()).To(Equal(reflect.Struct))
		Expect(elem.Type().Name()).To(Equal("sampleStruct"))
	})

	It("should walk recursively", func() {
		s := &sampleStruct{
			field3: &sampleStruct{
				field1: 1,
			},
		}

		elem, err := m.walkFields(s, "field3.field1")

		Expect(err).To(BeNil())
		Expect(elem.Kind()).To(Equal(reflect.Int))
		Expect(elem.Type().Name()).To(Equal("int"))
		Expect(elem.Int()).To(Equal(int64(1)))
	})

	It("should walk slice", func() {
		s := &sampleStruct{
			field4: []sampleStruct{{}, {}},
		}

		elem, err := m.walkFields(s, "field4")

		Expect(err).To(BeNil())
		Expect(elem.Kind()).To(Equal(reflect.Slice))
	})

	It("should walk slice recursively", func() {
		s := &sampleStruct{
			field4: []sampleStruct{{
				field4: []sampleStruct{
					{field1: 1},
				},
			}, {}},
		}

		elem, err := m.walkFields(s, "field4.0.field4.0.field1")

		Expect(err).To(BeNil())
		Expect(elem.Kind()).To(Equal(reflect.Int))
		Expect(elem.Type().Name()).To(Equal("int"))
		Expect(elem.Int()).To(Equal(int64(1)))
	})
})
