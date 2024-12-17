package serialization_test

import (
	"bytes"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v4/sim/serialization"
)

func TestSerialization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Serialization Suite")
}

type TestType struct {
	Value int
}

func (t *TestType) ID() string {
	return "test_type"
}

func (t *TestType) Serialize() (map[string]any, error) {
	return map[string]any{
		"value": t.Value,
	}, nil
}

func (t *TestType) Deserialize(data map[string]any) error {
	t.Value = data["value"].(int)
	return nil
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

	It("should register a type", func() {
		err := serialization.RegisterType(&TestType{})
		Expect(err).To(BeNil())
	})

	It("should serialize and deserialize a simple value", func() {
		err := manager.Serialize(1)
		Expect(err).To(BeNil())

		fmt.Println(buffer.String())

		var value int
		err = manager.Deserialize(&value)
		Expect(err).To(BeNil())
		Expect(value).To(Equal(1))
	})
})
