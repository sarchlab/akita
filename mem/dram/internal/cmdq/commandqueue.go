// Package cmdq provides command queue implementations
package cmdq

import (
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
)

// A CommandQueue is a queue of command that needs to be executed by a rank or
// a bank.
type CommandQueue interface {
	GetCommandToIssue() *signal.Command
	CanAccept(command *signal.Command) bool
	Accept(command *signal.Command)
}
