package dram

import "github.com/sarchlab/akita/v5/hooking"

// HookPosCmdIssued fires when the controller issues a DRAM command. Observers
// (command counters, energy/thermal models, command tracing) attach via the
// component's AcceptHook and read the CommandEvent payload. This is the Akita
// convention for letting an external observer watch a component's internal
// activity (cf. queueing.HookPosBufPush) — it replaces the bespoke observer
// interface the controller used to carry.
var HookPosCmdIssued = &hooking.HookPos{Name: "DRAM Command Issued"}

// CommandEvent is the HookCtx.Item payload carried by HookPosCmdIssued. All
// fields are exported so a hook in any package can consume it.
type CommandEvent struct {
	Kind      string // command mnemonic: RD, RDA, WR, WRA, ACT, PRE, REF, ...
	Rank      uint64
	BankGroup uint64
	Bank      uint64
	Row       uint64
	Column    uint64
	Tick      uint64
}

// String returns the JEDEC-style mnemonic for a command kind.
func (k commandKind) String() string {
	switch k {
	case cmdKindRead:
		return "RD"
	case cmdKindReadPrecharge:
		return "RDA"
	case cmdKindWrite:
		return "WR"
	case cmdKindWritePrecharge:
		return "WRA"
	case cmdKindActivate:
		return "ACT"
	case cmdKindPrecharge:
		return "PRE"
	case cmdKindRefreshBank:
		return "REFb"
	case cmdKindRefresh:
		return "REF"
	case cmdKindSRefEnter:
		return "SREFE"
	case cmdKindSRefExit:
		return "SREFX"
	default:
		return "UNKNOWN"
	}
}

// commandEventFor builds the hook payload for an issued command.
func commandEventFor(cmd *commandState, tick uint64) CommandEvent {
	return CommandEvent{
		Kind:      commandKind(cmd.Kind).String(),
		Rank:      cmd.Location.Rank,
		BankGroup: cmd.Location.BankGroup,
		Bank:      cmd.Location.Bank,
		Row:       cmd.Location.Row,
		Column:    cmd.Location.Column,
		Tick:      tick,
	}
}
