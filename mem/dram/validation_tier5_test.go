package dram

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Tier 5 — full-trace differential validation against DRAMSim3 and Ramulator2.
//
// The committed reference data (validation/data/reference.csv) was produced by
// running both oracles at pinned commits over the workload in
// validation/traces/scenarios.json (see validation/run_oracles.py). This test
// drives the *same* workload through Akita's dram.Comp and asserts the command
// counts match the reference exactly.
//
// For these pure close-page scenarios the command counts (activates == #ops,
// reads/writes == #ops of that type) are config- and address-map-independent,
// so they are a faithful cross-simulator quantity and are compared exactly.
// Latency alignment is a later increment (see validation/oracles/README.md).

const (
	tier5ScenariosPath = "validation/traces/scenarios.json"
	tier5ReferencePath = "validation/data/reference.csv"
)

type tier5Scenario struct {
	Name       string     `json:"name"`
	PagePolicy string     `json:"page_policy"`
	Ops        [][]uint64 `json:"ops"` // each op: [is_write, address]
}

type tier5Scenarios struct {
	Scenarios []tier5Scenario `json:"scenarios"`
}

type oracleCounts struct {
	activates, reads, writes int
}

// akitaCounts runs the scenario through a close-page DDR4 dram.Comp and returns
// the issued command counts.
func akitaCounts(scn tier5Scenario) oracleCounts {
	spec := DDR4Spec
	spec.PagePolicy = PagePolicyClose
	// DDR4Spec carries timing+geometry but not queue sizes; use the canonical
	// queue sizes the oracle configs use.
	spec.TransactionQueueSize = 32
	spec.CommandQueueCapacity = 8

	h := newP0Harness(spec)
	data := []byte{1, 2, 3, 4}
	for _, op := range scn.Ops {
		isWrite, addr := op[0], op[1]
		if isWrite == 1 {
			h.src.Send(h.write(addr, data))
		} else {
			h.src.Send(h.read(addr))
		}
	}
	h.engine.Run()

	st := &h.dram.State
	return oracleCounts{
		activates: int(st.TotalActivates),
		reads:     int(st.TotalReadCommands),
		writes:    int(st.TotalWriteCommands),
	}
}

func loadTier5Scenarios() ([]tier5Scenario, bool) {
	raw, err := os.ReadFile(tier5ScenariosPath)
	if err != nil {
		return nil, false
	}
	var s tier5Scenarios
	if json.Unmarshal(raw, &s) != nil {
		return nil, false
	}
	return s.Scenarios, true
}

// loadReference returns reference counts keyed by scenario then simulator.
func loadReference() (map[string]map[string]oracleCounts, bool) {
	f, err := os.Open(tier5ReferencePath)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil || len(records) < 2 {
		return nil, false
	}
	col := map[string]int{}
	for i, name := range records[0] {
		col[name] = i
	}

	out := map[string]map[string]oracleCounts{}
	for _, row := range records[1:] {
		scn := row[col["scenario"]]
		sim := row[col["simulator"]]
		atoi := func(k string) int { v, _ := strconv.Atoi(row[col[k]]); return v }
		if out[scn] == nil {
			out[scn] = map[string]oracleCounts{}
		}
		out[scn][sim] = oracleCounts{
			activates: atoi("activates"),
			reads:     atoi("reads"),
			writes:    atoi("writes"),
		}
	}
	return out, true
}

var _ = Describe("Tier 5: differential vs DRAMSim3 & Ramulator2", func() {
	scenarios, okScn := loadTier5Scenarios()
	reference, okRef := loadReference()

	BeforeEach(func() {
		if !okScn || !okRef {
			Skip("validation reference data not present; run " +
				"mem/dram/validation/run_oracles.py to (re)generate it")
		}
	})

	It("matches the committed oracle command counts exactly", func() {
		Expect(scenarios).NotTo(BeEmpty())

		for _, scn := range scenarios {
			ref, ok := reference[scn.Name]
			Expect(ok).To(BeTrue(), "no reference rows for %s", scn.Name)

			akita := akitaCounts(scn)

			for sim, want := range ref {
				Expect(akita.activates).To(Equal(want.activates),
					"%s: activates vs %s", scn.Name, sim)
				Expect(akita.reads).To(Equal(want.reads),
					"%s: reads vs %s", scn.Name, sim)
				Expect(akita.writes).To(Equal(want.writes),
					"%s: writes vs %s", scn.Name, sim)
			}
		}
	})
})
