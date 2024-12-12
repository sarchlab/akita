package org

import (
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/naming"
)

// A Bank is a DRAM Bank. It contains a number of rows and columns.
type Bank interface {
	naming.Named
	hooking.Hookable

	GetReadyCommand(
		cmd *signal.Command,
	) *signal.Command
	StartCommand(cmd *signal.Command)
	UpdateTiming(cmdKind signal.CommandKind, cycleNeeded int)
	Tick() bool
}
