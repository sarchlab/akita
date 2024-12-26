package dram

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"

	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"

	"github.com/sarchlab/akita/v4/mem/dram/internal/addressmapping"
	"github.com/sarchlab/akita/v4/mem/dram/internal/cmdq"
	"github.com/sarchlab/akita/v4/mem/dram/internal/org"
	"github.com/sarchlab/akita/v4/mem/dram/internal/trans"
)

// Builder can build new memory controllers.
type Builder struct {
	simulation       simulation.Simulation
	freq             timing.Freq
	useGlobalStorage bool
	storage          *mem.Storage
	addrConverter    mem.AddressConverter
	hooks            []hooking.Hook

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
}

// MakeBuilder creates a builder with default configuration.
func MakeBuilder() Builder {
	b := Builder{
		freq:                 1600 * timing.MHz,
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

// WithSimulation sets the simulation that the builder uses.
func (b Builder) WithSimulation(simulation simulation.Simulation) Builder {
	b.simulation = simulation
	return b
}

// WithFreq sets the frequency of the builder.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithGlobalStorage asks the DRAM to use a global storage instead of a local
// storage. Use this when you want to provide a unified storage for your whole
// simulation. The address of the storage is the global physical address.
func (b Builder) WithGlobalStorage(s *mem.Storage) Builder {
	b.storage = s
	b.useGlobalStorage = true

	return b
}

// WithInterleavingAddrConversion sets the rule to convert the global physical
// address to the internal physical address.
//
// For example, in a GPU that has 8 memory controllers. The addresses are
// interleaved across all the memory controllers at the page granularity. The
// current DRAM is the 3rd in the array of 8 memory controller. Also, there are
// 4 GPUs in total and each GPU has 4GB memory. The CPU also has 4GB memory,
// occupying the physical address from 0-4GB. The current GPU is the 2nd GPU. So
// the address range is from 8GB - 12GB. In this case, the use should call this
// function as `WithAddrConversion(4096, 8, 3, 8*mem.GB, 12*mem.GB)`.
//
// If there is only cone memory controller in your simulation, this function
// should not be called and the global physical address is equivalent to the
// DRAM controller's internal physical address.
func (b Builder) WithInterleavingAddrConversion(
	interleaveGranularity uint64,
	numTotalUnit, currentUnitIndex int,
	lowerBound, upperBound uint64,
) Builder {
	b.addrConverter = mem.InterleavingConverter{
		InterleavingSize:    interleaveGranularity,
		TotalNumOfElements:  numTotalUnit,
		CurrentElementIndex: currentUnitIndex,
		Offset:              lowerBound,
	}

	return b
}

// WithProtocol sets the protocol of the memory controller.
func (b Builder) WithProtocol(protocol Protocol) Builder {
	b.protocol = protocol
	return b
}

// WithTransactionQueueSize sets the number of transactions can be buffered
// before converting them into commands. Note that accesses that touches
// multiple access units (BusWidth/8*BurstLength bytes) may need to be split
// into multiple transactions.
func (b Builder) WithTransactionQueueSize(n int) Builder {
	b.transactionQueueSize = n
	return b
}

// WithCommandQueueSize sets the number of command that each command queue
// can hold.
func (b Builder) WithCommandQueueSize(n int) Builder {
	b.commandQueueSize = n
	return b
}

// WithBusWidth sets the number of bits can be transferred out of the banks
// at the same time.
func (b Builder) WithBusWidth(n int) Builder {
	b.busWidth = n
	return b
}

// WithBurstLength sets the number of access (each access manipulates the amount
// of data that equals the bus width) that takes place as one group.
func (b Builder) WithBurstLength(n int) Builder {
	b.burstLength = n
	return b
}

// WithDeviceWidth sets the number of bit that a bank can deliver at the same
// time.
func (b Builder) WithDeviceWidth(n int) Builder {
	b.deviceWidth = n
	return b
}

// WithNumChannel sets the channels that the memory controller controls.
func (b Builder) WithNumChannel(n int) Builder {
	b.numChannel = n
	return b
}

// WithNumRank sets the number of ranks in each channel. Number of ranks is
// typically the last parameter to determine. Here is how you can calculate
// the number of ranks. Suppose your total memory capacity is B_{ctrl}, channel
// count N_{chn}, row count N_{row}, column count N_col, bus width W_b, device
// width W_d. You can calculate the bank size as B_b with B_b = N_{col} *
// N_{row} * W_d. The rank size can be calculated with B_r = B_b * N_b *
// N_{device_per_rank}, where N_{device_per_rank} can be calculated with
// N_{device_per_rank} = W_b/W_d. Finally, the number of ranks is N_r =
// B_{ctrl} / N_{chn} / B_r.
func (b Builder) WithNumRank(n int) Builder {
	b.numRank = n
	return b
}

// WithNumBankGroup sets the number of bank groups in each rank.
func (b Builder) WithNumBankGroup(n int) Builder {
	b.numBankGroup = n
	return b
}

// WithNumBank sets the number of banks in each bank group.
func (b Builder) WithNumBank(n int) Builder {
	b.numBank = n
	return b
}

// WithNumRow sets the number of rows in each DRAM array.
func (b Builder) WithNumRow(n int) Builder {
	b.numRow = n
	return b
}

// WithNumCol sets the number of columns in each DRAM array.
func (b Builder) WithNumCol(n int) Builder {
	b.numCol = n
	return b
}

// WithAdditionalHooks adds the given hook to the memory controller and all the
// banks.
func (b Builder) WithAdditionalHooks(h hooking.Hook) Builder {
	b.hooks = append(b.hooks, h)
	return b
}

// WithTAL sets the additional latency to column access in cycles.
func (b Builder) WithTAL(cycle int) Builder {
	b.tAL = cycle
	return b
}

// WithTCL sets the column access strobe latency in cycles
func (b Builder) WithTCL(cycle int) Builder {
	b.tCL = cycle
	return b
}

// WithTCWL sets the column write strobe latency in cycles
func (b Builder) WithTCWL(cycle int) Builder {
	b.tCWL = cycle
	return b
}

// WithTRCD sets the row-to-column delay in cycles.
func (b Builder) WithTRCD(cycle int) Builder {
	b.tRCD = cycle
	return b
}

// WithTRP sets the row precharge latency in cycles.
func (b Builder) WithTRP(cycle int) Builder {
	b.tRP = cycle
	return b
}

// WithTRAS sets the row access strobe latency in cycles.
func (b Builder) WithTRAS(cycle int) Builder {
	b.tRAS = cycle
	return b
}

// WithTCCDL sets the long column-to-column delay in cycles. The long delay
// describes accesses to banks in the same bank group.
func (b Builder) WithTCCDL(cycle int) Builder {
	b.tCCDL = cycle
	return b
}

// WithTCCDS sets the short column-to-column delay in cycles. The long delay
// describes accesses to banks from different bank groups.
func (b Builder) WithTCCDS(cycle int) Builder {
	b.tCCDS = cycle
	return b
}

// WithTRTRS sets the rank-to-rank switching latency.
func (b Builder) WithTRTRS(cycle int) Builder {
	b.tRTRS = cycle
	return b
}

// WithTRTP sets the row-to-precharge latency in cycles.
func (b Builder) WithTRTP(cycle int) Builder {
	b.tRTP = cycle
	return b
}

// WithTWTRL sets the long write-to-read latency in cycles. The long latency
// describes write and read to banks from the same bank group.
func (b Builder) WithTWTRL(cycle int) Builder {
	b.tWTRL = cycle
	return b
}

// WithTWTRS sets the short write-to-read latency in cycles. The short latency
// describes write and read to banks from different bank groups.
func (b Builder) WithTWTRS(cycle int) Builder {
	b.tWTRS = cycle
	return b
}

// WithTWR sets the write recovery time in cycles.
func (b Builder) WithTWR(cycle int) Builder {
	b.tWR = cycle
	return b
}

// WithTPPD sets the precharge to precharge delay in cycles.
func (b Builder) WithTPPD(cycle int) Builder {
	b.tPPD = cycle
	return b
}

// WithTRRDL sets the long activate to activate latency in cycles. The long
// latency describes activating different banks from the same bank group.
func (b Builder) WithTRRDL(cycle int) Builder {
	b.tRRDL = cycle
	return b
}

// WithTRRDS sets the short activate to activate latency in cycles. The short
// latency describes activating different banks from different bank groups.
func (b Builder) WithTRRDS(cycle int) Builder {
	b.tRRDS = cycle
	return b
}

// WithTRCDRD sets the activate to read latency in cycles. It only works for
// GDDR DRAMs.
func (b Builder) WithTRCDRD(cycle int) Builder {
	b.tRCDRD = cycle
	return b
}

// WithTRCDWR sets the activate to write latency in cycles. It only works for
// GDDR DRAMs.
func (b Builder) WithTRCDWR(cycle int) Builder {
	b.tRCDWR = cycle
	return b
}

// WithTREFI sets the refresh interval in cycles.
func (b Builder) WithTREFI(cycle int) Builder {
	b.tREFI = cycle
	return b
}

// WithRFC sets the refresh cycle time in cycles.
func (b Builder) WithRFC(cycle int) Builder {
	b.tRFC = cycle
	return b
}

// WithRFCb sets the refresh to activate bank latency in cycles.
func (b Builder) WithRFCb(cycle int) Builder {
	b.tRFCb = cycle
	return b
}

// Build builds a new MemController.
func (b Builder) Build(name string) *Comp {
	m := &Comp{
		addrConverter: b.addrConverter,
		storage:       b.storage,
	}
	m.TickingComponent = modeling.NewTickingComponent(
		name, b.simulation.GetEngine(), b.freq, m)

	b.attachHooks(m)
	b.buildChannel(name, m)

	m.addrConverter = b.addrConverter
	m.addrMapper = addressmapping.MakeBuilder().
		WithBurstLength(b.burstLength).
		WithBusWidth(b.busWidth).
		WithNumChannel(b.numChannel).
		WithNumRank(b.numRank).
		WithNumBankGroup(b.numBankGroup).
		WithNumBank(b.numBank).
		WithNumCol(b.numCol).
		WithNumRow(b.numRow).
		Build()

	numAccessUnitBit, _ := log2(uint64(b.busWidth / 8 * b.burstLength))
	m.subTransSplitter = trans.NewSubTransSplitter(numAccessUnitBit)
	m.cmdQueue = &cmdq.CommandQueueImpl{
		Queues:           make([]cmdq.Queue, b.numChannel*b.numRank),
		CapacityPerQueue: b.commandQueueSize,
		Channel:          m.channel,
	}
	m.subTransactionQueue = &trans.FCFSSubTransactionQueue{
		Capacity: b.transactionQueueSize,
		CmdQueue: m.cmdQueue,
		CmdCreator: &trans.ClosePageCommandCreator{
			AddrMapper: m.addrMapper,
		},
	}

	if b.useGlobalStorage {
		m.storage = b.storage
	} else {
		devicePerRank := b.busWidth / b.deviceWidth
		bankSize := b.numCol * b.numRow * b.deviceWidth / 8
		rankSize := bankSize * b.numBank * devicePerRank
		totalSize := rankSize * b.numRank * b.numChannel
		m.storage = mem.NewStorage(uint64(totalSize))
	}

	m.topPort = modeling.PortBuilder{}.
		WithComponent(m).
		WithSimulation(b.simulation).
		WithIncomingBufCap(1024).
		WithOutgoingBufCap(1024).
		Build(name + ".TopPort")
	m.AddPort("Top", m.topPort)

	middleware := &middleware{Comp: m}
	m.AddMiddleware(middleware)

	return m
}

func (b Builder) attachHooks(hookable hooking.Hookable) {
	for _, hook := range b.hooks {
		hookable.AcceptHook(hook)
	}
}

func (b Builder) buildChannel(name string, m *Comp) {
	timing := b.generateTiming()
	channel := &org.ChannelImpl{
		Timing: timing,
	}

	channel.Banks = make(org.Banks, b.numRank)
	for i := 0; i < b.numRank; i++ {
		channel.Banks[i] = make([][]org.Bank, b.numBankGroup)

		for j := 0; j < b.numBankGroup; j++ {
			channel.Banks[i][j] = make([]org.Bank, b.numBank)

			for k := 0; k < b.numBank; k++ {
				bankName := fmt.Sprintf("%s.Bank[%d][%d][%d]",
					name, i, j, k)
				bank := org.NewBankImpl(bankName)
				bank.CmdCycles = map[signal.CommandKind]int{
					signal.CmdKindRead:           b.readDelay,
					signal.CmdKindReadPrecharge:  b.tRP,
					signal.CmdKindWrite:          b.writeDelay,
					signal.CmdKindWritePrecharge: b.tRP,
					signal.CmdKindActivate:       b.tRCD - b.tAL,
					signal.CmdKindPrecharge:      b.tRP,
					signal.CmdKindRefreshBank:    1,
					signal.CmdKindRefresh:        1,
					signal.CmdKindSRefEnter:      1,
					signal.CmdKindSRefExit:       1,
				}

				if b.protocol.isGDDR() || b.protocol.isHBM() {
					bank.CmdCycles[signal.CmdKindActivate] = b.tRCDRD - b.tAL
				}

				channel.Banks[i][j][k] = bank

				b.attachHooks(bank)
			}
		}
	}

	m.channel = channel
}

//nolint:gocyclo,funlen,govet
func (b *Builder) generateTiming() org.Timing {
	t := org.Timing{
		SameBank:              org.MakeTimeTable(),
		OtherBanksInBankGroup: org.MakeTimeTable(),
		SameRank:              org.MakeTimeTable(),
		OtherRanks:            org.MakeTimeTable(),
	}

	b.calculateBurstCycle()

	b.tRL = b.tAL + b.tCL
	b.tWL = b.tAL + b.tCWL
	b.readDelay = b.tRL + b.burstCycle
	b.writeDelay = b.tRL + b.burstCycle
	b.tRC = b.tRAS + b.tRP

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

	activateToRefresh := b.tRC // need to precharge before ref, so it's tRC

	refreshToRefresh := b.tREFI
	refreshToActivate := b.tRFC
	refreshToActivateBank := b.tRFCb

	selfRefreshEntryToExit := b.tCKESR
	selfRefreshExit := b.tXS

	if b.numBankGroup == 1 {
		// Bank-group can be disabled. In that case
		// the value of tXXX_S should be used instead of tXXX_L
		// (because now the device is running at a lower freq)
		// we overwrite the following values so that we don't have
		// to change the assignment of the vectors
		readToReadL = max(b.burstCycle, b.tCCDS)
		writeToReadL = b.writeDelay + b.tWTRS
		writeToWriteL = max(b.burstCycle, b.tCCDS)
		activateToActivateL = b.tRRDS
	}

	t.SameBank[signal.CmdKindRead] = []org.TimeTableEntry{
		{
			NextCmdKind:       signal.CmdKindRead,
			MinCycleInBetween: readToReadL,
		},
		{
			NextCmdKind:       signal.CmdKindWrite,
			MinCycleInBetween: readToWrite,
		},
		{
			NextCmdKind:       signal.CmdKindReadPrecharge,
			MinCycleInBetween: readToReadL,
		},
		{
			NextCmdKind:       signal.CmdKindWritePrecharge,
			MinCycleInBetween: readToWrite,
		},
		{
			NextCmdKind:       signal.CmdKindPrecharge,
			MinCycleInBetween: readToPrecharge,
		},
	}

	t.OtherBanksInBankGroup[signal.CmdKindRead] = []org.TimeTableEntry{
		{
			NextCmdKind:       signal.CmdKindRead,
			MinCycleInBetween: readToReadL,
		},
		{
			NextCmdKind:       signal.CmdKindWrite,
			MinCycleInBetween: readToWrite,
		},
		{
			NextCmdKind:       signal.CmdKindReadPrecharge,
			MinCycleInBetween: readToReadL,
		},
		{
			NextCmdKind:       signal.CmdKindWritePrecharge,
			MinCycleInBetween: readToWrite,
		},
	}
	t.SameRank[signal.CmdKindRead] = []org.TimeTableEntry{
		{
			NextCmdKind:       signal.CmdKindRead,
			MinCycleInBetween: readToReadS,
		},
		{
			NextCmdKind:       signal.CmdKindWrite,
			MinCycleInBetween: readToWrite,
		},
		{
			NextCmdKind:       signal.CmdKindReadPrecharge,
			MinCycleInBetween: readToReadS,
		},
	}
	t.OtherRanks[signal.CmdKindRead] = []org.TimeTableEntry{
		{
			NextCmdKind:       signal.CmdKindRead,
			MinCycleInBetween: readToReadO,
		},
		{
			NextCmdKind:       signal.CmdKindWrite,
			MinCycleInBetween: readToWriteO,
		},
	}

	t.SameBank[signal.CmdKindWrite] = []org.TimeTableEntry{
		{
			NextCmdKind:       signal.CmdKindRead,
			MinCycleInBetween: writeToReadL,
		},
		{
			NextCmdKind:       signal.CmdKindWrite,
			MinCycleInBetween: writeToWriteL,
		},
		{
			NextCmdKind:       signal.CmdKindReadPrecharge,
			MinCycleInBetween: writeToReadL,
		},
		{
			NextCmdKind:       signal.CmdKindPrecharge,
			MinCycleInBetween: writeToPrecharge,
		},
	}
	t.OtherBanksInBankGroup[signal.CmdKindWrite] = []org.TimeTableEntry{
		{
			NextCmdKind:       signal.CmdKindRead,
			MinCycleInBetween: writeToReadL,
		},
		{
			NextCmdKind:       signal.CmdKindWrite,
			MinCycleInBetween: writeToWriteL,
		},
		{
			NextCmdKind:       signal.CmdKindReadPrecharge,
			MinCycleInBetween: writeToReadL,
		},
	}
	t.SameRank[signal.CmdKindWrite] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: writeToReadS,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: writeToWriteS,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: writeToReadS,
			},
		}
	t.OtherRanks[signal.CmdKindWrite] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: writeToReadO,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: writeToWriteO,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: writeToReadO,
			},
			{
				NextCmdKind:       signal.CmdKindWritePrecharge,
				MinCycleInBetween: writeToWriteO,
			},
		}

	// command READ_PRECHARGE
	t.SameBank[signal.CmdKindReadPrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: readpToAct,
			},
			{
				NextCmdKind:       signal.CmdKindRefresh,
				MinCycleInBetween: readToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: readToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindSRefEnter,
				MinCycleInBetween: readToActivate,
			},
		}
	t.OtherBanksInBankGroup[signal.CmdKindReadPrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: readToReadL,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: readToWrite,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: readToReadL,
			},
			{
				NextCmdKind:       signal.CmdKindWritePrecharge,
				MinCycleInBetween: readToWrite,
			},
		}
	t.SameRank[signal.CmdKindReadPrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: readToReadS,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: readToWrite,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: readToReadS,
			},
		}
	t.OtherRanks[signal.CmdKindReadPrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: readToReadO,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: readToWriteO,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: readToReadO,
			},
			{
				NextCmdKind:       signal.CmdKindWritePrecharge,
				MinCycleInBetween: readToWriteO,
			},
		}

	// command WRITE_PRECHARGE
	t.SameBank[signal.CmdKindWritePrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: writeToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefresh,
				MinCycleInBetween: writeToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: writeToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindSRefEnter,
				MinCycleInBetween: writeToActivate,
			},
		}
	t.OtherBanksInBankGroup[signal.CmdKindWritePrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: writeToReadL,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: writeToWriteL,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: writeToReadL,
			},
		}
	t.SameRank[signal.CmdKindWritePrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: writeToReadS,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: writeToWriteS,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: writeToReadS,
			},
		}
	t.OtherRanks[signal.CmdKindWritePrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: writeToReadO,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: writeToWriteO,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: writeToReadO,
			},
		}

	// command ACTIVATE
	t.SameBank[signal.CmdKindActivate] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: activateToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRead,
				MinCycleInBetween: activateToRead,
			},
			{
				NextCmdKind:       signal.CmdKindWrite,
				MinCycleInBetween: activateToWrite,
			},
			{
				NextCmdKind:       signal.CmdKindReadPrecharge,
				MinCycleInBetween: activateToRead,
			},
			{
				NextCmdKind:       signal.CmdKindWritePrecharge,
				MinCycleInBetween: activateToWrite,
			},
			{
				NextCmdKind:       signal.CmdKindPrecharge,
				MinCycleInBetween: activateToPrecharge,
			},
		}

	t.OtherBanksInBankGroup[signal.CmdKindActivate] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: activateToActivateL,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: activateToRefresh,
			},
		}

	t.SameRank[signal.CmdKindActivate] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: activateToActivateS,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: activateToRefresh,
			},
		}

	// command PRECHARGE
	t.SameBank[signal.CmdKindPrecharge] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: prechargeToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefresh,
				MinCycleInBetween: prechargeToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: prechargeToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindSRefEnter,
				MinCycleInBetween: prechargeToActivate,
			},
		}

	// for those who need tPPD
	if b.protocol.isGDDR() || b.protocol == LPDDR4 {
		t.OtherBanksInBankGroup[signal.CmdKindPrecharge] =
			[]org.TimeTableEntry{
				{
					NextCmdKind:       signal.CmdKindPrecharge,
					MinCycleInBetween: prechargeToPrecharge,
				},
			}

		t.SameRank[signal.CmdKindPrecharge] =
			[]org.TimeTableEntry{
				{
					NextCmdKind:       signal.CmdKindPrecharge,
					MinCycleInBetween: prechargeToPrecharge,
				},
			}
	}

	// command REFRESH_BANK
	t.SameRank[signal.CmdKindRefreshBank] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: refreshToActivateBank,
			},
			{
				NextCmdKind:       signal.CmdKindRefresh,
				MinCycleInBetween: refreshToActivateBank,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: refreshToActivateBank,
			},
			{
				NextCmdKind:       signal.CmdKindSRefEnter,
				MinCycleInBetween: refreshToActivateBank,
			},
		}

	t.OtherBanksInBankGroup[signal.CmdKindRefreshBank] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: refreshToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: refreshToRefresh,
			},
		}

	t.SameRank[signal.CmdKindRefreshBank] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: refreshToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: refreshToRefresh,
			},
		}

	// REFRESH, SREF_ENTER and SREF_EXIT are isued to the entire
	// rank  command REFRESH
	t.SameRank[signal.CmdKindRefresh] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: refreshToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindRefresh,
				MinCycleInBetween: refreshToActivate,
			},
			{
				NextCmdKind:       signal.CmdKindSRefEnter,
				MinCycleInBetween: refreshToActivate,
			},
		}

	// command SREF_ENTER
	// TODO: add power down commands
	t.SameRank[signal.CmdKindSRefEnter] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindSRefExit,
				MinCycleInBetween: selfRefreshEntryToExit,
			},
		}

	// command SREF_EXIT
	t.SameRank[signal.CmdKindSRefExit] =
		[]org.TimeTableEntry{
			{
				NextCmdKind:       signal.CmdKindActivate,
				MinCycleInBetween: selfRefreshExit,
			},
			{
				NextCmdKind:       signal.CmdKindRefresh,
				MinCycleInBetween: selfRefreshExit,
			},
			{
				NextCmdKind:       signal.CmdKindRefreshBank,
				MinCycleInBetween: selfRefreshExit,
			},
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
