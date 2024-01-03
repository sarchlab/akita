package org

import (
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// A Bank is a DRAM Bank. It contains a number of rows and columns.
type Bank interface {
	tracing.NamedHookable

	GetReadyCommand(
		now sim.VTimeInSec,
		cmd *signal.Command,
	) *signal.Command
	StartCommand(now sim.VTimeInSec, cmd *signal.Command)
	UpdateTiming(cmdKind signal.CommandKind, cycleNeeded int)
	Tick(now sim.VTimeInSec) bool
}
