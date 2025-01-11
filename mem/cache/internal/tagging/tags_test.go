package tagging

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tags", func() {
	var (
		mockCtrl *gomock.Controller
		tags     *tagArrayImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		tags = &tagArrayImpl{
			NumSets:   1024,
			NumWays:   4,
			BlockSize: 64,
			Sets:      []Set{},
		}
		tags.Reset()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should be able to get total size", func() {
		Expect(tags.TotalSize()).To(Equal(uint64(262144)))
	})

	It("should lookup", func() {
		block := Block{
			PID:     1,
			Tag:     0x100,
			IsValid: true,
		}
		set, _ := tags.GetSet(0x100)
		set.Blocks[0] = block

		block, ok := tags.Lookup(1, 0x100)
		Expect(ok).To(BeTrue())
		Expect(block).To(Equal(block))
	})

	It("should return nil when lookup cannot find block", func() {
		block, ok := tags.Lookup(1, 0x100)
		Expect(ok).To(BeFalse())
		Expect(block).To(BeZero())
	})

	It("should return nil if block is invalid", func() {
		block := Block{
			PID:     1,
			Tag:     0x100,
			IsValid: false,
		}
		set, _ := tags.GetSet(0x100)
		set.Blocks[0] = block

		block, ok := tags.Lookup(1, 0x100)
		Expect(ok).To(BeFalse())
		Expect(block).To(BeZero())
	})

	It("should return nil if PID does not match", func() {
		block := Block{
			PID:     2,
			Tag:     0x100,
			IsValid: true,
		}
		set, _ := tags.GetSet(0x100)
		set.Blocks[0] = block

		block, ok := tags.Lookup(1, 0x100)
		Expect(ok).To(BeFalse())
		Expect(block).To(BeZero())
	})

	It("should update LRU queue when visiting a block", func() {
		set, _ := tags.GetSet(0x100)

		tags.Visit(set.Blocks[1])

		Expect(set.LRUQueue).To(Equal([]int{0, 2, 3, 1}))
	})
})
