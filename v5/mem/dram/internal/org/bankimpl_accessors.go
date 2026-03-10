package org

import "github.com/sarchlab/akita/v5/mem/dram/internal/signal"

// GetState returns the current bank state.
func (b *BankImpl) GetState() BankState {
	return b.state
}

// SetState sets the bank state.
func (b *BankImpl) SetState(s BankState) {
	b.state = s
}

// GetOpenRow returns the currently open row.
func (b *BankImpl) GetOpenRow() uint64 {
	return b.openRow
}

// SetOpenRow sets the currently open row.
func (b *BankImpl) SetOpenRow(row uint64) {
	b.openRow = row
}

// GetCurrentCmd returns the current command being processed.
func (b *BankImpl) GetCurrentCmd() *signal.Command {
	return b.currentCmd
}

// SetCurrentCmd sets the current command being processed.
func (b *BankImpl) SetCurrentCmd(cmd *signal.Command) {
	b.currentCmd = cmd
}

// GetCyclesToCmdAvailable returns the cycles-to-command-available map.
func (b *BankImpl) GetCyclesToCmdAvailable() map[signal.CommandKind]int {
	return b.cyclesToCmdAvailable
}

// SetCyclesToCmdAvailable sets the cycles-to-command-available map.
func (b *BankImpl) SetCyclesToCmdAvailable(m map[signal.CommandKind]int) {
	b.cyclesToCmdAvailable = m
}
