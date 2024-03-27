package trans

import (
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
)

// A SubTransactionQueue is a queue for subtransactions.
type SubTransactionQueue interface {
	CanPush(n int) bool
	Push(t *signal.Transaction)
	Tick() bool
}
