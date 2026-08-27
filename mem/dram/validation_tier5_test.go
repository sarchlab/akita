package dram

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Tier 5 — differential validation against DRAMSim3 and Ramulator2.
//
// One scenario set (validation/traces/scenarios.json) drives all three
// simulators on the same workload. Each scenario is compared on BOTH:
//
//   * command counts (activates/reads/writes) — Akita is asserted to match the
//     oracles wherever the two oracles AGREE on a count (reads/writes always
//     agree: one column command per access; activates agree where the count is
//     map-independent). Where the oracles disagree it's a documented reference
//     divergence and is not asserted.
//
//   * read latency vs DRAMSim3 — within 15% for "enforced" scenarios.
//     "known_gap" scenarios currently exceed 15% for a feature Akita lacks
//     (configurable address mapping) and are tracked until it lands.
//
// The committed reference data was produced by validation/run_oracles.py.

const (
	scenariosPath    = "validation/traces/scenarios.json"
	referencePath    = "validation/data/reference.csv"
	latencyTolerance = 0.15
)

type scnPattern struct {
	Op     string `json:"op"`     // "read" | "write"
	Count  int    `json:"count"`  // number of accesses
	Stride string `json:"stride"` // hex address stride, e.g. "0x2000"
}

type scenario struct {
	Name        string     `json:"name"`
	PagePolicy  string     `json:"page_policy"`
	Pattern     scnPattern `json:"pattern"`
	ReadLatency string     `json:"read_latency"` // "enforced" | "known_gap" | "off"
	GapReason   string     `json:"gap_reason"`
}

// ops expands the scenario's pattern into [is_write, address] pairs, matching
// run_oracles.build_ops so Akita drives the exact workload the oracles saw.
func (s scenario) ops() [][2]uint64 {
	stride, _ := strconv.ParseUint(s.Pattern.Stride, 0, 64)
	var isWrite uint64
	if s.Pattern.Op == "write" {
		isWrite = 1
	}
	out := make([][2]uint64, s.Pattern.Count)
	for i := range out {
		out[i] = [2]uint64{isWrite, uint64(i) * stride}
	}
	return out
}

type oracleResult struct {
	activates, reads, writes int
	avgReadLatency           float64 // NaN when the oracle did not record it
}

type akitaResult struct {
	activates, reads, writes int
	avgReadLatency           float64
}

// runAkita drives the scenario through a dram.Comp configured to match the
// canonical oracle config (DDR4, refresh off, given page policy) and returns
// the issued command counts and the average read latency in DRAM cycles.
func runAkita(s scenario) akitaResult {
	spec := DDR4Spec
	if s.PagePolicy == "open" {
		spec.PagePolicy = PagePolicyOpen
	} else {
		spec.PagePolicy = PagePolicyClose
	}
	spec.TransactionQueueSize = 32
	spec.CommandQueueCapacity = 8
	spec.TREFI = 0 // refresh off, matching the oracle reference runs

	h := newP0Harness(spec)
	data := []byte{1, 2, 3, 4}
	for _, op := range s.ops() {
		if op[0] == 1 {
			h.src.Send(h.write(op[1], data))
		} else {
			h.src.Send(h.read(op[1]))
		}
	}
	h.engine.Run()

	st := &h.dram.State
	lat := math.NaN()
	if st.CompletedReads > 0 {
		lat = float64(st.TotalReadLatencyCycles) / float64(st.CompletedReads)
	}
	return akitaResult{
		activates:      int(st.TotalActivates),
		reads:          int(st.TotalReadCommands),
		writes:         int(st.TotalWriteCommands),
		avgReadLatency: lat,
	}
}

func loadScenarios() ([]scenario, bool) {
	raw, err := os.ReadFile(scenariosPath)
	if err != nil {
		return nil, false
	}
	var doc struct {
		Scenarios []scenario `json:"scenarios"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil, false
	}
	return doc.Scenarios, true
}

// loadReference returns reference results keyed by scenario then simulator.
func loadReference() (map[string]map[string]oracleResult, bool) {
	f, err := os.Open(referencePath)
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

	out := map[string]map[string]oracleResult{}
	for _, row := range records[1:] {
		scn := row[col["scenario"]]
		sim := row[col["simulator"]]
		atoi := func(k string) int { v, _ := strconv.Atoi(row[col[k]]); return v }
		lat := math.NaN()
		if s := row[col["avg_read_latency_cycles"]]; s != "" {
			lat, _ = strconv.ParseFloat(s, 64)
		}
		if out[scn] == nil {
			out[scn] = map[string]oracleResult{}
		}
		out[scn][sim] = oracleResult{
			activates:      atoi("activates"),
			reads:          atoi("reads"),
			writes:         atoi("writes"),
			avgReadLatency: lat,
		}
	}
	return out, true
}

var (
	scen, okScen = loadScenarios()
	ref, okRef   = loadReference()
)

// requireReference fails (not skips): the fixtures are committed and required,
// so a missing or malformed file is a breakage CI must catch, not silently pass.
func requireReference() {
	Expect(okScen).To(BeTrue(), "committed %s missing or malformed "+
		"(required; regenerate with validation/run_oracles.py)", scenariosPath)
	Expect(okRef).To(BeTrue(), "committed %s missing or malformed "+
		"(required; regenerate with validation/run_oracles.py)", referencePath)
}

// dramsim3RefLatency returns the DRAMSim3 reference read latency for a scenario,
// failing if the row is missing or not a positive finite value.
func dramsim3RefLatency(scnName string) float64 {
	row, ok := ref[scnName]["dramsim3"]
	Expect(ok).To(BeTrue(), "%s: missing dramsim3 row in reference.csv", scnName)
	lat := row.avgReadLatency
	Expect(math.IsNaN(lat)).To(BeFalse(), "%s: dramsim3 latency not recorded", scnName)
	Expect(lat).To(BeNumerically(">", 0), "%s: dramsim3 latency must be positive", scnName)
	return lat
}

var _ = Describe("Tier 5: Differential validation vs DRAMSim3 & Ramulator2", func() {
	BeforeEach(requireReference)

	It("matches command counts where the two oracles agree", func() {
		checked := 0
		for _, s := range scen {
			ds3, okd := ref[s.Name]["dramsim3"]
			ram, okr := ref[s.Name]["ramulator2"]
			Expect(okd && okr).To(BeTrue(),
				"%s: need both oracle rows in reference.csv", s.Name)

			akita := runAkita(s)
			assertIfAgree := func(name string, dv, rv, av int) {
				if dv == rv { // oracles agree -> the count is well-defined
					Expect(av).To(Equal(dv),
						"%s: %s vs oracles (both report %d)", s.Name, name, dv)
				}
			}
			assertIfAgree("activates", ds3.activates, ram.activates, akita.activates)
			assertIfAgree("reads", ds3.reads, ram.reads, akita.reads)
			assertIfAgree("writes", ds3.writes, ram.writes, akita.writes)
			checked++
		}
		Expect(checked).To(BeNumerically(">", 0))
	})

	It("matches DRAMSim3 read latency within 15% (enforced scenarios)", func() {
		checked := 0
		for _, s := range scen {
			if s.ReadLatency != "enforced" {
				continue
			}
			refLat := dramsim3RefLatency(s.Name)
			akita := runAkita(s).avgReadLatency
			relErr := math.Abs(akita-refLat) / refLat
			Expect(relErr).To(BeNumerically("<=", latencyTolerance),
				"%s: Akita %.1f vs DRAMSim3 %.1f cyc (%.0f%% off)",
				s.Name, akita, refLat, relErr*100)
			checked++
		}
		Expect(checked).To(BeNumerically(">", 0))
	})

	// KNOWN GAPS — probe a feature Akita does not yet support. The suite records
	// that the divergence is currently large; when the feature lands and the gap
	// closes (<=15%), this spec FAILS — the signal to move the scenario to
	// read_latency="enforced".
	It("records the known feature gaps (currently exceeding 15%)", func() {
		checked := 0
		for _, s := range scen {
			if s.ReadLatency != "known_gap" {
				continue
			}
			refLat := dramsim3RefLatency(s.Name)
			akita := runAkita(s).avgReadLatency
			relErr := math.Abs(akita-refLat) / refLat
			GinkgoWriter.Printf(
				"KNOWN GAP %-16s Akita %.1f vs DRAMSim3 %.1f cyc (%.0f%% off) — %s\n",
				s.Name, akita, refLat, relErr*100, s.GapReason)
			Expect(relErr).To(BeNumerically(">", latencyTolerance),
				"%s gap closed (now %.0f%% off) — promote to read_latency=enforced",
				s.Name, relErr*100)
			checked++
		}
		Expect(checked).To(BeNumerically(">", 0),
			"no known_gap scenarios ran — if all gaps are fixed, delete this spec")
	})
})
