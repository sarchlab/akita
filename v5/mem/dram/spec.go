package dram

import "github.com/sarchlab/akita/v5/sim"

// Protocol defines the category of the memory controller.
type Protocol int

// A list of all supported DRAM protocols.
const (
	DDR3 Protocol = iota
	DDR4
	GDDR5
	GDDR5X
	GDDR6
	LPDDR
	LPDDR3
	LPDDR4
	HBM
	HBM2
	HMC
	DDR5
	HBM3
	LPDDR5
	HBM3E
)

func (p Protocol) isGDDR() bool {
	return p == GDDR5 || p == GDDR5X || p == GDDR6
}

func (p Protocol) isHBM() bool {
	return p == HBM || p == HBM2 || p == HBM3 || p == HBM3E
}

// PagePolicy defines the page management policy for the DRAM controller.
type PagePolicy int

// A list of supported page policies.
const (
	PagePolicyClose PagePolicy = 0
	PagePolicyOpen  PagePolicy = 1
)

// Spec contains immutable configuration for the DRAM memory controller.
type Spec struct {
	// Frequency
	Freq sim.Freq `json:"freq"`

	// Protocol
	Protocol int `json:"protocol"`

	// Page policy
	PagePolicy PagePolicy `json:"page_policy"`

	// Timing params
	TAL        int `json:"t_al"`
	TCL        int `json:"t_cl"`
	TCWL       int `json:"t_cwl"`
	TRL        int `json:"t_rl"`
	TWL        int `json:"t_wl"`
	ReadDelay  int `json:"read_delay"`
	WriteDelay int `json:"write_delay"`
	TRCD       int `json:"t_rcd"`
	TRP        int `json:"t_rp"`
	TRAS       int `json:"t_ras"`
	TCCDS      int `json:"t_ccds"`
	TCCDL      int `json:"t_ccdl"`
	TRTRS      int `json:"t_rtrs"`
	TRTP       int `json:"t_rtp"`
	TWTRL      int `json:"t_wtrl"`
	TWTRS      int `json:"t_wtrs"`
	TWR        int `json:"t_wr"`
	TPPD       int `json:"t_ppd"`
	TRC        int `json:"t_rc"`
	TRRDS      int `json:"t_rrds"`
	TRRDL      int `json:"t_rrdl"`
	TRCDRD     int `json:"t_rcdrd"`
	TRCDWR     int `json:"t_rcdwr"`
	TREFI      int `json:"t_refi"`
	TRFC       int `json:"t_rfc"`
	TRFCb      int `json:"t_rfcb"`
	TCKESR     int `json:"t_ckesr"`
	TXS        int `json:"t_xs"`
	BurstCycle int `json:"burst_cycle"`

	// Bus / burst / device params
	BusWidth    int `json:"bus_width"`
	BurstLength int `json:"burst_length"`
	DeviceWidth int `json:"device_width"`

	// Bank / rank / channel counts
	NumChannel   int `json:"num_channel"`
	NumRank      int `json:"num_rank"`
	NumBankGroup int `json:"num_bank_group"`
	NumBank      int `json:"num_bank"`
	NumRow       int `json:"num_row"`
	NumCol       int `json:"num_col"`

	// Queue sizes
	TransactionQueueSize int `json:"transaction_queue_size"`
	CommandQueueCapacity int `json:"command_queue_capacity"`

	// Read/Write queue separation
	ReadQueueSize      int `json:"read_queue_size"`
	WriteQueueSize     int `json:"write_queue_size"`
	WriteHighWatermark int `json:"write_high_watermark"`
	WriteLowWatermark  int `json:"write_low_watermark"`

	// Address converter params
	HasAddrConverter    bool   `json:"has_addr_converter"`
	InterleavingSize    uint64 `json:"interleaving_size"`
	TotalNumOfElements  int    `json:"total_num_of_elements"`
	CurrentElementIndex int    `json:"current_element_index"`
	Offset              uint64 `json:"offset"`

	// Address mapping: position/mask pairs
	ChannelPos    int    `json:"channel_pos"`
	ChannelMask   uint64 `json:"channel_mask"`
	RankPos       int    `json:"rank_pos"`
	RankMask      uint64 `json:"rank_mask"`
	BankGroupPos  int    `json:"bank_group_pos"`
	BankGroupMask uint64 `json:"bank_group_mask"`
	BankPos       int    `json:"bank_pos"`
	BankMask      uint64 `json:"bank_mask"`
	RowPos        int    `json:"row_pos"`
	RowMask       uint64 `json:"row_mask"`
	ColPos        int    `json:"col_pos"`
	ColMask       uint64 `json:"col_mask"`

	// Sub-transaction splitting
	Log2AccessUnitSize uint64 `json:"log2_access_unit_size"`
}

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

