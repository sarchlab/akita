package dram

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

type bankTickMW struct {
	comp *modeling.Component[Spec, State]
}

// Name delegates to the underlying component.
func (m *bankTickMW) Name() string {
	return m.comp.Name()
}

// AcceptHook delegates to the underlying component.
func (m *bankTickMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

// Hooks delegates to the underlying component.
func (m *bankTickMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

// NumHooks delegates to the underlying component.
func (m *bankTickMW) NumHooks() int {
	return m.comp.NumHooks()
}

// InvokeHook delegates to the underlying component.
func (m *bankTickMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

// Tick runs tickBanks, issue, and tickSubTransQueue.
func (m *bankTickMW) Tick() bool {
	curVal := m.comp.GetState()
	cur := &curVal
	next := m.comp.GetNextState()
	spec := m.comp.GetSpec()

	progress := tickBanks(&spec, next)
	progress = m.issue(&spec, cur, next) || progress
	progress = tickSubTransQueue(&spec, next) || progress

	return progress
}

func (m *bankTickMW) issue(spec *Spec, cur *State, next *State) bool {
	cmd := getCommandToIssue(spec, cur, next)
	if cmd == nil {
		return false
	}

	bs := findBankStateByLocation(&next.BankStates, cmd.Location)
	if bs == nil {
		return false
	}

	startCommand(spec, next, bs, cmd)
	updateTiming(spec, next, cmd)

	return true
}
