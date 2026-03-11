package trans

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
)

var _ = Describe("Default SubTransSplitter", func() {

	It("should split", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 1020
		read.AccessByteSize = 128
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		transaction := &signal.Transaction{
			Read: read,
		}

		splitter := NewSubTransSplitter(6)

		splitter.Split(transaction)

		Expect(transaction.SubTransactions).To(HaveLen(3))
	})
})
