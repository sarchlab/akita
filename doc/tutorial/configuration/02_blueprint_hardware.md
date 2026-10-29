---
sidebar_position: 2
title: "Declare the same hardware with Blueprint"
---

This tutorial runs the same ping workload as [the manual hardware tutorial](./01_manual_hardware.md). Agent names, port capacities, direct-connection binding order, and output remain identical. Only hardware assembly changes.

A Blueprint domain declares nodes; it must not invoke live builders. `Add` snapshots a locally named node. `Initialize` expands copies into absolute names, resolves references and exposed ports, then constructs and validates the runtime topology.

## Runnable hardware example

The following is the runnable Go example from `examples/ping/blueprint_example_test.go`. Run it from the repository root:

```sh
go test -run Example_blueprintHardware -v ./examples/ping
```

```go
package ping_test

import (
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/examples/ping"
	"github.com/sarchlab/akita/v5/examples/ping/workload"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example_blueprintHardware() {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()

	s.Add("", blueprint.NewDomain[modeling.None, modeling.None](modeling.None{},
		func(_ modeling.None, bp *blueprint.Blueprint) {
			buffers := twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
			bp.Add("AgentA", blueprint.Comp[*ping.Comp](ping.DefaultSpec()).Port("Out", buffers))
			bp.Add("AgentB", blueprint.Comp[*ping.Comp](ping.DefaultSpec()).Port("Out", buffers))
			bp.Connect("Conn", timing.GHz, bp.PortRef("AgentA.Out"), bp.PortRef("AgentB.Out"))
		}))

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

## References and exported ports

`bp.PortRef("AgentB.Out")` is relative to the current domain declaration environment, even if the target is declared later. It immediately produces an absolute address; expansion resolves exposed aliases before builders run. A reference passed into a child Spec is already absolute and passes through unchanged. Do not pass the absolute path returned by `Add` back to `PortRef`.

For a reusable nested domain, declare its static `Ports` type and use `bp.Expose("Public", "Inner.Out")` to bind each public port. All single ports and every group element must be exposed. Expansion retains only flat bindings to actual ports; after initialization, `s.Lookup("Domain").(domain.Ports)` retrieves that exported Ports value. See [Blueprint assembly details](https://github.com/sarchlab/akita/blob/main/blueprint/README.md) for nested aliases, resources, and connectors.

## Lifecycle and the execution contract

1. Assemble or declare hardware.
2. Call `s.Initialize()` once.
3. Call `workload.New(spec).Execute(s)` to load and schedule input.
4. Call `s.Run()` to advance simulation time, then `s.Terminate()`.

`Lookup` is a Simulation instance method and uses the same runtime inventory for manual and Blueprint components, resources, ports, and connections. It panics before successful initialization or when a name is missing. A wrong concrete type panics at the caller's type assertion. There is no universal hardware start interface or execution-model registry: agreed names, concrete types, and package-specific operations are the contract.

The workload can also look up resources, such as a page table, to load initial data. Mutable execution data belongs in checkpointed State; hardware Spec remains configuration. The manual and Blueprint examples print exactly the same two lines.

`Spec`/`DefaultSpec` is a useful convention for future tools. This tutorial does not add workload discovery, a JSON loader, or JSON-to-Go generation; those remain separate work.
