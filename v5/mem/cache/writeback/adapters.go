package writeback

import (
	"github.com/sarchlab/akita/v5/sim"
)

// --- stateTransBuffer ---
// Wraps a *[]int (transaction indices in inFlightTransactions).
// Push/Pop/Peek convert between *transactionState and index.
// Used for dirStageBuffer, dirToBankBuffers, writeBufferToBankBuffers,
// mshrStageBuffer, writeBufferBuffer.

type stateTransBuffer struct {
	sim.HookableBase
	name     string
	items    *[]int
	capacity int
	mw       *middleware // needed to resolve indices to transaction pointers
}

func (b *stateTransBuffer) Name() string  { return b.name }
func (b *stateTransBuffer) Capacity() int { return b.capacity }
func (b *stateTransBuffer) Size() int     { return len(*b.items) }
func (b *stateTransBuffer) CanPush() bool { return len(*b.items) < b.capacity }
func (b *stateTransBuffer) Clear()        { *b.items = (*b.items)[:0] }

func (b *stateTransBuffer) Push(e interface{}) {
	trans := e.(*transactionState)
	idx := b.findTransIdx(trans)
	*b.items = append(*b.items, idx)
}

func (b *stateTransBuffer) Peek() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	idx := (*b.items)[0]
	return b.mw.inFlightTransactions[idx]
}

func (b *stateTransBuffer) Pop() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	idx := (*b.items)[0]
	*b.items = (*b.items)[1:]
	return b.mw.inFlightTransactions[idx]
}

func (b *stateTransBuffer) findTransIdx(
	trans *transactionState,
) int {
	for i, t := range b.mw.inFlightTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in inFlightTransactions")
}

// --- stateDirPostBufAdapter ---
// Wraps a *[]int (transaction indices). Peek/Pop return dirPipelineItem.
// Used for directory post-pipeline buffer.

type stateDirPostBufAdapter struct {
	sim.HookableBase
	name     string
	items    *[]int
	capacity int
	mw       *middleware
}

func (b *stateDirPostBufAdapter) Name() string  { return b.name }
func (b *stateDirPostBufAdapter) Capacity() int { return b.capacity }
func (b *stateDirPostBufAdapter) Size() int     { return len(*b.items) }
func (b *stateDirPostBufAdapter) CanPush() bool { return len(*b.items) < b.capacity }
func (b *stateDirPostBufAdapter) Clear()        { *b.items = (*b.items)[:0] }

func (b *stateDirPostBufAdapter) Push(e interface{}) {
	item := e.(dirPipelineItem)
	idx := b.findTransIdx(item.trans)
	*b.items = append(*b.items, idx)
}

func (b *stateDirPostBufAdapter) Peek() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	idx := (*b.items)[0]
	return dirPipelineItem{trans: b.mw.inFlightTransactions[idx]}
}

func (b *stateDirPostBufAdapter) Pop() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	idx := (*b.items)[0]
	*b.items = (*b.items)[1:]
	return dirPipelineItem{trans: b.mw.inFlightTransactions[idx]}
}

func (b *stateDirPostBufAdapter) findTransIdx(
	trans *transactionState,
) int {
	for i, t := range b.mw.inFlightTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in inFlightTransactions")
}

// --- stateBankPostBufAdapter ---
// Wraps a *[]int (transaction indices). Peek/Pop return bankPipelineElem.
// Used for bank post-pipeline buffers.

type stateBankPostBufAdapter struct {
	sim.HookableBase
	name     string
	items    *[]int
	capacity int
	mw       *middleware
}

func (b *stateBankPostBufAdapter) Name() string  { return b.name }
func (b *stateBankPostBufAdapter) Capacity() int { return b.capacity }
func (b *stateBankPostBufAdapter) Size() int     { return len(*b.items) }
func (b *stateBankPostBufAdapter) CanPush() bool { return len(*b.items) < b.capacity }
func (b *stateBankPostBufAdapter) Clear()        { *b.items = (*b.items)[:0] }

func (b *stateBankPostBufAdapter) Push(e interface{}) {
	switch v := e.(type) {
	case bankPipelineElem:
		idx := b.findTransIdx(v.trans)
		*b.items = append(*b.items, idx)
	default:
		panic("unexpected type pushed to stateBankPostBufAdapter")
	}
}

func (b *stateBankPostBufAdapter) Peek() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	idx := (*b.items)[0]
	return bankPipelineElem{
		trans: b.mw.inFlightTransactions[idx],
	}
}

func (b *stateBankPostBufAdapter) Pop() interface{} {
	if len(*b.items) == 0 {
		return nil
	}
	idx := (*b.items)[0]
	*b.items = (*b.items)[1:]
	return bankPipelineElem{
		trans: b.mw.inFlightTransactions[idx],
	}
}

func (b *stateBankPostBufAdapter) findTransIdx(
	trans *transactionState,
) int {
	for i, t := range b.mw.inFlightTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in inFlightTransactions")
}

// --- Pipeline free functions ---
// These operate on State pipeline arrays directly, following the switch pattern.

func dirPipelineCanAccept(
	stages []dirPipelineStageState,
	pipelineWidth int,
) bool {
	for lane := 0; lane < pipelineWidth; lane++ {
		if !dirPipelineSlotOccupied(stages, lane, 0) {
			return true
		}
	}
	return false
}

func dirPipelineSlotOccupied(
	stages []dirPipelineStageState,
	lane, stage int,
) bool {
	for _, s := range stages {
		if s.Lane == lane && s.Stage == stage {
			return true
		}
	}
	return false
}

func dirPipelineAccept(
	stages *[]dirPipelineStageState,
	pipelineWidth int,
	transIdx int,
) {
	for lane := 0; lane < pipelineWidth; lane++ {
		if !dirPipelineSlotOccupied(*stages, lane, 0) {
			*stages = append(*stages, dirPipelineStageState{
				Lane:       lane,
				Stage:      0,
				TransIndex: transIdx,
				CycleLeft:  0,
			})
			return
		}
	}
	panic("dir pipeline is full")
}

func dirPipelineTick(
	stages *[]dirPipelineStageState,
	postBuf *[]int,
	postBufCapacity int,
	numStages int,
) bool {
	if numStages == 0 {
		return false
	}

	madeProgress := false
	lastStage := numStages - 1

	type pipeAction int
	const (
		keep    pipeAction = iota
		advance pipeAction = iota
		moveBuf pipeAction = iota
	)

	actions := make([]pipeAction, len(*stages))
	newStages := make([]dirPipelineStageState, len(*stages))
	copy(newStages, *stages)

	for stageNum := lastStage; stageNum >= 0; stageNum-- {
		for i := range newStages {
			if actions[i] != keep {
				continue
			}
			if newStages[i].Stage != stageNum {
				continue
			}

			if newStages[i].CycleLeft > 0 {
				newStages[i].CycleLeft--
				madeProgress = true
				continue
			}

			if stageNum == lastStage {
				if len(*postBuf) < postBufCapacity {
					*postBuf = append(*postBuf, newStages[i].TransIndex)
					actions[i] = moveBuf
					madeProgress = true
				}
			} else {
				nextStage := stageNum + 1
				occupied := false
				for j := range newStages {
					if actions[j] != keep {
						continue
					}
					if newStages[j].Lane == newStages[i].Lane &&
						newStages[j].Stage == nextStage {
						occupied = true
						break
					}
				}
				if !occupied {
					newStages[i].Stage = nextStage
					newStages[i].CycleLeft = 0
					actions[i] = advance
					madeProgress = true
				}
			}
		}
	}

	remaining := make([]dirPipelineStageState, 0, len(newStages))
	for i, a := range actions {
		if a != moveBuf {
			remaining = append(remaining, newStages[i])
		}
	}
	*stages = remaining

	return madeProgress
}

func bankPipelineCanAccept(
	stages []bankPipelineStageState,
	pipelineWidth int,
) bool {
	for lane := 0; lane < pipelineWidth; lane++ {
		if !bankPipelineSlotOccupied(stages, lane, 0) {
			return true
		}
	}
	return false
}

func bankPipelineSlotOccupied(
	stages []bankPipelineStageState,
	lane, stage int,
) bool {
	for _, s := range stages {
		if s.Lane == lane && s.Stage == stage {
			return true
		}
	}
	return false
}

func bankPipelineAccept(
	stages *[]bankPipelineStageState,
	pipelineWidth int,
	transIdx int,
) {
	for lane := 0; lane < pipelineWidth; lane++ {
		if !bankPipelineSlotOccupied(*stages, lane, 0) {
			*stages = append(*stages, bankPipelineStageState{
				Lane:       lane,
				Stage:      0,
				TransIndex: transIdx,
				CycleLeft:  0,
			})
			return
		}
	}
	panic("bank pipeline is full")
}

func bankPipelineTick(
	stages *[]bankPipelineStageState,
	postBuf *[]int,
	postBufCapacity int,
	numStages int,
) bool {
	if numStages == 0 {
		return false
	}

	madeProgress := false
	lastStage := numStages - 1

	type pipeAction int
	const (
		keep    pipeAction = iota
		advance pipeAction = iota
		moveBuf pipeAction = iota
	)

	actions := make([]pipeAction, len(*stages))
	newStages := make([]bankPipelineStageState, len(*stages))
	copy(newStages, *stages)

	for stageNum := lastStage; stageNum >= 0; stageNum-- {
		for i := range newStages {
			if actions[i] != keep {
				continue
			}
			if newStages[i].Stage != stageNum {
				continue
			}

			if newStages[i].CycleLeft > 0 {
				newStages[i].CycleLeft--
				madeProgress = true
				continue
			}

			if stageNum == lastStage {
				if len(*postBuf) < postBufCapacity {
					*postBuf = append(*postBuf, newStages[i].TransIndex)
					actions[i] = moveBuf
					madeProgress = true
				}
			} else {
				nextStage := stageNum + 1
				occupied := false
				for j := range newStages {
					if actions[j] != keep {
						continue
					}
					if newStages[j].Lane == newStages[i].Lane &&
						newStages[j].Stage == nextStage {
						occupied = true
						break
					}
				}
				if !occupied {
					newStages[i].Stage = nextStage
					newStages[i].CycleLeft = 0
					actions[i] = advance
					madeProgress = true
				}
			}
		}
	}

	remaining := make([]bankPipelineStageState, 0, len(newStages))
	for i, a := range actions {
		if a != moveBuf {
			remaining = append(remaining, newStages[i])
		}
	}
	*stages = remaining

	return madeProgress
}
