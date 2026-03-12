package dram

import (
	"github.com/sarchlab/akita/v5/modeling"
)

type bankTickMW struct {
	comp *modeling.Component[Spec, State]
}

// Tick runs tickBanks, issue, and tickSubTransQueue.
func (m *bankTickMW) Tick() bool {
	next := m.comp.GetNextState()
	spec := m.comp.GetSpec()

	progress := tickBanks(&spec, next)
	progress = m.issue(&spec, next) || progress
	progress = tickSubTransQueue(&spec, next) || progress

	return progress
}

func (m *bankTickMW) issue(spec *Spec, next *State) bool {
	cmd := getCommandToIssue(spec, next)
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
