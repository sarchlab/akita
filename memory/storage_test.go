package memory_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/syifan/yaotsu/memory"
)

var _ = Describe("Storage", func() {
	It("should read and write in single unit", func() {
		storage := memory.NewStorage(4096)
		storage.Write(0, []byte{1, 2, 3, 4})

		res, _ := storage.Read(0, 2)
		Expect(res).To(Equal([]byte{1, 2}))

		res, _ = storage.Read(1, 2)
		Expect(res).To(Equal([]byte{2, 3}))
	})

	It("should read and write across units", func() {
		storage := memory.NewStorage(8192)
		storage.Write(4094, []byte{1, 2, 3, 4})

		res, _ := storage.Read(4094, 4)
		Expect(res).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should return error if accessing over the capacity", func() {
		storage := memory.NewStorage(4096)
		err := storage.Write(4097, []byte{1})
		Expect(err).To(MatchError("Accessing physical address beyond the storage capacity"))

		_, err = storage.Read(4097, 1)
		Expect(err).To(MatchError("Accessing physical address beyond the storage capacity"))
	})

})
