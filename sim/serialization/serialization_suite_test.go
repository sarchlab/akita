package serialization_test

import (
	"bytes"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v4/sim/serialization"
)

func init() {
	serialization.RegisterType(TestType1{})
	serialization.RegisterType(&TestType2{})
}

func TestSerialization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Serialization Suite")
}

type TestType1 struct {
	Value int
}

func (t TestType1) ID() string {
	return "test_type"
}

func (t TestType1) Serialize() (map[string]any, error) {
	return map[string]any{
		"value": t.Value,
	}, nil
}

func (t TestType1) Deserialize(
	data map[string]any,
) (serialization.Serializable, error) {
	t.Value = data["value"].(int)

	return t, nil
}

type TestType2 struct {
	Value int
}

func (t *TestType2) ID() string {
	return "test_type_2"
}

func (t *TestType2) Serialize() (map[string]any, error) {
	return map[string]any{
		"value": t.Value,
	}, nil
}

func (t *TestType2) Deserialize(
	data map[string]any,
) (serialization.Serializable, error) {
	t.Value = data["value"].(int)

	return t, nil
}

var _ = Describe("Serialization", func() {
	var (
		buffer  *bytes.Buffer
		codec   *serialization.JSONCodec
		manager *serialization.Manager
	)

	BeforeEach(func() {
		buffer = &bytes.Buffer{}
		codec = serialization.NewJSONCodec(buffer, buffer)
		manager = serialization.NewManager(codec)
	})

	It("should serialize and deserialize a simple value", func() {
		err := manager.Serialize(1)
		Expect(err).To(BeNil())

		fmt.Println(buffer.String())

		value, err := manager.Deserialize()
		Expect(err).To(BeNil())
		Expect(value).To(Equal(1))
	})

	It("should serialize and deserialize a struct", func() {
		err := manager.Serialize(TestType1{Value: 1})
		Expect(err).To(BeNil())

		fmt.Println(buffer.String())

		value, err := manager.Deserialize()
		Expect(err).To(BeNil())
		Expect(value).To(BeAssignableToTypeOf(TestType1{}))
		Expect(value.(TestType1).Value).To(Equal(1))
	})

	It("should serialize and deserialize a pointer to a struct", func() {
		err := manager.Serialize(&TestType1{Value: 1})
		Expect(err).To(BeNil())

		fmt.Println(buffer.String())

		value, err := manager.Deserialize()
		Expect(err).To(BeNil())
		Expect(value).To(BeAssignableToTypeOf(&TestType1{}))
		Expect(value.(*TestType1).Value).To(Equal(1))
	})

	It("should serialize ptr to primitive", func() {
		val := int(1)
		ptr := &val
		err := manager.Serialize(ptr)
		Expect(err).To(BeNil())

		fmt.Println(buffer.String())

		value, err := manager.Deserialize()
		Expect(err).To(BeNil())
		Expect(value).To(Equal(ptr))
	})

	It("should serialize and deserialize a pointer to a struct, "+
		"with struct values not deserializable", func() {
		err := manager.Serialize(&TestType2{Value: 1})
		Expect(err).To(BeNil())

		fmt.Println(buffer.String())

		value, err := manager.Deserialize()
		Expect(err).To(BeNil())
		Expect(value).To(BeAssignableToTypeOf(&TestType2{}))
		Expect(value.(*TestType2).Value).To(Equal(1))
	})
})
