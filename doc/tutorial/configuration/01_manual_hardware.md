---
sidebar_position: 1
title: "Build hardware manually, then execute a workload"
---

Hardware is the simulation topology: components, ports, connections, and shared resources. A workload is input data plus code that loads or starts work on that hardware. Keeping them separate lets two hardware assembly styles run exactly the same experiment.

This tutorial uses the tiny wakeup-based ping component in `examples/ping`. Both agents are hardware. The ping schedule is workload data. The execution contract is concrete: `AgentA` and `AgentB` must be `*ping.Comp` values with bound `Out` ports.

## Runnable hardware example

The following is the runnable Go example from `examples/ping/example_test.go`. Run it from the repository root:

```sh
go test -run Example_manualHardware -v ./examples/ping
```

```go
package ping_test

import (
	"github.com/sarchlab/akita/v5/examples/ping"
	"github.com/sarchlab/akita/v5/examples/ping/workload"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example_manualHardware() {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()

	buffers := twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
	agentA := modeling.MakeBuilder[*ping.Comp](ping.DefaultSpec()).WithSimulation(s).Build("AgentA")
	agentB := modeling.MakeBuilder[*ping.Comp](ping.DefaultSpec()).WithSimulation(s).Build("AgentB")
	agentA.BindPort("Out", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(buffers).Build())
	agentB.BindPort("Out", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(buffers).Build())
	conn := direct.NewConnection("Conn", s, timing.GHz)
	conn.BindPort(agentA.Ports.Out)
	conn.BindPort(agentB.Ports.Out)

	if err := s.Initialize(); err != nil {
		panic(err)
	}
	workload.New(workload.DefaultSpec()).Execute(s)
	if err := s.Run(); err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
```

## A workload owns its input and execution

This shared workload lives in `examples/ping/workload/workload.go`. `Spec`, `DefaultSpec`, and `New` describe the input; `Execute` uses the ping package's own scheduling API after initialization. It does not mutate hardware Spec.

```go
// Package workload describes pings independently of hardware assembly.
package workload

import (
	"github.com/sarchlab/akita/v5/examples/ping"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec names the hardware contract and the ping schedule.
type Spec struct {
	Agent, Peer string
	SendAt      []timing.VTimeInPicoSec
}

func DefaultSpec() Spec {
	return Spec{Agent: "AgentA", Peer: "AgentB", SendAt: []timing.VTimeInPicoSec{1, 3}}
}

type Workload struct{ spec Spec }

func New(spec Spec) *Workload {
	spec.SendAt = append([]timing.VTimeInPicoSec(nil), spec.SendAt...)
	return &Workload{spec: spec}
}

// Execute loads the schedule using the ping component's own API. The contract
// is two named *ping.Comp values with bound Out ports, after Initialize.
func (w *Workload) Execute(s *sim.Simulation) {
	agent := s.Lookup(w.spec.Agent).(*ping.Comp)
	peer := s.Lookup(w.spec.Peer).(*ping.Comp)
	for _, at := range w.spec.SendAt {
		ping.SchedulePing(agent, at, peer.Ports.Out.AsRemote())
	}
}
```

## Lifecycle and the execution contract

1. Assemble or declare hardware.
2. Call `s.Initialize()` once.
3. Call `workload.New(spec).Execute(s)` to load and schedule input.
4. Call `s.Run()` to advance simulation time, then `s.Terminate()`.

`Lookup` is a Simulation instance method and uses the same runtime inventory for manual and Blueprint components, resources, ports, and connections. It panics before successful initialization or when a name is missing. A wrong concrete type panics at the caller's type assertion. There is no universal hardware start interface or execution-model registry: agreed names, concrete types, and package-specific operations are the contract.

The workload can also look up resources, such as a page table, to load initial data. Mutable execution data belongs in checkpointed State; hardware Spec remains configuration. The manual and Blueprint examples print exactly the same two lines.

`Spec`/`DefaultSpec` is a useful convention for future tools. This tutorial does not add workload discovery, a JSON loader, or JSON-to-Go generation; those remain separate work.
