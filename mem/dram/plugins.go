package dram

import (
	"fmt"

	"github.com/sarchlab/akita/v5/timing"
)

// This file introduces the pluggable controller architecture (roadmap Phase 1).
// The baked-in scheduler, row policy, refresh handling, and address mapping are
// expressed as interfaces with config-selectable default implementations, so
// later phases add a feature by implementing an interface rather than forking
// the controller. P1.1 wires the seams with no behavior change: the default
// plugins delegate to the exact functions the controller has always used.

// Scheduler chooses the next command to put on the command bus from the
// per-rank command queues. Pick resolves the command's concrete kind, removes
// it from its queue, and returns it, or returns nil if nothing is ready.
// Implementations may consult the timing table to make readiness-aware choices.
type Scheduler interface {
	Name() string
	Pick(spec *Spec, st *State, t *dramTiming) *commandState
}

// RowPolicy turns a queued sub-transaction into a column command, deciding the
// open- vs close-page variant (Read vs ReadPrecharge, Write vs WritePrecharge).
// The physical location has already been resolved by the AddrMapper.
type RowPolicy interface {
	Name() string
	CommandFor(spec *Spec, st *State, ref subTransRef, loc location) *commandState
}

// RefreshManager decides when refresh activity occurs. Tick advances the
// manager by one cycle and reports whether refresh is currently active (which
// stalls command issue for that cycle).
type RefreshManager interface {
	Name() string
	Tick(spec *Spec, st *State) bool
}

// AddrMapper maps a physical address to a DRAM location. The location keeps a
// Channel field so the interface stays stable if first-class channels are
// added later; today the controller models one channel per component.
type AddrMapper interface {
	Name() string
	Map(spec *Spec, addr uint64) location
}

// CommandHook observes every issued command. It is the extension point for
// counters, tracing, energy, and (later) RowHammer mitigations. Hooks observe;
// they do not steer scheduling and must not change results.
type CommandHook interface {
	Name() string
	OnIssue(spec *Spec, st *State, cmd *commandState, now uint64)
}

// --- Default plugin implementations --------------------------------------

const schedulerFRFCFS = "FRFCFS"

// frfcfsScheduler is the default First-Ready, First-Come-First-Served scheduler
// with row-buffer-hit priority and optional write-drain — the behavior the
// controller has always had (see getCommandToIssue).
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

const refreshFakeStall = "fakestall"

// fakeStallRefreshManager models refresh as a global tRFC stall every tREFI,
// without issuing real refresh commands or closing rows (the P0 behavior,
// deviation D2). Real refresh commands arrive in a later phase.
type fakeStallRefreshManager struct{}

func (fakeStallRefreshManager) Name() string { return refreshFakeStall }

func (fakeStallRefreshManager) Tick(spec *Spec, st *State) bool {
	return runFakeStallRefresh(spec, st)
}

const addrMapperDefault = "default"

// fixedAddrMapper applies the single fixed bit-decode scheme configured on Spec.
type fixedAddrMapper struct{}

func (fixedAddrMapper) Name() string { return addrMapperDefault }

func (fixedAddrMapper) Map(spec *Spec, addr uint64) location {
	return mapAddress(spec, addr)
}

const commandHookNull = "null"

// nullCommandHook is a no-op hook. It exists to exercise the hook path without
// affecting results (and as a base for real hooks).
type nullCommandHook struct{}

func (nullCommandHook) Name() string { return commandHookNull }

func (nullCommandHook) OnIssue(_ *Spec, _ *State, _ *commandState, _ uint64) {}

// --- Registries ----------------------------------------------------------

var schedulerRegistry = map[string]func() Scheduler{
	schedulerFRFCFS: func() Scheduler { return frfcfsScheduler{} },
}

var refreshRegistry = map[string]func() RefreshManager{
	refreshFakeStall: func() RefreshManager { return fakeStallRefreshManager{} },
}

var addrMapperRegistry = map[string]func() AddrMapper{
	addrMapperDefault: func() AddrMapper { return fixedAddrMapper{} },
}

func newScheduler(name string) Scheduler {
	if name == "" {
		name = schedulerFRFCFS
	}
	factory, ok := schedulerRegistry[name]
	if !ok {
		panic(fmt.Sprintf("dram: unknown scheduler %q", name))
	}
	return factory()
}

func newRefreshManager(name string) RefreshManager {
	if name == "" {
		name = refreshFakeStall
	}
	factory, ok := refreshRegistry[name]
	if !ok {
		panic(fmt.Sprintf("dram: unknown refresh manager %q", name))
	}
	return factory()
}

func newAddrMapper(name string) AddrMapper {
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

// controller is the set of pluggable behaviors selected for one DRAM
// component. It is behavior/configuration, not serialized runtime state; the
// bank-tick middleware drives the model through it.
type controller struct {
	scheduler  Scheduler
	rowPolicy  RowPolicy
	refresh    RefreshManager
	addrMapper AddrMapper
	hooks      []CommandHook
}

// fillCommandQueue moves at most one ready sub-transaction from the
// sub-transaction queue into a command queue: it maps the address and turns the
// sub-transaction into a column command via the configured plugins. Returns
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
			acceptCommand(state, cmd)
			state.SubTransQueue.Entries = append(
				state.SubTransQueue.Entries[:i],
				state.SubTransQueue.Entries[i+1:]...,
			)
			return true
		}
	}

	return false
}

// onIssue notifies every command hook that a command has been issued.
func (c *controller) onIssue(
	spec *Spec, st *State, cmd *commandState, now uint64,
) {
	for _, h := range c.hooks {
		h.OnIssue(spec, st, cmd, now)
	}
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
