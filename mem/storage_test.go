package mem

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Storage", func() {
	It("should read and write in single unit", func() {
		storage := NewStorage(4096)
		storage.Write(0, []byte{1, 2, 3, 4})

		res := storage.Read(0, 2)
		Expect(res).To(Equal([]byte{1, 2}))

		res = storage.Read(1, 2)
		Expect(res).To(Equal([]byte{2, 3}))
	})

	It("should read and write across units", func() {
		storage := NewStorage(8192)
		storage.Write(4094, []byte{1, 2, 3, 4})

		res := storage.Read(4094, 4)
		Expect(res).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should panic before accessing beyond capacity", func() {
		storage := NewStorage(4096)
		Expect(func() { storage.Write(4097, []byte{1}) }).To(Panic())
		Expect(func() { storage.Read(4097, 1) }).To(Panic())
	})
})
