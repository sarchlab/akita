package serialization_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v4/sim/serialization"
)

func init() {
	serialization.RegisterType(reflect.TypeOf(&TestType1{}))
	serialization.RegisterType(reflect.TypeOf(&TestType2{}))
	serialization.RegisterType(reflect.TypeOf(&TestType3{}))
}

func TestSerialization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Serialization Suite")
}

type TestType1 struct {
	id             string
	v1             int
	NonSerializing int
}

func (t *TestType1) ID() string {
	return t.id
}

func (t *TestType1) Serialize() (map[string]any, error) {
	return map[string]any{
		"v1": t.v1,
	}, nil
}

func (t *TestType1) Deserialize(
	data map[string]any,
) error {
	t.v1 = data["v1"].(int)

	return nil
}

type TestType2 struct {
	id  string
	v2  int
	Ptr *TestType1
}

func (t *TestType2) ID() string {
	return t.id
}

func (t *TestType2) Serialize() (map[string]any, error) {
	return map[string]any{
		"v2":  t.v2,
		"Ptr": t.Ptr,
	}, nil
}

func (t *TestType2) Deserialize(
	data map[string]any,
) error {
	t.v2 = data["v2"].(int)

	if data["Ptr"] == nil {
		t.Ptr = nil
	} else {
		t.Ptr = data["Ptr"].(*TestType1)
	}

	return nil
}

type TestType3 struct {
	Value int
	Data  []byte
	deps  []*TestType1
}

func (t *TestType3) ID() string {
	return "test_type_3"
}

func (t *TestType3) Serialize() (map[string]any, error) {
	return map[string]any{
		"Value": t.Value,
		"Data":  t.Data,
		"deps":  t.deps,
	}, nil
}

func (t *TestType3) Deserialize(
	data map[string]any,
) error {
	t.Value = data["Value"].(int)

	for _, v := range data["Data"].([]any) {
		t.Data = append(t.Data, v.(byte))
	}

	for i, depMap := range data["deps"].([]any) {
		var dep *TestType1
		if i < len(t.deps) {
			dep = t.deps[i]
		} else {
			dep = &TestType1{}
			t.deps = append(t.deps, dep)
		}

		err := dep.Deserialize(depMap.(map[string]any))
		if err != nil {
			return err
		}
	}

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
		codec = serialization.NewJSONCodec()
		manager = serialization.NewManager(codec)
	})

	It("should serialize a simple serializable", func() {
		s := &TestType1{id: "1", v1: 1}
		manager.StartSerialization()
		_, err := manager.Serialize(s)
		Expect(err).To(BeNil())
		manager.FinalizeSerialization(buffer)

		fmt.Println(buffer.String())

		manager.StartDeserialization(buffer)
		result, err := manager.Deserialize(serialization.IDToDeserialize("1"))
		Expect(err).To(BeNil())
		Expect(result).To(BeAssignableToTypeOf(&TestType1{}))
		Expect(result.(*TestType1).v1).To(Equal(1))
		manager.FinalizeDeserialization()
	})

	It("should serialize nested serializable", func() {
		s := &TestType2{
			id:  "2",
			v2:  1,
			Ptr: &TestType1{id: "1", v1: 2},
		}

		manager.StartSerialization()
		_, err := manager.Serialize(s)
		Expect(err).To(BeNil())
		manager.FinalizeSerialization(buffer)

		fmt.Println(buffer.String())

		manager.StartDeserialization(buffer)
		result, err := manager.Deserialize(serialization.IDToDeserialize("2"))
		Expect(err).To(BeNil())
		Expect(result).To(BeAssignableToTypeOf(&TestType2{}))
		Expect(result.(*TestType2).v2).To(Equal(1))
		Expect(result.(*TestType2).Ptr.v1).To(Equal(2))
		manager.FinalizeDeserialization()
	})

	It("should serialized if field is nil", func() {
		s := &TestType2{
			id: "2",
			v2: 1,
		}

		manager.StartSerialization()
		_, err := manager.Serialize(s)
		Expect(err).To(BeNil())
		manager.FinalizeSerialization(buffer)

		fmt.Println(buffer.String())

		manager.StartDeserialization(buffer)
		result, err := manager.Deserialize(serialization.IDToDeserialize("2"))
		Expect(err).To(BeNil())
		Expect(result).To(BeAssignableToTypeOf(&TestType2{}))
		Expect(result.(*TestType2).v2).To(Equal(1))
		Expect(result.(*TestType2).Ptr).To(BeNil())
		manager.FinalizeDeserialization()
	})

	It("should merge non-serializing fields", func() {
		s := &TestType2{
			v2:  1,
			Ptr: &TestType1{v1: 2, NonSerializing: 3},
		}

		manager.StartSerialization()
		_, err := manager.Serialize(s)
		Expect(err).To(BeNil())
		manager.FinalizeSerialization(buffer)

		fmt.Println(buffer.String())

		value := &TestType2{
			Ptr: &TestType1{
				NonSerializing: 3,
			},
		}

		manager.StartDeserialization(buffer)
		manager.RegisterDeserializationStartingPoint(value)
		result, err := manager.Deserialize(serialization.IDToDeserialize("2"))
		Expect(err).To(BeNil())
		Expect(result).To(BeAssignableToTypeOf(&TestType2{}))
		Expect(result.(*TestType2).v2).To(Equal(1))
		Expect(result.(*TestType2).Ptr.v1).To(Equal(2))
		Expect(result.(*TestType2).Ptr.NonSerializing).To(Equal(3))
		manager.FinalizeDeserialization()
	})

	// It("should serialize slices", func() {
	// 	s := &TestType3{
	// 		Value: 1,
	// 		Data:  []byte{1, 2, 3},
	// 		deps: []*TestType1{
	// 			{v1: 2, NonSerializing: 3},
	// 			{v1: 3, NonSerializing: 4},
	// 		},
	// 	}
	// 	err := manager.Serialize(s)
	// 	Expect(err).To(BeNil())

	// 	fmt.Println(buffer.String())

	// 	value := &TestType3{
	// 		deps: []*TestType1{
	// 			{NonSerializing: 3},
	// 		},
	// 	}
	// 	err = manager.Deserialize(value)
	// 	Expect(err).To(BeNil())
	// 	Expect(value.Value).To(Equal(1))
	// 	Expect(value.Data).To(Equal([]byte{1, 2, 3}))
	// 	Expect(value.deps).To(HaveLen(2))
	// 	Expect(value.deps[0].v1).To(Equal(2))
	// 	Expect(value.deps[0].NonSerializing).To(Equal(3))
	// 	Expect(value.deps[1].v1).To(Equal(3))
	// 	Expect(value.deps[1].NonSerializing).To(Equal(0))
	// })
})
