// Command tasktree shows how request tasks chain into a tree across a
// component hierarchy.
//
// A Client sends one request down a memory hierarchy: Client -> L1 -> L2 ->
// Memory. Each cache "misses" and forwards the request one level down. The
// trick is the parent task id: when a cache initiates its downstream request,
// it passes the ID returned by tasks.StartReqIn as the parent — the ID
// of the req_in task it is currently handling. That makes every downstream
// task a child of the task that caused it, so the whole hierarchy forms one
// task tree. A custom tracer attached to every component prints that tree.
//
// The three component types live in the client, cache, and memory packages,
// one component per package; this file is the system builder that wires them
// together.
package main

import (
	"fmt"
	"strings"

	"github.com/sarchlab/akita/v5/examples/tasktree/cache"
	"github.com/sarchlab/akita/v5/examples/tasktree/client"
	"github.com/sarchlab/akita/v5/examples/tasktree/memory"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// --- A custom tracer that prints the task tree ---

type taskNode struct {
	parent uint64
	kind   string
	loc    string
}

type treeTracer struct {
	tasks.NopObserver
	order []uint64
	nodes map[uint64]taskNode
}

func (t *treeTracer) TaskStarted(task tasks.Task) {
	if task.Kind != tasks.KindReqIn && task.Kind != tasks.KindReqOut {
		return
	}
	if _, seen := t.nodes[task.ID]; seen {
		return
	}
	t.nodes[task.ID] = taskNode{parent: task.ParentID, kind: task.Kind, loc: task.Location}
	t.order = append(t.order, task.ID)
}

func (t *treeTracer) print() {
	children := make(map[uint64][]uint64)
	for _, id := range t.order {
		children[t.nodes[id].parent] = append(children[t.nodes[id].parent], id)
	}

	var rec func(id uint64, depth int)
	rec = func(id uint64, depth int) {
		n := t.nodes[id]
		fmt.Printf("%s%s @ %s\n", strings.Repeat("  ", depth), n.kind, n.loc)
		for _, c := range children[id] {
			rec(c, depth+1)
		}
	}

	for _, id := range t.order {
		if _, hasParent := t.nodes[t.nodes[id].parent]; !hasParent {
			rec(id, 0) // a root: its parent is not itself a recorded task
		}
	}
}

// --- Wiring ---

// buildCache builds a cache level that forwards its misses to lower, the Top
// port of the level below.
func buildCache(sim timing.Simulation, name string, lower messaging.Port) *cache.Comp {
	spec := cache.DefaultSpec()
	spec.Downstream = lower.AsRemote()

	builtComponent := modeling.MakeBuilder[*cache.Comp](spec).
		WithSimulation(sim).
		Build(name)

	builtComponent.BindPort("Top", twowaybuffered.NewPort(4, 4))
	builtComponent.BindPort("Bottom", twowaybuffered.NewPort(4, 4))
	return builtComponent
}

func main() {
	sim := akitasim.MakeBuilder().WithTaskTracking().Build()

	// Build bottom-up, so each level's Spec can name the port of the level
	// below it.
	mem := modeling.MakeBuilder[*memory.Comp](memory.DefaultSpec()).
		WithSimulation(sim).
		Build("Memory")

	mem.BindPort("Top", twowaybuffered.NewPort(4, 4))

	l2 := buildCache(sim, "L2", mem.Ports.Top)
	l1 := buildCache(sim, "L1", l2.Ports.Top)

	clientSpec := client.DefaultSpec()
	clientSpec.Dst = l1.Ports.Top.AsRemote()
	cli := modeling.MakeBuilder[*client.Comp](clientSpec).
		WithSimulation(sim).
		Build("Client")

	cli.BindPort("Out", twowaybuffered.NewPort(4, 4))

	connect := func(name string, a, b messaging.Port) {
		conn := direct.NewConnection(name, sim, timing.GHz)
		conn.BindPort(a)
		conn.BindPort(b)
	}
	connect("ConnClientL1", cli.Ports.Out, l1.Ports.Top)
	connect("ConnL1L2", l1.Ports.Bottom, l2.Ports.Top)
	connect("ConnL2Mem", l2.Ports.Bottom, mem.Ports.Top)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	tracer := &treeTracer{nodes: map[uint64]taskNode{}}
	for _, c := range []tasks.Owner{cli, l1, l2, mem} {
		c.TaskTracker().Subscribe(tracer, c)
	}

	cli.TickLater()
	if err := sim.Run(); err != nil {
		panic(err)
	}

	tracer.print()
}
