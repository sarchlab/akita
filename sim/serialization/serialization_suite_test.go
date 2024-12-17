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
	serialization.RegisterType(&TestType1{})
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
	t.Value = int(data["value"].(float64))

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
})
