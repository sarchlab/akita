package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
)

// Comp models a banked memory with configurable banking and pipeline behavior.
type Comp struct {
	*modeling.Component[Spec, State]

	storage *mem.Storage
}

// GetStorage returns the underlying storage.
func (c *Comp) GetStorage() *mem.Storage {
	return c.storage
}

// StorageName returns the name used to identify this component's storage.
func (c *Comp) StorageName() string {
	return c.GetSpec().StorageRef
}

// --- Free functions for pipeline / buffer / bank-selection / address conversion ---

func pipelineCanAccept(bank bankState, spec Spec) bool {
	if spec.BankPipelineDepth == 0 {
		return len(bank.PostPipelineBuf) < spec.PostPipelineBufSize
	}

	return bank.Pipeline.CanAccept()
}

func pipelineAccept(
	bank *bankState,
	spec Spec,
	item bankPipelineItemState,
) {
	if spec.BankPipelineDepth == 0 {
		bank.PostPipelineBuf = append(bank.PostPipelineBuf, item)
		return
	}

	bank.Pipeline.Accept(item)
}

func pipelineTick(bank *bankState, spec Spec) bool {
	return bank.Pipeline.TickFunc(func(item bankPipelineItemState) bool {
		if len(bank.PostPipelineBuf) < spec.PostPipelineBufSize {
			bank.PostPipelineBuf = append(bank.PostPipelineBuf, item)
			return true
		}

		return false
	})
}

func bufferPeek(bank bankState) (bankPipelineItemState, bool) {
	if len(bank.PostPipelineBuf) == 0 {
		return bankPipelineItemState{}, false
	}

	return bank.PostPipelineBuf[0], true
}

func bufferPop(bank *bankState) {
	if len(bank.PostPipelineBuf) == 0 {
		return
	}

	bank.PostPipelineBuf = bank.PostPipelineBuf[1:]
}

func selectBank(spec Spec, addr uint64) int {
	interleaveSize := uint64(1) << spec.BankSelectorLog2InterleaveSize
	if interleaveSize == 0 {
		panic("simplebankedmemory: invalid interleave size")
	}

	return int((addr / interleaveSize) % uint64(spec.NumBanks))
}
