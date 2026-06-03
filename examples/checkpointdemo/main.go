// Command checkpoint builds a tiny simulation, gives it some runtime state, and
// writes a checkpoint archive to disk. It exists so you can produce a real
// checkpoint file and inspect the on-disk format:
//
//	go run ./examples/checkpointdemo --out /tmp/akita-checkpoint.tar.gz
//	tar tzf /tmp/akita-checkpoint.tar.gz            # list entries
//	tar xzf /tmp/akita-checkpoint.tar.gz -C /tmp    # extract
//	cat /tmp/build_id; echo
//	cat /tmp/entities/Comp        # generic component: spec hash + state (JSON)
//	cat /tmp/entities/IDGenerator # ID generator kind + counter (JSON)
//	cat /tmp/entities/Engine      # serial engine current time (JSON)
//	xxd /tmp/entities/Mem | head  # storage: compact binary (shape + units)
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

type demoSpec struct {
	Latency int `json:"latency"`
}

type demoState struct {
	Counter int      `json:"counter"`
	Log     []string `json:"log"`
}

func main() {
	out := flag.String("out", "akita-checkpoint.tar.gz", "checkpoint output path")
	flag.Parse()

	sim := simulation.MakeBuilder().WithoutMonitoring().Build()
	defer func() {
		sim.Terminate()
		os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
	}()

	engine := sim.GetEngine().(*timing.SerialEngine)

	// A port-less component plus a storage resource: every registered entity
	// (Engine, IDGenerator, Comp, Mem) has a checkpoint serializer.
	comp := modeling.NewBuilder[demoSpec, demoState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(demoSpec{Latency: 5}).
		Build("Comp")
	sim.RegisterComponent(comp)

	storage := mem.MakeStorageBuilder().
		WithCapacity(4 * mem.KB).
		WithSimulation(sim).
		Build("Mem")

	// Give every entity some runtime state worth saving.
	comp.State = demoState{Counter: 42, Log: []string{"hello", "world"}}
	if err := storage.Write(0, []byte("checkpoint demo data")); err != nil {
		panic(err)
	}
	for i := 0; i < 7; i++ {
		timing.GetIDGenerator().Generate()
	}
	engine.SetCurrentTime(1234)

	// Pass "" to use the default (same-binary) build identity.
	if err := sim.SaveCheckpoint(*out, ""); err != nil {
		panic(err)
	}

	info, _ := os.Stat(*out)
	fmt.Printf("wrote checkpoint to %s (%d bytes)\n", *out, info.Size())
	fmt.Println("inspect with: tar tzf", *out)
}
