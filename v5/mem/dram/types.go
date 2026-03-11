package dram

// CommandKind represents the kind of the command.
type CommandKind int

// A list of supported DRAM command kinds.
const (
	CmdKindRead CommandKind = iota
	CmdKindReadPrecharge
	CmdKindWrite
	CmdKindWritePrecharge
	CmdKindActivate
	CmdKindPrecharge
	CmdKindRefreshBank
	CmdKindRefresh
	CmdKindSRefEnter
	CmdKindSRefExit
	NumCmdKind
)

var cmdKindString = map[CommandKind]string{
	CmdKindRead:           "Read",
	CmdKindReadPrecharge:  "ReadPrecharge",
	CmdKindWrite:          "Write",
	CmdKindWritePrecharge: "WritePrecharge",
	CmdKindActivate:       "Activate",
	CmdKindPrecharge:      "Precharge",
	CmdKindRefreshBank:    "RefreshBank",
	CmdKindRefresh:        "Refresh",
	CmdKindSRefEnter:      "SRefEnter",
	CmdKindSRefExit:       "SRefExit",
}

// String converts the command kind to the string representation.
func (k CommandKind) String() string {
	str, found := cmdKindString[k]
	if found {
		return str
	}
	return "Invalid"
}

// Location determines where to find the data to access.
type Location struct {
	Channel   uint64 `json:"channel"`
	Rank      uint64 `json:"rank"`
	BankGroup uint64 `json:"bank_group"`
	Bank      uint64 `json:"bank"`
	Row       uint64 `json:"row"`
	Column    uint64 `json:"column"`
}

// BankStateKind represents the current state of a bank.
type BankStateKind int

// A list of possible bank states.
const (
	BankStateOpen BankStateKind = iota
	BankStateClosed
	BankStateSRef
	BankStatePD
	BankStateInvalid
)

// TimeTableEntry is an entry in the TimeTable.
type TimeTableEntry struct {
	NextCmdKind       CommandKind
	MinCycleInBetween int
}

// TimeTable is a table that records the minimum number of cycles between any
// two types of DRAM commands.
type TimeTable [][]TimeTableEntry

// MakeTimeTable creates a new TimeTable.
func MakeTimeTable() TimeTable {
	return make([][]TimeTableEntry, NumCmdKind)
}

// Timing records all the timing-related parameters for a DRAM model.
type Timing struct {
	SameBank              TimeTable
	OtherBanksInBankGroup TimeTable
	SameRank              TimeTable
	OtherRanks            TimeTable
}
