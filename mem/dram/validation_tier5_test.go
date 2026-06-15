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

// Tier 5/6 — differential validation against DRAMSim3 and Ramulator2.
//
// The committed reference data (validation/data/reference.csv) was produced by
// running both oracles at pinned commits over the workload in
// validation/traces/scenarios.json (see validation/run_oracles.py). These tests
// drive the *same* workload through Akita's dram.Comp and compare:
//
//   Tier 5 — command counts (activates/reads/writes), close-page count
//            scenarios, compared EXACTLY against both oracles. These quantities
//            are config- and address-map-independent, so exact match is fair.
//
//   Tier 6 — average read latency, open-page stream scenarios, compared against
//            DRAMSim3 within a 15% tolerance. Some of these deliberately probe a
//            feature Akita does NOT support (configurable address mapping):
//            when bank parallelism depends on the mapping, Akita's fixed map
//            diverges far past 15%. Those scenarios are marked "known_gap" and
//            the suite asserts the gap is *currently* large (a tracked,
//            executable record). When configurable address mapping lands and the
//            gap closes, the characterization assertion will fail — flip it to
//            "enforced" then.

const (
	tier5ScenariosPath = "validation/traces/scenarios.json"
	tier5ReferencePath = "validation/data/reference.csv"
	latencyTolerance   = 0.15
)

// tier5Pattern is the compact workload generator stored in scenarios.json.
type tier5Pattern struct {
	Op     string `json:"op"`     // "read" | "write"
	Count  int    `json:"count"`  // number of accesses
	Stride string `json:"stride"` // hex address stride, e.g. "0x2000"
}

type tier5Scenario struct {
	Name         string       `json:"name"`
	PagePolicy   string       `json:"page_policy"`
	Pattern      tier5Pattern `json:"pattern"`
	CountsCheck  string       `json:"counts_check"`
	LatencyCheck string       `json:"latency_check"`
	GapReason    string       `json:"gap_reason"`
}

// ops expands the scenario's pattern into [is_write, address] pairs, matching
// run_oracles.build_ops so Akita drives the exact workload the oracles saw.
func (s tier5Scenario) ops() [][2]uint64 {
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

type tier5Scenarios struct {
	Scenarios []tier5Scenario `json:"scenarios"`
}

type oracleResult struct {
	activates, reads, writes int
	avgReadLatency           float64 // NaN when not recorded by that oracle
}

type akitaResult struct {
	activates, reads, writes int
	avgReadLatency           float64
}

// runAkita drives the scenario through a dram.Comp configured to match the
// canonical oracle config (DDR4, refresh off, given page policy) and returns
// the issued command counts and the average read latency in DRAM cycles.
func runAkita(scn tier5Scenario) akitaResult {
	spec := DDR4Spec
	if scn.PagePolicy == "open" {
		spec.PagePolicy = PagePolicyOpen
	} else {
		spec.PagePolicy = PagePolicyClose
	}
	spec.TransactionQueueSize = 32
	spec.CommandQueueCapacity = 8
	spec.TREFI = 0 // refresh off, matching the oracle reference runs

	h := newDramHarness(spec)
	data := []byte{1, 2, 3, 4}
	for _, op := range scn.ops() {
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

// loadReference returns reference results keyed by scenario then simulator.
func loadReference() (map[string]map[string]oracleResult, bool) {
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
	tier5Scen, tier5OKScn = loadTier5Scenarios()
	tier5Ref, tier5OKRef  = loadReference()
)

// requireReference fails (not skips): the fixtures are committed and required,
// so a missing or malformed file is a breakage CI must catch, not silently pass.
func requireReference() {
	Expect(tier5OKScn).To(BeTrue(), "committed %s missing or malformed "+
		"(required; regenerate with validation/run_oracles.py)", tier5ScenariosPath)
	Expect(tier5OKRef).To(BeTrue(), "committed %s missing or malformed "+
		"(required; regenerate with validation/run_oracles.py)", tier5ReferencePath)
}

// countOracles is the set of oracles every enforced count scenario must be
// compared against (both must be present in reference.csv).
var countOracles = []string{"dramsim3", "ramulator2"}

var _ = Describe("Tier 5: command counts vs DRAMSim3 & Ramulator2", func() {
	BeforeEach(requireReference)

	It("matches the committed oracle command counts exactly", func() {
		checked := 0
		for _, scn := range tier5Scen {
			if scn.CountsCheck != "enforced" {
				continue
			}
			akita := runAkita(scn)
			ref := tier5Ref[scn.Name]
			for _, sim := range countOracles {
				want, ok := ref[sim]
				Expect(ok).To(BeTrue(),
					"%s: missing %s row in reference.csv", scn.Name, sim)
				Expect(akita.activates).To(Equal(want.activates),
					"%s: activates vs %s", scn.Name, sim)
				Expect(akita.reads).To(Equal(want.reads),
					"%s: reads vs %s", scn.Name, sim)
				Expect(akita.writes).To(Equal(want.writes),
					"%s: writes vs %s", scn.Name, sim)
			}
			checked++
		}
		Expect(checked).To(BeNumerically(">", 0))
	})
})

// dramsim3RefLatency returns the DRAMSim3 reference read latency for a
// scenario, failing if the row is missing or not a positive finite value — so a
// dropped/misspelled oracle row can't silently turn the relative-error check
// into +Inf (which would pass the known-gap assertion).
func dramsim3RefLatency(scnName string) float64 {
	row, ok := tier5Ref[scnName]["dramsim3"]
	Expect(ok).To(BeTrue(), "%s: missing dramsim3 row in reference.csv", scnName)
	ref := row.avgReadLatency
	Expect(math.IsNaN(ref)).To(BeFalse(), "%s: dramsim3 latency is not recorded", scnName)
	Expect(ref).To(BeNumerically(">", 0), "%s: dramsim3 latency must be positive", scnName)
	return ref
}

var _ = Describe("Tier 6: read latency vs DRAMSim3 (15% tolerance)", func() {
	BeforeEach(requireReference)

	It("matches DRAMSim3 read latency where Akita's model is faithful", func() {
		checked := 0
		for _, scn := range tier5Scen {
			if scn.LatencyCheck != "enforced" {
				continue
			}
			ref := dramsim3RefLatency(scn.Name)
			akita := runAkita(scn).avgReadLatency
			relErr := math.Abs(akita-ref) / ref
			Expect(relErr).To(BeNumerically("<=", latencyTolerance),
				"%s: Akita %.1f vs DRAMSim3 %.1f cyc (%.0f%% off)",
				scn.Name, akita, ref, relErr*100)
			checked++
		}
		Expect(checked).To(BeNumerically(">", 0))
	})

	// KNOWN GAPS — these probe a feature Akita does not yet support. The suite
	// records that the divergence is currently large; when the feature lands and
	// the gap closes (relErr <= 15%), this spec FAILS — that is the signal to
	// move the scenario to latency_check="enforced".
	It("records the known feature gaps (currently exceeding 15%)", func() {
		checked := 0
		for _, scn := range tier5Scen {
			if scn.LatencyCheck != "known_gap" {
				continue
			}
			ref := dramsim3RefLatency(scn.Name)
			akita := runAkita(scn).avgReadLatency
			relErr := math.Abs(akita-ref) / ref
			GinkgoWriter.Printf(
				"KNOWN GAP %-16s Akita %.1f vs DRAMSim3 %.1f cyc (%.0f%% off) — %s\n",
				scn.Name, akita, ref, relErr*100, scn.GapReason)
			Expect(relErr).To(BeNumerically(">", latencyTolerance),
				"%s gap closed (now %.0f%% off) — promote to latency_check=enforced",
				scn.Name, relErr*100)
			checked++
		}
		Expect(checked).To(BeNumerically(">", 0),
			"no known_gap scenarios ran — if all gaps are fixed, delete this spec")
	})
})
