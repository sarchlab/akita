package saveload_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
)

const (
	maxAddress = 4096
	seed       = 42
)

// buildSimulation creates a Simulation with an idealmemcontroller and a
// MemAccessAgent connected via DirectConnection.
func buildSimulation(
	t *testing.T,
	writeLeft, readLeft int,
	rng *rand.Rand,
) (*simulation.Simulation, *memaccessagent.MemAccessAgent, *idealmemcontroller.Comp) {
	t.Helper()

	s := simulation.MakeBuilder().WithoutMonitoring().Build()
	engine := s.GetEngine()

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	agent := memaccessagent.MakeBuilder().
		WithEngine(engine).
		WithMaxAddress(maxAddress).
		WithWriteLeft(writeLeft).
		WithReadLeft(readLeft).
		WithMemPort(sim.NewPort(nil, 4, 4, "Agent.Mem")).
		Build("Agent")
	agent.Rand = rng

	dram := idealmemcontroller.MakeBuilder().
		WithEngine(engine).
		WithNewStorage(4 * mem.MB).
		WithSpec(idealmemcontroller.Spec{
			Width:         1,
			Latency:       10,
			CacheLineSize: 64,
		}).
		WithTopPort(sim.NewPort(nil, 16, 16, "DRAM.TopPort")).
		WithCtrlPort(sim.NewPort(nil, 16, 16, "DRAM.CtrlPort")).
		Build("DRAM")
	agent.LowModule = dram.GetPortByName("Top")

	conn.PlugIn(agent.GetPortByName("Mem"))
	conn.PlugIn(dram.GetPortByName("Top"))

	s.RegisterComponent(agent)
	s.RegisterComponent(dram)

	return s, agent, dram
}

// agentResult captures the final observable state of a MemAccessAgent.
type agentResult struct {
	WriteLeft     int
	ReadLeft      int
	KnownMemValue map[uint64][]uint32
}

func captureResult(agent *memaccessagent.MemAccessAgent) agentResult {
	kmv := make(map[uint64][]uint32, len(agent.KnownMemValue))
	for k, v := range agent.KnownMemValue {
		dup := make([]uint32, len(v))
		copy(dup, v)
		kmv[k] = dup
	}

	return agentResult{
		WriteLeft:     agent.WriteLeft,
		ReadLeft:      agent.ReadLeft,
		KnownMemValue: kmv,
	}
}

func resultsEqual(a, b agentResult) bool {
	if a.WriteLeft != b.WriteLeft || a.ReadLeft != b.ReadLeft {
		return false
	}

	if len(a.KnownMemValue) != len(b.KnownMemValue) {
		return false
	}

	for k, va := range a.KnownMemValue {
		vb, ok := b.KnownMemValue[k]
		if !ok || len(va) != len(vb) {
			return false
		}

		for i := range va {
			if va[i] != vb[i] {
				return false
			}
		}
	}

	return true
}

func cleanupSim(s *simulation.Simulation) {
	path := "akita_sim_" + s.ID() + ".sqlite3"
	os.Remove(path)
}

func runEngine(t *testing.T, s *simulation.Simulation, phase string) {
	t.Helper()

	err := s.GetEngine().(*sim.SerialEngine).Run()
	if err != nil {
		t.Fatalf("%s run failed: %v", phase, err)
	}
}

// runPhaseA runs the first batch of writes and saves a checkpoint.
func runPhaseA(
	t *testing.T,
) (*simulation.Simulation, *memaccessagent.MemAccessAgent, string) {
	t.Helper()

	rngA := rand.New(rand.NewSource(seed))
	sA, agentA, _ := buildSimulation(t, 50, 0, rngA)
	agentA.TickLater()

	runEngine(t, sA, "Phase A")

	t.Logf("Phase A done: WriteLeft=%d ReadLeft=%d keys=%d engineTime=%v idNext=%d",
		agentA.WriteLeft, agentA.ReadLeft, len(agentA.KnownMemValue),
		sA.GetEngine().CurrentTime(), sim.GetIDGeneratorNextID())

	checkpointDir := t.TempDir()

	err := sA.Save(checkpointDir)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	return sA, agentA, checkpointDir
}

// runPhaseB continues the original simulation after the checkpoint.
func runPhaseB(
	t *testing.T,
	sA *simulation.Simulation,
	agentA *memaccessagent.MemAccessAgent,
) agentResult {
	t.Helper()

	rngB := rand.New(rand.NewSource(seed + 1))
	agentA.Rand = rngB
	agentA.WriteLeft = 50
	agentA.ReadLeft = 100
	agentA.TickLater()

	runEngine(t, sA, "Phase B")

	result := captureResult(agentA)
	t.Logf("Phase B done: WriteLeft=%d ReadLeft=%d keys=%d",
		result.WriteLeft, result.ReadLeft, len(result.KnownMemValue))

	sA.Terminate()
	cleanupSim(sA)

	if result.WriteLeft != 0 || result.ReadLeft != 0 {
		t.Fatalf("Original sim didn't complete: W=%d R=%d",
			result.WriteLeft, result.ReadLeft)
	}

	return result
}

// runPhaseC builds a new simulation, loads the checkpoint, and runs the
// same continuation to verify determinism.
func runPhaseC(
	t *testing.T,
	checkpointDir string,
) agentResult {
	t.Helper()

	sim.ResetIDGenerator()
	sim.UseSequentialIDGenerator()

	rngC := rand.New(rand.NewSource(seed + 1))
	sC, agentC, _ := buildSimulation(t, 50, 0, rngC)
	defer func() {
		sC.Terminate()
		cleanupSim(sC)
	}()

	err := sC.Load(checkpointDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Logf("Phase C loaded: WriteLeft=%d ReadLeft=%d keys=%d idNext=%d engineTime=%v",
		agentC.WriteLeft, agentC.ReadLeft, len(agentC.KnownMemValue),
		sim.GetIDGeneratorNextID(), sC.GetEngine().CurrentTime())

	agentC.WriteLeft = 50
	agentC.ReadLeft = 100
	agentC.TickLater()

	runEngine(t, sC, "Phase C")

	result := captureResult(agentC)
	t.Logf("Phase C done: WriteLeft=%d ReadLeft=%d keys=%d",
		result.WriteLeft, result.ReadLeft, len(result.KnownMemValue))

	if result.WriteLeft != 0 || result.ReadLeft != 0 {
		t.Fatalf("Loaded sim didn't complete: W=%d R=%d",
			result.WriteLeft, result.ReadLeft)
	}

	return result
}

func reportDiff(t *testing.T, orig, loaded agentResult) {
	t.Helper()

	if orig.WriteLeft != loaded.WriteLeft {
		t.Errorf("WriteLeft: orig=%d loaded=%d",
			orig.WriteLeft, loaded.WriteLeft)
	}

	if orig.ReadLeft != loaded.ReadLeft {
		t.Errorf("ReadLeft: orig=%d loaded=%d",
			orig.ReadLeft, loaded.ReadLeft)
	}

	if len(orig.KnownMemValue) != len(loaded.KnownMemValue) {
		t.Errorf("KnownMemValue keys: orig=%d loaded=%d",
			len(orig.KnownMemValue), len(loaded.KnownMemValue))
	}

	diffCount := 0

	for k, vo := range orig.KnownMemValue {
		vl, ok := loaded.KnownMemValue[k]
		if !ok {
			t.Errorf("Key 0x%X in original but not in loaded", k)
			diffCount++
		} else if len(vo) != len(vl) {
			t.Errorf("Key 0x%X: orig len=%d loaded len=%d", k, len(vo), len(vl))
			diffCount++
		}

		if diffCount > 10 {
			t.Error("... (truncated)")
			break
		}
	}

	t.Fatal("Results do not match between original and loaded simulation")
}

// TestSaveLoadDeterminism verifies that saving and loading produces
// deterministic results.
//
// Strategy:
//   - Phase A: Run sim with 50 writes, 0 reads → run to completion → save
//   - Phase B: Set additional work (50 writes, 100 reads), new rand seed → run to completion → record result
//   - Phase C: Build NEW sim, load checkpoint, set same additional work + seed → run to completion → compare
func TestSaveLoadDeterminism(t *testing.T) {
	sim.ResetIDGenerator()
	sim.UseSequentialIDGenerator()

	sA, agentA, checkpointDir := runPhaseA(t)
	resultOriginal := runPhaseB(t, sA, agentA)
	resultLoaded := runPhaseC(t, checkpointDir)

	if !resultsEqual(resultOriginal, resultLoaded) {
		reportDiff(t, resultOriginal, resultLoaded)
	}

	t.Log("Deterministic save/load test passed!")
}
