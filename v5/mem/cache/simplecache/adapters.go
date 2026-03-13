package simplecache

import (
	"github.com/sarchlab/akita/v5/sim"
)

// --- stateTransBuffer ---
// Wraps pointers to []int (transaction indices in postCoalesceTransactions).
// Push/Pop/Peek convert between *transactionState and index.
// Used for dirBuf and bankBufs.
//
// A-B pattern: readItems points to cur (A) state (reserved for future use),
// writeItems points to next (B) state and is used by all operations
// (Peek/Pop/Push/CanPush/Size) to ensure intra-tick consistency.

type stateTransBuffer struct {
	sim.HookableBase
	name       string
	readItems  *[]int // points to cur (A) state - reserved
	writeItems *[]int // points to next (B) state - all ops use this
	capacity   int
	mw         *pipelineMW // needed to resolve indices to transaction pointers
}

func (b *stateTransBuffer) Name() string  { return b.name }
func (b *stateTransBuffer) Capacity() int { return b.capacity }
func (b *stateTransBuffer) Size() int     { return len(*b.writeItems) }
func (b *stateTransBuffer) CanPush() bool { return len(*b.writeItems) < b.capacity }
func (b *stateTransBuffer) Clear()        { *b.writeItems = (*b.writeItems)[:0] }

func (b *stateTransBuffer) Push(e any) {
	trans := e.(*transactionState)
	idx := b.findPostCoalesceIdx(trans)
	*b.writeItems = append(*b.writeItems, idx)
}

// Peek reads from writeItems (next/B state) so it sees mutations from Pop
// within the same tick. Skips nil entries (transactions already finalized).
func (b *stateTransBuffer) Peek() any {
	for len(*b.writeItems) > 0 {
		idx := (*b.writeItems)[0]
		trans := b.mw.postCoalesceTransactions[idx]
		if trans != nil {
			return trans
		}
		*b.writeItems = (*b.writeItems)[1:]
	}
	return nil
}

func (b *stateTransBuffer) Pop() any {
	for len(*b.writeItems) > 0 {
		idx := (*b.writeItems)[0]
		*b.writeItems = (*b.writeItems)[1:]
		trans := b.mw.postCoalesceTransactions[idx]
		if trans != nil {
			return trans
		}
	}
	return nil
}

func (b *stateTransBuffer) findPostCoalesceIdx(
	trans *transactionState,
) int {
	for i, t := range b.mw.postCoalesceTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in postCoalesceTransactions")
}

// --- stateDirPostBufAdapter ---
// Wraps read/write pointers to []int (transaction indices). Peek/Pop return dirPipelineItem.
// Used for directory post-pipeline buffer.

type stateDirPostBufAdapter struct {
	sim.HookableBase
	name       string
	readItems  *[]int // points to cur (A) state - reserved
	writeItems *[]int // points to next (B) state - all ops use this
	capacity   int
	mw         *pipelineMW
}

func (b *stateDirPostBufAdapter) Name() string  { return b.name }
func (b *stateDirPostBufAdapter) Capacity() int { return b.capacity }
func (b *stateDirPostBufAdapter) Size() int     { return len(*b.writeItems) }
func (b *stateDirPostBufAdapter) CanPush() bool { return len(*b.writeItems) < b.capacity }
func (b *stateDirPostBufAdapter) Clear()        { *b.writeItems = (*b.writeItems)[:0] }

func (b *stateDirPostBufAdapter) Push(e any) {
	item := e.(dirPipelineItem)
	idx := b.findPostCoalesceIdx(item.trans)
	*b.writeItems = append(*b.writeItems, idx)
}

// Peek reads from writeItems (next/B state) so it sees mutations from Pop
// within the same tick.
func (b *stateDirPostBufAdapter) Peek() any {
	if len(*b.writeItems) == 0 {
		return nil
	}
	idx := (*b.writeItems)[0]
	return dirPipelineItem{trans: b.mw.postCoalesceTransactions[idx]}
}

func (b *stateDirPostBufAdapter) Pop() any {
	if len(*b.writeItems) == 0 {
		return nil
	}
	idx := (*b.writeItems)[0]
	*b.writeItems = (*b.writeItems)[1:]
	return dirPipelineItem{trans: b.mw.postCoalesceTransactions[idx]}
}

func (b *stateDirPostBufAdapter) findPostCoalesceIdx(
	trans *transactionState,
) int {
	for i, t := range b.mw.postCoalesceTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in postCoalesceTransactions")
}

// --- stateBankPostBufAdapter ---
// Wraps read/write pointers to []int (transaction indices). Peek/Pop return *bankTransaction.
// Used for bank post-pipeline buffers.

type stateBankPostBufAdapter struct {
	sim.HookableBase
	name       string
	readItems  *[]int // points to cur (A) state - reserved
	writeItems *[]int // points to next (B) state - all ops use this
	capacity   int
	mw         *pipelineMW
}

func (b *stateBankPostBufAdapter) Name() string  { return b.name }
func (b *stateBankPostBufAdapter) Capacity() int { return b.capacity }
func (b *stateBankPostBufAdapter) Size() int     { return len(*b.writeItems) }
func (b *stateBankPostBufAdapter) CanPush() bool { return len(*b.writeItems) < b.capacity }
func (b *stateBankPostBufAdapter) Clear()        { *b.writeItems = (*b.writeItems)[:0] }

func (b *stateBankPostBufAdapter) Push(e any) {
	// Can accept either *bankTransaction or dirPipelineItem-wrapped
	switch v := e.(type) {
	case *bankTransaction:
		idx := b.findPostCoalesceIdx(v.transactionState)
		*b.writeItems = append(*b.writeItems, idx)
	default:
		panic("unexpected type pushed to stateBankPostBufAdapter")
	}
}

// Peek reads from writeItems (next/B state) so it sees mutations from Pop
// within the same tick. Skips nil entries (transactions already finalized).
func (b *stateBankPostBufAdapter) Peek() any {
	if len(*b.writeItems) == 0 {
		return nil
	}
	idx := (*b.writeItems)[0]
	trans := b.mw.postCoalesceTransactions[idx]
	if trans == nil {
		return nil
	}
	return &bankTransaction{transactionState: trans}
}

func (b *stateBankPostBufAdapter) Pop() any {
	if len(*b.writeItems) == 0 {
		return nil
	}
	idx := (*b.writeItems)[0]
	*b.writeItems = (*b.writeItems)[1:]
	trans := b.mw.postCoalesceTransactions[idx]
	if trans == nil {
		return nil
	}
	return &bankTransaction{transactionState: trans}
}

func (b *stateBankPostBufAdapter) findPostCoalesceIdx(
	trans *transactionState,
) int {
	for i, t := range b.mw.postCoalesceTransactions {
		if t != nil && t == trans {
			return i
		}
	}
	panic("transaction not found in postCoalesceTransactions")
}

// --- Pipeline free functions ---
// These operate on State pipeline arrays directly, following the switch pattern.

func dirPipelineCanAccept(
	stages []dirPipelineStageState,
	pipelineWidth int,
) bool {
	for lane := range pipelineWidth {
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
	for lane := range pipelineWidth {
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

type dirAction int

const (
	dirKeep    dirAction = iota
	dirAdvance dirAction = iota
	dirMoveBuf dirAction = iota
)

func dirNextStageOccupied(
	newStages []dirPipelineStageState,
	actions []dirAction,
	lane, nextStage int,
) bool {
	for j := range newStages {
		if actions[j] != dirKeep {
			continue
		}
		if newStages[j].Lane == lane && newStages[j].Stage == nextStage {
			return true
		}
	}
	return false
}

func dirProcessItem(
	newStages []dirPipelineStageState,
	actions []dirAction,
	postBuf *[]int,
	postBufCapacity int,
	i, stageNum, lastStage int,
) bool {
	if newStages[i].CycleLeft > 0 {
		newStages[i].CycleLeft--
		return true
	}

	if stageNum == lastStage {
		if len(*postBuf) < postBufCapacity {
			*postBuf = append(*postBuf, newStages[i].TransIndex)
			actions[i] = dirMoveBuf
			return true
		}
		return false
	}

	nextStage := stageNum + 1
	if !dirNextStageOccupied(newStages, actions, newStages[i].Lane, nextStage) {
		newStages[i].Stage = nextStage
		newStages[i].CycleLeft = 0
		actions[i] = dirAdvance
		return true
	}
	return false
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

	actions := make([]dirAction, len(*stages))
	newStages := make([]dirPipelineStageState, len(*stages))
	copy(newStages, *stages)

	for stageNum := lastStage; stageNum >= 0; stageNum-- {
		for i := range newStages {
			if actions[i] != dirKeep || newStages[i].Stage != stageNum {
				continue
			}
			if dirProcessItem(newStages, actions, postBuf,
				postBufCapacity, i, stageNum, lastStage) {
				madeProgress = true
			}
		}
	}

	remaining := make([]dirPipelineStageState, 0, len(newStages))
	for i, a := range actions {
		if a != dirMoveBuf {
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
	for lane := range pipelineWidth {
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
	for lane := range pipelineWidth {
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

type bankAction int

const (
	bankKeep    bankAction = iota
	bankAdvance bankAction = iota
	bankMoveBuf bankAction = iota
)

func bankNextStageOccupied(
	newStages []bankPipelineStageState,
	actions []bankAction,
	lane, nextStage int,
) bool {
	for j := range newStages {
		if actions[j] != bankKeep {
			continue
		}
		if newStages[j].Lane == lane && newStages[j].Stage == nextStage {
			return true
		}
	}
	return false
}

func bankProcessItem(
	newStages []bankPipelineStageState,
	actions []bankAction,
	postBuf *[]int,
	postBufCapacity int,
	i, stageNum, lastStage int,
) bool {
	if newStages[i].CycleLeft > 0 {
		newStages[i].CycleLeft--
		return true
	}

	if stageNum == lastStage {
		if len(*postBuf) < postBufCapacity {
			*postBuf = append(*postBuf, newStages[i].TransIndex)
			actions[i] = bankMoveBuf
			return true
		}
		return false
	}

	nextStage := stageNum + 1
	if !bankNextStageOccupied(newStages, actions, newStages[i].Lane, nextStage) {
		newStages[i].Stage = nextStage
		newStages[i].CycleLeft = 0
		actions[i] = bankAdvance
		return true
	}
	return false
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

	actions := make([]bankAction, len(*stages))
	newStages := make([]bankPipelineStageState, len(*stages))
	copy(newStages, *stages)

	for stageNum := lastStage; stageNum >= 0; stageNum-- {
		for i := range newStages {
			if actions[i] != bankKeep || newStages[i].Stage != stageNum {
				continue
			}
			if bankProcessItem(newStages, actions, postBuf,
				postBufCapacity, i, stageNum, lastStage) {
				madeProgress = true
			}
		}
	}

	remaining := make([]bankPipelineStageState, 0, len(newStages))
	for i, a := range actions {
		if a != bankMoveBuf {
			remaining = append(remaining, newStages[i])
		}
	}
	*stages = remaining

	return madeProgress
}
