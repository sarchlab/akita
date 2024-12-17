package mshr_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/mshr"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

var _ = Describe("MSHRImpl", func() {
	var (
		m mshr.MSHR
	)

	BeforeEach(func() {
		m = mshr.MSHR{
			Capacity: 4,
		}
	})

	It("should add an entry", func() {
		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x00,
		})

		found := m.Lookup(1, 0x00)
		Expect(found).To(BeTrue())

		m.RemoveEntry(1, 0x00)
		found = m.Lookup(1, 0x00)
		Expect(found).To(BeFalse())
	})

	It("should error if adding an address that is already in MSHR", func() {
		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x00,
		})

		Expect(m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x00,
		})).To(MatchError("trying to add an address that is already in MSHR"))
	})

	It("should error if adding to a full MSHR", func() {
		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x00,
		})
		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x40,
		})
		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x80,
		})

		Expect(m.IsFull()).To(BeFalse())

		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0xc0,
		})

		Expect(m.IsFull()).To(BeTrue())
		Expect(m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x100,
		})).To(MatchError("trying to add to a full MSHR"))
	})

	It("should add/remove a request to an entry", func() {
		m.AddEntry(mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				ID: "2",
			},
			PID:     1,
			Address: 0x00,
		})

		// Add a request to an entry
		err := m.AddReqToEntry(mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				ID: "1",
			},
			PID:     1,
			Address: 0x00,
		})
		Expect(err).To(BeNil())

		// Get the next request in the entry
		req, err := m.GetNextReqInEntry(1, 0x00)
		Expect(err).To(BeNil())
		Expect(req.Meta().ID).To(Equal("1"))

		// Remove the request from the entry
		err = m.RemoveReqFromEntry("1")
		Expect(err).To(BeNil())

		// Get the next request in the entry
		req, err = m.GetNextReqInEntry(1, 0x00)
		Expect(err).To(MatchError("no request found for pid 1 and addr 0x0"))
	})

	It("should reset the mshr", func() {
		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x00,
		})

		m.Reset()
		Expect(m.Lookup(1, 0x00)).To(BeFalse())
	})

	It("should error if removing an non-exist entry", func() {
		Expect(m.RemoveEntry(1, 0x00)).
			To(MatchError("trying to remove an non-exist entry"))
	})

	It("should error if adding a request to an non-exist entry", func() {
		Expect(m.AddReqToEntry(mem.ReadReq{
			PID:     1,
			Address: 0x00,
		})).To(MatchError("trying to add a request to an non-exist entry"))
	})

	It("should error if removing a request that is not in the entry", func() {
		Expect(m.RemoveReqFromEntry("1")).To(MatchError("request 1 not found"))
	})

	It("should error if getting the next request in an empty entry", func() {
		m.AddEntry(mem.ReadReq{
			PID:     1,
			Address: 0x00,
		})

		_, err := m.GetNextReqInEntry(1, 0x00)

		Expect(err).To(MatchError("no request found for pid 1 and addr 0x0"))
	})

	It("should error if getting the next request in an non-exist entry",
		func() {
			_, err := m.GetNextReqInEntry(1, 0x00)

			Expect(err).To(MatchError("no entry found for pid 1 and addr 0x0"))
		})

})
