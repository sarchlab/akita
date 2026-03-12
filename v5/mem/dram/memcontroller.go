package dram

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
)

func (p Protocol) isGDDR() bool {
	return p == GDDR5 || p == GDDR5X || p == GDDR6
}

func (p Protocol) isHBM() bool {
	return p == HBM || p == HBM2
}

// Spec contains immutable configuration for the DRAM memory controller.
type Spec struct {
	// Protocol
	Protocol int `json:"protocol"`

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

	// CmdCycles: cycles per command kind
	CmdCycles map[CommandKind]int `json:"cmd_cycles"`

	// The entire Timing structure (computed once in builder)
	Timing Timing `json:"timing"`
}

// State contains mutable runtime data for the DRAM memory controller.
type State struct {
	Transactions  []transactionState `json:"transactions"`
	SubTransQueue subTransQueueState `json:"sub_trans_queue"`
	CommandQueues commandQueueState  `json:"command_queues"`
	BankStates    bankStatesFlat     `json:"bank_states"`
}
