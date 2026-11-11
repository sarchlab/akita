// Package maintenance orders address-filtered control requests in running caches.
package maintenance

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
)

// State tracks a single command from acceptance to ACK. Lines retain request
// order for deterministic directory lookups; Tags gates later accesses until ACK.
type State struct {
	Command          memcontrolprotocol.Command `json:"command"`
	ID               uint64                     `json:"id"`
	Src              messaging.RemotePort       `json:"src"`
	PID              vm.PID                     `json:"pid"`
	RecvTaskID       uint64                     `json:"recv_task_id"`
	Tags             map[uint64]bool            `json:"tags"`
	Lines            []uint64                   `json:"lines"`
	AccessesDrained  bool                       `json:"accesses_drained"`
	LastIssuedLookup bool                       `json:"last_issued_lookup"`
	LookupsInFlight  int                        `json:"lookups_in_flight"`
	// Deferred holds transaction indices admitted after the command. They
	// go to the directory in admission order after the ACK.
	Deferred []int `json:"deferred"`
}

func (s *State) Start(msg messaging.Msg, log2BlockSize uint64, taskID uint64) {
	req := msg.Payload.(memcontrolprotocol.Req)
	*s = State{Command: req.Command, ID: msg.ID, Src: msg.Src, PID: req.PID,
		RecvTaskID: taskID, Tags: make(map[uint64]bool, len(req.Addresses))}
	for _, addr := range req.Addresses {
		tag := addr >> log2BlockSize << log2BlockSize
		if !s.Tags[tag] {
			s.Tags[tag] = true
			s.Lines = append(s.Lines, tag)
		}
	}
}

func (s *State) Matches(pid vm.PID, addr, log2BlockSize uint64) bool {
	return (s.PID == 0 || s.PID == pid) && s.Tags[addr>>log2BlockSize<<log2BlockSize]
}

func (s *State) Blocks(msg messaging.Msg, log2BlockSize uint64) bool {
	if s.Command == 0 {
		return false
	}
	switch req := msg.Payload.(type) {
	case memprotocol.ReadReq:
		return s.Matches(req.PID, req.Address, log2BlockSize)
	case memprotocol.WriteReq:
		return s.Matches(req.PID, req.Address, log2BlockSize)
	default:
		return false
	}
}

// Admit shares the directory input between demand traffic and maintenance
// lookups. Either side can use the full bandwidth when the other stalls.
func (s *State) Admit(demand, lookup func() bool) bool {
	if !s.AccessesDrained || len(s.Lines) == 0 {
		return demand()
	}
	if s.LastIssuedLookup && demand() {
		s.LastIssuedLookup = false
		return true
	}
	if lookup() {
		s.LastIssuedLookup = true
		return true
	}
	return !s.LastIssuedLookup && demand()
}

func (s *State) Finish() { *s = State{Deferred: s.Deferred} }

// Resume moves one deferred transaction into the normal directory input.
func (s *State) Resume(dir *queueing.Buffer[int]) bool {
	if s.Command != 0 || len(s.Deferred) == 0 || !dir.CanPush() {
		return false
	}
	dir.Push(s.Deferred[0])
	s.Deferred = s.Deferred[1:]
	return true
}

// VisitLine visits matching resident ways in the indexed set. A zero PID
// selects all address spaces, so more than one way can match the same tag.
func (s *State) VisitLine(ds *cache.DirectoryState, blockSize int, addr uint64, visit func(*cache.BlockState)) {
	setID := cache.DirectorySetID(addr, blockSize, len(ds.Sets))
	blocks := ds.Sets[setID].Blocks
	for i := range blocks {
		b := &blocks[i]
		if b.IsValid && b.Tag == addr && (s.PID == 0 || s.PID == vm.PID(b.PID)) {
			visit(b)
		}
	}
}

func Invalidate(b *cache.BlockState) {
	b.IsValid = false
	b.IsDirty = false
	b.DirtyMask = nil
}
