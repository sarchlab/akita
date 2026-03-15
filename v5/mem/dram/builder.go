package dram

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// DefaultSpec provides default configuration for the DRAM memory controller.
var DefaultSpec = Spec{
	Freq:                 1600 * sim.MHz,
	Protocol:             int(DDR3),
	TAL:                  0,
	TCL:                  11,
	TCWL:                 8,
	TRCD:                 11,
	TRP:                  11,
	TRAS:                 28,
	TCCDL:                4,
	TCCDS:                4,
	TRTRS:                1,
	TRTP:                 6,
	TWTRL:                6,
	TWTRS:                6,
	TWR:                  12,
	TPPD:                 0,
	TRRDL:                5,
	TRRDS:                5,
	TRCDRD:               24,
	TRCDWR:               20,
	TREFI:                6240,
	TRFC:                 208,
	TRFCb:                1950,
	TCKESR:               5,
	TXS:                  216,
	BusWidth:             64,
	BurstLength:          8,
	DeviceWidth:          16,
	NumChannel:           1,
	NumRank:              2,
	NumBankGroup:         1,
	NumBank:              8,
	NumRow:               32768,
	NumCol:               1024,
	TransactionQueueSize: 32,
	CommandQueueCapacity: 8,
}

// Builder can build new memory controllers.
type Builder struct {
	engine           sim.EventScheduler
	spec             Spec
	useGlobalStorage bool
	storage          *mem.Storage

	hasAddrConverter    bool
	interleavingSize    uint64
	totalNumOfElements  int
	currentElementIndex int
	offset              uint64

	protocol             Protocol
	transactionQueueSize int
	commandQueueSize     int
	busWidth             int
	burstLength          int
	deviceWidth          int
	numChannel           int
	numRank              int
	numBankGroup         int
	numBank              int
	numRow               int
	numCol               int

	burstCycle int
	tAL        int
	tCL        int
	tCWL       int
	tRL        int
	tWL        int
	readDelay  int
	writeDelay int
	tRCD       int
	tRP        int
	tRAS       int
	tCCDL      int
	tCCDS      int
	tRTRS      int
	tRTP       int
	tWTRL      int
	tWTRS      int
	tWR        int
	tPPD       int
	tRC        int
	tRRDL      int
	tRRDS      int
	tRCDRD     int
	tRCDWR     int
	tREFI      int
	tRFC       int
	tRFCb      int
	tCKESR     int
	tXS        int

	tracers []tracing.Tracer
	topPort sim.Port
}

// MakeBuilder creates a builder with default configuration.
func MakeBuilder() Builder {
	b := Builder{
		spec:                 DefaultSpec,
		protocol:             DDR3,
		transactionQueueSize: 32,
		commandQueueSize:     8,
		busWidth:             64,
		burstLength:          8,
		deviceWidth:          16,
		numChannel:           1,
		numRank:              2,
		numBankGroup:         1,
		numBank:              8,
		numRow:               32768,
		numCol:               1024,
		burstCycle:           4,
		tAL:                  0,
		tCL:                  11,
		tCWL:                 8,
		tRCD:                 11,
		tRP:                  11,
		tRAS:                 28,
		tCCDL:                4,
		tCCDS:                4,
		tRTRS:                1,
		tRTP:                 6,
		tWTRL:                6,
		tWTRS:                6,
		tWR:                  12,
		tPPD:                 0,
		tRRDL:                5,
		tRRDS:                5,
		tRCDRD:               24,
		tRCDWR:               20,
		tREFI:                6240,
		tRFC:                 208,
		tRFCb:                1950,
		tCKESR:               5,
		tXS:                  216,
	}

	return b
}

// WithSpec sets the builder's spec and copies all relevant fields from
// the spec into the builder's individual fields.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	b.protocol = Protocol(spec.Protocol)
	b.transactionQueueSize = spec.TransactionQueueSize
	b.commandQueueSize = spec.CommandQueueCapacity
	b.busWidth = spec.BusWidth
	b.burstLength = spec.BurstLength
	b.deviceWidth = spec.DeviceWidth
	b.numChannel = spec.NumChannel
	b.numRank = spec.NumRank
	b.numBankGroup = spec.NumBankGroup
	b.numBank = spec.NumBank
	b.numRow = spec.NumRow
	b.numCol = spec.NumCol
	b.tAL = spec.TAL
	b.tCL = spec.TCL
	b.tCWL = spec.TCWL
	b.tRCD = spec.TRCD
	b.tRP = spec.TRP
	b.tRAS = spec.TRAS
	b.tCCDL = spec.TCCDL
	b.tCCDS = spec.TCCDS
	b.tRTRS = spec.TRTRS
	b.tRTP = spec.TRTP
	b.tWTRL = spec.TWTRL
	b.tWTRS = spec.TWTRS
	b.tWR = spec.TWR
	b.tPPD = spec.TPPD
	b.tRRDL = spec.TRRDL
	b.tRRDS = spec.TRRDS
	b.tRCDRD = spec.TRCDRD
	b.tRCDWR = spec.TRCDWR
	b.tREFI = spec.TREFI
	b.tRFC = spec.TRFC
	b.tRFCb = spec.TRFCb
	b.tCKESR = spec.TCKESR
	b.tXS = spec.TXS

	return b
}

// WithEngine sets the engine that the builder uses.
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency of the builder.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithGlobalStorage asks the DRAM to use a global storage instead of a local
// storage.
func (b Builder) WithGlobalStorage(s *mem.Storage) Builder {
	b.storage = s
	b.useGlobalStorage = true
	return b
}

// WithInterleavingAddrConversion sets the rule to convert the global physical
// address to the internal physical address.
func (b Builder) WithInterleavingAddrConversion(
	interleaveGranularity uint64,
	numTotalUnit, currentUnitIndex int,
	lowerBound, upperBound uint64,
) Builder {
	b.hasAddrConverter = true
	b.interleavingSize = interleaveGranularity
	b.totalNumOfElements = numTotalUnit
	b.currentElementIndex = currentUnitIndex
	b.offset = lowerBound
	return b
}

// WithPagePolicy sets the page policy of the memory controller.
func (b Builder) WithPagePolicy(p PagePolicy) Builder {
	b.spec.PagePolicy = p
	return b
}

// WithProtocol sets the protocol of the memory controller.
func (b Builder) WithProtocol(protocol Protocol) Builder {
	b.protocol = protocol
	return b
}

// WithTransactionQueueSize sets the number of transactions can be buffered.
func (b Builder) WithTransactionQueueSize(n int) Builder {
	b.transactionQueueSize = n
	return b
}

// WithCommandQueueSize sets the number of commands per command queue.
func (b Builder) WithCommandQueueSize(n int) Builder {
	b.commandQueueSize = n
	return b
}

// WithBusWidth sets the bus width.
func (b Builder) WithBusWidth(n int) Builder {
	b.busWidth = n
	return b
}

// WithBurstLength sets the burst length.
func (b Builder) WithBurstLength(n int) Builder {
	b.burstLength = n
	return b
}

// WithDeviceWidth sets the device width.
func (b Builder) WithDeviceWidth(n int) Builder {
	b.deviceWidth = n
	return b
}

// WithNumChannel sets the number of channels.
func (b Builder) WithNumChannel(n int) Builder {
	b.numChannel = n
	return b
}

// WithNumRank sets the number of ranks.
func (b Builder) WithNumRank(n int) Builder {
	b.numRank = n
	return b
}

// WithNumBankGroup sets the number of bank groups.
func (b Builder) WithNumBankGroup(n int) Builder {
	b.numBankGroup = n
	return b
}

// WithNumBank sets the number of banks.
func (b Builder) WithNumBank(n int) Builder {
	b.numBank = n
	return b
}

// WithNumRow sets the number of rows.
func (b Builder) WithNumRow(n int) Builder {
	b.numRow = n
	return b
}

// WithNumCol sets the number of columns.
func (b Builder) WithNumCol(n int) Builder {
	b.numCol = n
	return b
}

// WithReadQueueSize sets the read queue size for R/W queue separation.
func (b Builder) WithReadQueueSize(n int) Builder {
	b.spec.ReadQueueSize = n
	return b
}

// WithWriteQueueSize sets the write queue size for R/W queue separation.
func (b Builder) WithWriteQueueSize(n int) Builder {
	b.spec.WriteQueueSize = n
	return b
}

// WithWriteHighWatermark sets the write high watermark for drain mode.
func (b Builder) WithWriteHighWatermark(n int) Builder {
	b.spec.WriteHighWatermark = n
	return b
}

// WithWriteLowWatermark sets the write low watermark for drain mode.
func (b Builder) WithWriteLowWatermark(n int) Builder {
	b.spec.WriteLowWatermark = n
	return b
}

// WithTopPort sets the top port.
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithAdditionalTracer adds one tracer.
func (b Builder) WithAdditionalTracer(t tracing.Tracer) Builder {
	b.tracers = append(b.tracers, t)
	return b
}

// WithTAL sets tAL.
func (b Builder) WithTAL(cycle int) Builder {
	b.tAL = cycle
	return b
}

// WithTCL sets tCL.
func (b Builder) WithTCL(cycle int) Builder {
	b.tCL = cycle
	return b
}

// WithTCWL sets tCWL.
func (b Builder) WithTCWL(cycle int) Builder {
	b.tCWL = cycle
	return b
}

// WithTRCD sets tRCD.
func (b Builder) WithTRCD(cycle int) Builder {
	b.tRCD = cycle
	return b
}

// WithTRP sets tRP.
func (b Builder) WithTRP(cycle int) Builder {
	b.tRP = cycle
	return b
}

// WithTRAS sets tRAS.
func (b Builder) WithTRAS(cycle int) Builder {
	b.tRAS = cycle
	return b
}

// WithTCCDL sets tCCDL.
func (b Builder) WithTCCDL(cycle int) Builder {
	b.tCCDL = cycle
	return b
}

// WithTCCDS sets tCCDS.
func (b Builder) WithTCCDS(cycle int) Builder {
	b.tCCDS = cycle
	return b
}

// WithTRTRS sets tRTRS.
func (b Builder) WithTRTRS(cycle int) Builder {
	b.tRTRS = cycle
	return b
}

// WithTRTP sets tRTP.
func (b Builder) WithTRTP(cycle int) Builder {
	b.tRTP = cycle
	return b
}

// WithTWTRL sets tWTRL.
func (b Builder) WithTWTRL(cycle int) Builder {
	b.tWTRL = cycle
	return b
}

// WithTWTRS sets tWTRS.
func (b Builder) WithTWTRS(cycle int) Builder {
	b.tWTRS = cycle
	return b
}

// WithTWR sets tWR.
func (b Builder) WithTWR(cycle int) Builder {
	b.tWR = cycle
	return b
}

// WithTPPD sets tPPD.
func (b Builder) WithTPPD(cycle int) Builder {
	b.tPPD = cycle
	return b
}

// WithTRRDL sets tRRDL.
func (b Builder) WithTRRDL(cycle int) Builder {
	b.tRRDL = cycle
	return b
}

// WithTRRDS sets tRRDS.
func (b Builder) WithTRRDS(cycle int) Builder {
	b.tRRDS = cycle
	return b
}

// WithTRCDRD sets tRCDRD.
func (b Builder) WithTRCDRD(cycle int) Builder {
	b.tRCDRD = cycle
	return b
}

// WithTRCDWR sets tRCDWR.
func (b Builder) WithTRCDWR(cycle int) Builder {
	b.tRCDWR = cycle
	return b
}

// WithTREFI sets tREFI.
func (b Builder) WithTREFI(cycle int) Builder {
	b.tREFI = cycle
	return b
}

// WithRFC sets tRFC.
func (b Builder) WithRFC(cycle int) Builder {
	b.tRFC = cycle
	return b
}

// WithRFCb sets tRFCb.
func (b Builder) WithRFCb(cycle int) Builder {
	b.tRFCb = cycle
	return b
}

// Build builds a new MemController.
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	b.calculateBurstCycle()
	b.tRL = b.tAL + b.tCL
	b.tWL = b.tAL + b.tCWL
	b.readDelay = b.tRL + b.burstCycle
	b.writeDelay = b.tRL + b.burstCycle
	b.tRC = b.tRAS + b.tRP

	spec := b.buildSpec()
	timing := b.generateTiming()
	spec.Timing = timing

	initialState := State{
		SubTransQueue: subTransQueueState{
			Entries: []subTransRef{},
		},
		CommandQueues: commandQueueState{
			NumQueues: b.numChannel * b.numRank,
			Entries:   []queueEntry{},
		},
		BankStates: initBankStatesFlat(
			b.numRank, b.numBankGroup, b.numBank),
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	var storage *mem.Storage
	if b.useGlobalStorage {
		storage = b.storage
	} else {
		devicePerRank := b.busWidth / b.deviceWidth
		bankSize := b.numCol * b.numRow * b.deviceWidth / 8
		rankSize := bankSize * b.numBank * devicePerRank
		totalSize := rankSize * b.numRank * b.numChannel
		storage = mem.NewStorage(uint64(totalSize))
	}

	b.addMiddlewares(modelComp, storage)

	b.topPort.SetComponent(modelComp)
	modelComp.AddPort("Top", b.topPort)

	for _, tracer := range b.tracers {
		tracing.CollectTrace(modelComp, tracer)
	}

	return modelComp
}

func (b Builder) addMiddlewares(
	modelComp *modeling.Component[Spec, State],
	storage *mem.Storage,
) {
	rMW := &respondMW{
		comp:    modelComp,
		topPort: b.topPort,
		storage: storage,
	}
	modelComp.AddMiddleware(rMW)

	btMW := &bankTickMW{
		comp: modelComp,
	}
	modelComp.AddMiddleware(btMW)

	ptMW := &parseTopMW{
		comp:    modelComp,
		topPort: b.topPort,
	}
	modelComp.AddMiddleware(ptMW)
}

func (b Builder) buildSpec() Spec {
	numAccessUnitBit, _ := log2(uint64(b.busWidth / 8 * b.burstLength))
	addrMapping := b.buildAddressMapping()
	cmdCycles := b.buildCmdCycles()

	spec := b.buildTimingSpec()
	spec.PagePolicy = b.spec.PagePolicy
	spec.BusWidth = b.busWidth
	spec.BurstLength = b.burstLength
	spec.DeviceWidth = b.deviceWidth
	spec.NumChannel = b.numChannel
	spec.NumRank = b.numRank
	spec.NumBankGroup = b.numBankGroup
	spec.NumBank = b.numBank
	spec.NumRow = b.numRow
	spec.NumCol = b.numCol
	spec.TransactionQueueSize = b.transactionQueueSize
	spec.CommandQueueCapacity = b.commandQueueSize
	spec.ReadQueueSize = b.spec.ReadQueueSize
	spec.WriteQueueSize = b.spec.WriteQueueSize
	spec.WriteHighWatermark = b.spec.WriteHighWatermark
	spec.WriteLowWatermark = b.spec.WriteLowWatermark
	spec.HasAddrConverter = b.hasAddrConverter
	spec.InterleavingSize = b.interleavingSize
	spec.TotalNumOfElements = b.totalNumOfElements
	spec.CurrentElementIndex = b.currentElementIndex
	spec.Offset = b.offset

	b.applyAddrMapping(&spec, addrMapping)

	spec.Log2AccessUnitSize = numAccessUnitBit
	spec.CmdCycles = cmdCycles

	return spec
}

func (b Builder) buildTimingSpec() Spec {
	return Spec{
		Freq:       b.spec.Freq,
		Protocol:   int(b.protocol),
		TAL:        b.tAL,
		TCL:        b.tCL,
		TCWL:       b.tCWL,
		TRL:        b.tRL,
		TWL:        b.tWL,
		ReadDelay:  b.readDelay,
		WriteDelay: b.writeDelay,
		TRCD:       b.tRCD,
		TRP:        b.tRP,
		TRAS:       b.tRAS,
		TCCDS:      b.tCCDS,
		TCCDL:      b.tCCDL,
		TRTRS:      b.tRTRS,
		TRTP:       b.tRTP,
		TWTRL:      b.tWTRL,
		TWTRS:      b.tWTRS,
		TWR:        b.tWR,
		TPPD:       b.tPPD,
		TRC:        b.tRC,
		TRRDS:      b.tRRDS,
		TRRDL:      b.tRRDL,
		TRCDRD:     b.tRCDRD,
		TRCDWR:     b.tRCDWR,
		TREFI:      b.tREFI,
		TRFC:       b.tRFC,
		TRFCb:      b.tRFCb,
		TCKESR:     b.tCKESR,
		TXS:        b.tXS,
		BurstCycle: b.burstCycle,
	}
}

func (b Builder) buildCmdCycles() map[CommandKind]int {
	cmdCycles := map[CommandKind]int{
		CmdKindRead:           b.readDelay,
		CmdKindReadPrecharge:  b.tRP,
		CmdKindWrite:          b.writeDelay,
		CmdKindWritePrecharge: b.tRP,
		CmdKindActivate:       b.tRCD - b.tAL,
		CmdKindPrecharge:      b.tRP,
		CmdKindRefreshBank:    1,
		CmdKindRefresh:        1,
		CmdKindSRefEnter:      1,
		CmdKindSRefExit:       1,
	}

	if b.protocol.isGDDR() || b.protocol.isHBM() {
		cmdCycles[CmdKindActivate] = b.tRCDRD - b.tAL
	}

	return cmdCycles
}

func (b Builder) applyAddrMapping(spec *Spec, m addrMappingResult) {
	spec.ChannelPos = m.channelPos
	spec.ChannelMask = m.channelMask
	spec.RankPos = m.rankPos
	spec.RankMask = m.rankMask
	spec.BankGroupPos = m.bankGroupPos
	spec.BankGroupMask = m.bankGroupMask
	spec.BankPos = m.bankPos
	spec.BankMask = m.bankMask
	spec.RowPos = m.rowPos
	spec.RowMask = m.rowMask
	spec.ColPos = m.colPos
	spec.ColMask = m.colMask
}

type addrMappingResult struct {
	channelPos    int
	channelMask   uint64
	rankPos       int
	rankMask      uint64
	bankGroupPos  int
	bankGroupMask uint64
	bankPos       int
	bankMask      uint64
	rowPos        int
	rowMask       uint64
	colPos        int
	colMask       uint64
}

func (b Builder) buildAddressMapping() addrMappingResult {
	channelBit, _ := log2(uint64(b.numChannel))
	rankBit, _ := log2(uint64(b.numRank))
	bankGroupBit, _ := log2(uint64(b.numBankGroup))
	bankBit, _ := log2(uint64(b.numBank))
	rowBit, _ := log2(uint64(b.numRow))
	colBit, _ := log2(uint64(b.numCol))
	colLoBit, _ := log2(uint64(b.burstLength))
	colHiBit := colBit - colLoBit
	accessUnitBit, _ := log2(uint64(b.busWidth / 8 * b.burstLength))

	r := addrMappingResult{
		channelMask:   (1 << channelBit) - 1,
		rankMask:      (1 << rankBit) - 1,
		bankGroupMask: (1 << bankGroupBit) - 1,
		bankMask:      (1 << bankBit) - 1,
		rowMask:       (1 << rowBit) - 1,
		colMask:       (1 << colHiBit) - 1,
	}

	bitWidths := addrBitWidths{
		channel:   channelBit,
		rank:      rankBit,
		bankGroup: bankGroupBit,
		bank:      bankBit,
		row:       rowBit,
		colHi:     colHiBit,
	}

	r.assignBitPositions(accessUnitBit, bitWidths)

	return r
}

type addrBitWidths struct {
	channel, rank, bankGroup, bank, row, colHi uint64
}

func (r *addrMappingResult) assignBitPositions(
	startPos uint64, w addrBitWidths,
) {
	// Default bit order high→low: Row, Channel, Rank, Bank, BankGroup, Column
	type locItem int
	const (
		liChannel locItem = iota
		liRank
		liBankGroup
		liBank
		liRow
		liColumn
	)

	bitOrder := []locItem{
		liRow, liChannel, liRank, liBank, liBankGroup, liColumn,
	}

	pos := startPos
	for i := len(bitOrder) - 1; i >= 0; i-- {
		switch bitOrder[i] {
		case liChannel:
			r.channelPos = int(pos)
			pos += w.channel
		case liRank:
			r.rankPos = int(pos)
			pos += w.rank
		case liBankGroup:
			r.bankGroupPos = int(pos)
			pos += w.bankGroup
		case liBank:
			r.bankPos = int(pos)
			pos += w.bank
		case liRow:
			r.rowPos = int(pos)
			pos += w.row
		case liColumn:
			r.colPos = int(pos)
			pos += w.colHi
		}
	}
}

//nolint:gocyclo,funlen
func (b *Builder) generateTiming() Timing {
	t := Timing{
		SameBank:              MakeTimeTable(),
		OtherBanksInBankGroup: MakeTimeTable(),
		SameRank:              MakeTimeTable(),
		OtherRanks:            MakeTimeTable(),
	}

	readToReadL := max(b.burstCycle, b.tCCDL)
	readToReadS := max(b.burstCycle, b.tCCDS)
	readToReadO := b.burstCycle + b.tRTRS
	readToWrite := b.tRL + b.burstCycle - b.tWL + b.tRTRS
	readToWriteO := b.readDelay + b.burstCycle +
		b.tRTRS - b.writeDelay
	readToPrecharge := b.tAL + b.tRTP
	readpToAct := b.tAL + b.burstCycle + b.tRTP + b.tRP

	writeToReadL := b.writeDelay + b.tWTRL
	writeToReadS := b.writeDelay + b.tWTRS
	writeToReadO := b.writeDelay + b.burstCycle +
		b.tRTRS - b.readDelay
	writeToWriteL := max(b.burstCycle, b.tCCDL)
	writeToWriteS := max(b.burstCycle, b.tCCDS)
	writeToWriteO := b.burstCycle
	writeToPrecharge := b.tWL + b.burstCycle + b.tWR

	prechargeToActivate := b.tRP
	prechargeToPrecharge := b.tPPD
	readToActivate := readToPrecharge + prechargeToActivate
	writeToActivate := writeToPrecharge + prechargeToActivate

	activateToActivate := b.tRC
	activateToActivateL := b.tRRDL
	activateToActivateS := b.tRRDS
	activateToPrecharge := b.tRAS
	activateToRead := b.tRCD - b.tAL
	activateToWrite := b.tRCD - b.tAL

	if b.protocol.isGDDR() || b.protocol.isHBM() {
		activateToRead = b.tRCDRD
		activateToWrite = b.tRCDWR
	}

	activateToRefresh := b.tRC

	refreshToRefresh := b.tREFI
	refreshToActivate := b.tRFC
	refreshToActivateBank := b.tRFCb

	selfRefreshEntryToExit := b.tCKESR
	selfRefreshExit := b.tXS

	if b.numBankGroup == 1 {
		readToReadL = max(b.burstCycle, b.tCCDS)
		writeToReadL = b.writeDelay + b.tWTRS
		writeToWriteL = max(b.burstCycle, b.tCCDS)
		activateToActivateL = b.tRRDS
	}

	t.SameBank[CmdKindRead] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: readToWrite},
		{NextCmdKind: CmdKindPrecharge, MinCycleInBetween: readToPrecharge},
	}
	t.OtherBanksInBankGroup[CmdKindRead] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.SameRank[CmdKindRead] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: readToReadS},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: readToReadS},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.OtherRanks[CmdKindRead] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: readToReadO},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: readToWriteO},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: readToReadO},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: readToWriteO},
	}

	t.SameBank[CmdKindWrite] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: CmdKindPrecharge, MinCycleInBetween: writeToPrecharge},
	}
	t.OtherBanksInBankGroup[CmdKindWrite] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
	}
	t.SameRank[CmdKindWrite] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: writeToReadS},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: writeToWriteS},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: writeToReadS},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: writeToWriteS},
	}
	t.OtherRanks[CmdKindWrite] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: writeToReadO},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: writeToWriteO},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: writeToReadO},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: writeToWriteO},
	}

	// READ_PRECHARGE
	t.SameBank[CmdKindReadPrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: readpToAct},
		{NextCmdKind: CmdKindRefresh, MinCycleInBetween: readToActivate},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: readToActivate},
		{NextCmdKind: CmdKindSRefEnter, MinCycleInBetween: readToActivate},
	}
	t.OtherBanksInBankGroup[CmdKindReadPrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.SameRank[CmdKindReadPrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: readToReadS},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: readToReadS},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.OtherRanks[CmdKindReadPrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: readToReadO},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: readToWriteO},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: readToReadO},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: readToWriteO},
	}

	// WRITE_PRECHARGE
	t.SameBank[CmdKindWritePrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: writeToActivate},
		{NextCmdKind: CmdKindRefresh, MinCycleInBetween: writeToActivate},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: writeToActivate},
		{NextCmdKind: CmdKindSRefEnter, MinCycleInBetween: writeToActivate},
	}
	t.OtherBanksInBankGroup[CmdKindWritePrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
	}
	t.SameRank[CmdKindWritePrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: writeToReadS},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: writeToWriteS},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: writeToReadS},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: writeToWriteS},
	}
	t.OtherRanks[CmdKindWritePrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindRead, MinCycleInBetween: writeToReadO},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: writeToWriteO},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: writeToReadO},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: writeToWriteO},
	}

	// ACTIVATE
	t.SameBank[CmdKindActivate] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: activateToActivate},
		{NextCmdKind: CmdKindRead, MinCycleInBetween: activateToRead},
		{NextCmdKind: CmdKindWrite, MinCycleInBetween: activateToWrite},
		{NextCmdKind: CmdKindReadPrecharge, MinCycleInBetween: activateToRead},
		{NextCmdKind: CmdKindWritePrecharge, MinCycleInBetween: activateToWrite},
		{NextCmdKind: CmdKindPrecharge, MinCycleInBetween: activateToPrecharge},
	}
	t.OtherBanksInBankGroup[CmdKindActivate] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: activateToActivateL},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: activateToRefresh},
	}
	t.SameRank[CmdKindActivate] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: activateToActivateS},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: activateToRefresh},
	}

	// PRECHARGE
	t.SameBank[CmdKindPrecharge] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: CmdKindRefresh, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: CmdKindSRefEnter, MinCycleInBetween: prechargeToActivate},
	}

	if b.protocol.isGDDR() || b.protocol == LPDDR4 {
		t.OtherBanksInBankGroup[CmdKindPrecharge] = []TimeTableEntry{
			{NextCmdKind: CmdKindPrecharge, MinCycleInBetween: prechargeToPrecharge},
		}
		t.SameRank[CmdKindPrecharge] = []TimeTableEntry{
			{NextCmdKind: CmdKindPrecharge, MinCycleInBetween: prechargeToPrecharge},
		}
	}

	// REFRESH_BANK
	t.SameRank[CmdKindRefreshBank] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: refreshToActivateBank},
		{NextCmdKind: CmdKindRefresh, MinCycleInBetween: refreshToActivateBank},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: refreshToActivateBank},
		{NextCmdKind: CmdKindSRefEnter, MinCycleInBetween: refreshToActivateBank},
	}
	t.OtherBanksInBankGroup[CmdKindRefreshBank] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: refreshToRefresh},
	}
	t.SameRank[CmdKindRefreshBank] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: refreshToRefresh},
	}

	// REFRESH
	t.SameRank[CmdKindRefresh] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: CmdKindRefresh, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: CmdKindSRefEnter, MinCycleInBetween: refreshToActivate},
	}

	// SREF_ENTER
	t.SameRank[CmdKindSRefEnter] = []TimeTableEntry{
		{NextCmdKind: CmdKindSRefExit, MinCycleInBetween: selfRefreshEntryToExit},
	}

	// SREF_EXIT
	t.SameRank[CmdKindSRefExit] = []TimeTableEntry{
		{NextCmdKind: CmdKindActivate, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: CmdKindRefresh, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: CmdKindRefreshBank, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: CmdKindSRefEnter, MinCycleInBetween: selfRefreshExit},
	}

	return t
}

func (b *Builder) calculateBurstCycle() {
	b.burstLengthMustNotBeZero()

	switch b.protocol {
	case GDDR5:
		b.burstCycle = b.burstLength / 4
	case GDDR5X:
		b.burstCycle = b.burstLength / 8
	case GDDR6:
		b.burstCycle = b.burstLength / 16
	default:
		b.burstCycle = b.burstLength / 2
	}
}

func (b *Builder) burstLengthMustNotBeZero() {
	if b.burstLength == 0 {
		panic("burst length cannot be 0")
	}
}

// log2 returns the log2 of a number. It also returns false if it is not a log2
// number.
func log2(n uint64) (uint64, bool) {
	oneCount := 0
	onePos := uint64(0)

	for i := uint64(0); i < 64; i++ {
		if n&(1<<i) > 0 {
			onePos = i
			oneCount++
		}
	}

	return onePos, oneCount == 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
