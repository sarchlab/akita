package trans

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
)

var _ = Describe("Default SubTransSplitter", func() {

	It("should split", func() {
		read := mem.ReadReq{
			Address:        1020,
			AccessByteSize: 128,
		}
		transaction := &signal.Transaction{
			Type: signal.TransactionTypeRead,
			Read: read,
		}

		splitter := NewSubTransSplitter(6)

		splitter.Split(transaction)

		Expect(transaction.SubTransactions).To(HaveLen(3))
	})
})
