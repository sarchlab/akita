package org

import (
	"github.com/sarchlab/akita/v5/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v5/tracing"
)

// A Bank is a DRAM Bank. It contains a number of rows and columns.
type Bank interface {
	tracing.NamedHookable

	GetReadyCommand(
		cmd *signal.Command,
	) *signal.Command
	StartCommand(cmd *signal.Command)
	UpdateTiming(cmdKind signal.CommandKind, cycleNeeded int)
	Tick() bool
}
