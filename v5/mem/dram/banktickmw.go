package dram

import (
	"github.com/sarchlab/akita/v5/modeling"
)

type bankTickMW struct {
	comp      *modeling.Component[Spec, State]
	Timing    Timing
	CmdCycles map[CommandKind]int
}

// Tick runs tickBanks, issue, and tickSubTransQueue.
func (m *bankTickMW) Tick() bool {
	next := m.comp.GetNextState()
	spec := m.comp.GetSpec()

	progress := tickBanks(&spec, m.CmdCycles, next)
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

	startCommand(m.CmdCycles, next, bs, cmd)
	updateTiming(m.Timing, next, cmd)

	return true
}
