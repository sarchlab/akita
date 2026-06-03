// Command checkpointdemo demonstrates checkpoint/resume across a quiescent phase
// boundary, and lets you inspect the on-disk archive format.
//
// A "worker" component processes one item per tick: it generates an ID, folds it
// into a running checksum, and counts items processed. It works in two batches
// with an idle (engine-quiescent) gap between them — the realistic place to
// checkpoint, e.g. between GPU kernels.
//
// Run it in two modes to see the oracle "run-to-end == checkpoint, resume,
// run-to-end" hold:
//
//	go run ./examples/checkpointdemo -mode save -ckpt /tmp/ck.tar.gz
//	go run ./examples/checkpointdemo -mode load -ckpt /tmp/ck.tar.gz
//
// Both print the same FINAL processed/checksum/time. The checksum only matches
// because phase 2's IDs continue from the restored ID-generator counter (6,7,8
// rather than 1,2,3) — that is the ID-generator checkpoint doing its job.
//
// Inspect the saved archive (no manifest; the payload files are the inventory):
//
//	tar tzf /tmp/ck.tar.gz
//	tar xzf /tmp/ck.tar.gz -C /tmp/ck && cat /tmp/ck/entities/Worker
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

// A fixed build identity keeps the demo reproducible across separate `go run`
// invocations. Real code passes "" to use checkpoint.DefaultBuildID().
const buildID = "checkpoint-demo"

const (
	batch1 = 5
	batch2 = 3
)

type workerSpec struct {
	Label string `json:"label"`
}

type workerState struct {
	Processed int    `json:"processed"`
	Pending   int    `json:"pending"`
	Checksum  uint64 `json:"checksum"`
}

// workerMW processes one pending item per tick, generating an ID and folding it
// into the checksum. When no items are pending it makes no progress, so the
// engine runs out of events and goes quiescent.
type workerMW struct {
	comp *modeling.Component[workerSpec, workerState, modeling.None]
}

func (m *workerMW) Tick() bool {
	if m.comp.State.Pending <= 0 {
		return false
	}

	id := timing.GetIDGenerator().Generate()
	m.comp.State.Processed++
	m.comp.State.Checksum = m.comp.State.Checksum*1000003 + id
	m.comp.State.Pending--

	return true
}

func main() {
	mode := flag.String("mode", "save",
		"save: run a batch, checkpoint at the quiescent boundary, finish | "+
			"load: resume from the checkpoint and finish")
	ckpt := flag.String("ckpt", "/tmp/akita-checkpoint.tar.gz", "checkpoint path")
	flag.Parse()

	sim := simulation.MakeBuilder().WithoutMonitoring().Build()
	defer func() {
		sim.Terminate()
		os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
	}()

	engine := sim.GetEngine().(*timing.SerialEngine)
	worker := modeling.NewBuilder[workerSpec, workerState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(workerSpec{Label: "demo"}).
		Build("Worker")
	worker.AddMiddleware(&workerMW{comp: worker})
	sim.RegisterComponent(worker)

	switch *mode {
	case "save":
		runBatch(engine, worker, batch1)
		report("save", "phase 1 done", engine, worker)

		if err := sim.SaveCheckpoint(*ckpt, buildID); err != nil {
			panic(err)
		}
		fmt.Printf("[save] checkpoint written to %s\n", *ckpt)

		runBatch(engine, worker, batch2)
		report("save", "FINAL", engine, worker)

	case "load":
		if err := sim.LoadCheckpoint(*ckpt, buildID); err != nil {
			panic(err)
		}
		report("load", "resumed", engine, worker)

		runBatch(engine, worker, batch2)
		report("load", "FINAL", engine, worker)

	default:
		fmt.Fprintln(os.Stderr, "mode must be 'save' or 'load'")
		os.Exit(2)
	}
}

// runBatch queues a batch of work and runs the engine until it goes quiescent.
func runBatch(
	engine *timing.SerialEngine,
	worker *modeling.Component[workerSpec, workerState, modeling.None],
	n int,
) {
	worker.State.Pending = n
	worker.TickLater()
	if err := engine.Run(); err != nil {
		panic(err)
	}
}

func report(
	mode, label string,
	engine *timing.SerialEngine,
	worker *modeling.Component[workerSpec, workerState, modeling.None],
) {
	s := worker.State
	fmt.Printf("[%s] %-12s processed=%d checksum=%d t=%d\n",
		mode, label, s.Processed, s.Checksum, engine.CurrentTime())
}
