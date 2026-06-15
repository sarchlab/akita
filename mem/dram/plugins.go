package dram

import (
	"fmt"

	"github.com/sarchlab/akita/v5/timing"
)

// This file holds the controller's swappable *strategies* — the scheduler, row
// policy, and address mapper. They are internal, config-selectable algorithm
// choices (chosen by Spec fields, instantiated from a registry), not Akita
// middleware or hooks: a strategy is used *inside* the bank-tick middleware to
// produce a value, rather than running each cycle or observing.
//
// The two genuinely reactive/observing concerns are expressed with Akita's own
// mechanisms instead: refresh is a Middleware (refreshmw.go) and command
// observation is a hook (hook.go). New schedulers/mappers are added in-tree and
// selected by name — the same model DRAMSim3 and Ramulator2 use.

// scheduler chooses the next command to put on the command bus from the per-rank
// command queues. Pick resolves the command's concrete kind, removes it from its
// queue, and returns it, or returns nil if nothing is ready.
type scheduler interface {
	Name() string
	Pick(spec *Spec, st *State, t *dramTiming) *commandState
}

// rowPolicy turns a queued sub-transaction into a column command, deciding the
// open- vs close-page variant. The location is resolved by the addrMapper.
type rowPolicy interface {
	Name() string
	CommandFor(spec *Spec, st *State, ref subTransRef, loc location) *commandState
}

// addrMapper maps a physical address to a DRAM location. The location keeps a
// Channel field so the interface stays stable if first-class channels are added
// later; today the controller models one channel per component.
type addrMapper interface {
	Name() string
	Map(spec *Spec, addr uint64) location
}

// --- Default strategy implementations ------------------------------------

const schedulerFRFCFS = "FRFCFS"

// frfcfsScheduler is the default First-Ready, First-Come-First-Served scheduler
// with row-buffer-hit priority and optional write-drain (see getCommandToIssue).
type frfcfsScheduler struct{}

func (frfcfsScheduler) Name() string { return schedulerFRFCFS }

func (frfcfsScheduler) Pick(spec *Spec, st *State, _ *dramTiming) *commandState {
	return getCommandToIssue(spec, st)
}

const (
	rowPolicyOpen  = "open"
	rowPolicyClose = "close"
)

// openPageRowPolicy issues plain Read/Write commands, leaving the row open.
type openPageRowPolicy struct{}

func (openPageRowPolicy) Name() string { return rowPolicyOpen }

func (openPageRowPolicy) CommandFor(
	_ *Spec, st *State, ref subTransRef, loc location,
) *commandState {
	return buildColumnCommand(st, ref, loc, cmdKindRead, cmdKindWrite)
}

// closePageRowPolicy issues ReadPrecharge/WritePrecharge (auto-precharge),
// closing the row after each access.
type closePageRowPolicy struct{}

func (closePageRowPolicy) Name() string { return rowPolicyClose }

func (closePageRowPolicy) CommandFor(
	_ *Spec, st *State, ref subTransRef, loc location,
) *commandState {
	return buildColumnCommand(
		st, ref, loc, cmdKindReadPrecharge, cmdKindWritePrecharge)
}

// buildColumnCommand constructs a column command for a sub-transaction at the
// given location, choosing the read or write variant from the parent
// transaction's direction.
func buildColumnCommand(
	st *State, ref subTransRef, loc location,
	readKind, writeKind commandKind,
) *commandState {
	trans := findTransaction(st, ref.TxID)
	sub := &trans.SubTransactions[ref.SubIndex]

	cmd := &commandState{
		ID:          timing.GetIDGenerator().Generate(),
		Address:     sub.Address,
		SubTransRef: ref,
		Location:    loc,
	}
	if isTransactionRead(trans) {
		cmd.Kind = int(readKind)
	} else {
		cmd.Kind = int(writeKind)
	}
	return cmd
}

const addrMapperDefault = "default"

// fixedAddrMapper applies the single fixed bit-decode scheme configured on Spec.
type fixedAddrMapper struct{}

func (fixedAddrMapper) Name() string { return addrMapperDefault }

func (fixedAddrMapper) Map(spec *Spec, addr uint64) location {
	return mapAddress(spec, addr)
}

// --- Registries ----------------------------------------------------------

var schedulerRegistry = map[string]func() scheduler{
	schedulerFRFCFS: func() scheduler { return frfcfsScheduler{} },
}

var addrMapperRegistry = map[string]func() addrMapper{
	addrMapperDefault: func() addrMapper { return fixedAddrMapper{} },
}

func newScheduler(name string) scheduler {
	if name == "" {
		name = schedulerFRFCFS
	}
	factory, ok := schedulerRegistry[name]
	if !ok {
		panic(fmt.Sprintf("dram: unknown scheduler %q", name))
	}
	return factory()
}

func newAddrMapper(name string) addrMapper {
	if name == "" {
		name = addrMapperDefault
	}
	factory, ok := addrMapperRegistry[name]
	if !ok {
		panic(fmt.Sprintf("dram: unknown address mapper %q", name))
	}
	return factory()
}

// --- Controller ----------------------------------------------------------

// controller bundles the swappable strategies selected for one DRAM component.
// It is behavior/configuration, not serialized runtime state; the bank-tick
// middleware drives the model through it.
type controller struct {
	scheduler  scheduler
	rowPolicy  rowPolicy
	addrMapper addrMapper
}

// fillCommandQueue moves at most one ready sub-transaction from the
// sub-transaction queue into a command queue: it maps the address and turns the
// sub-transaction into a column command via the configured strategies. Returns
// true if a sub-transaction was enqueued.
func (c *controller) fillCommandQueue(spec *Spec, state *State) bool {
	for i, ref := range state.SubTransQueue.Entries {
		sub := subTransByRef(state, ref)
		if sub == nil {
			continue
		}

		loc := c.addrMapper.Map(spec, sub.Address)
		cmd := c.rowPolicy.CommandFor(spec, state, ref, loc)

		if canAcceptCommand(state, cmd, spec) {
			acceptCommand(state, cmd, spec)
			state.SubTransQueue.Entries = append(
				state.SubTransQueue.Entries[:i],
				state.SubTransQueue.Entries[i+1:]...,
			)
			return true
		}
	}

	return false
}

// subTransByRef resolves a sub-transaction reference to its current state, or
// nil if the parent transaction is no longer present.
func subTransByRef(state *State, ref subTransRef) *subTransState {
	trans := findTransaction(state, ref.TxID)
	if trans == nil ||
		ref.SubIndex < 0 || ref.SubIndex >= len(trans.SubTransactions) {
		return nil
	}
	return &trans.SubTransactions[ref.SubIndex]
}
