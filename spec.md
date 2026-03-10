# Project Spec

## What to Build

We need to redefine the component. 

Check /v5/migration.md, under section "Defining Components in V5: Philosophy and Patterns".

We need to redefine a component as a combination of spec, state, ports, and middlewares. Spec and state are easily serializable. Ports and middlewares are containers of dependencies. 

Create a new modeling package. Define a `Component` struct there. In v5, a concrete struct is no longer a struct, but an instance of a component. The package only provides the specs, state, middlewares, and builder.  

Builders should avoid `with` functions for basic parameters. Instead, builders should have a `withSpec` method to take a spec struct.

We need to consider the serialization requirement. In the simulation package, the simulation struct should have a save and load function, which takes a file name. They can save and load the current state of a simulation.

## Success Criteria

Design simple, straightforward, intuitive APIs. 

Create an acceptance test for the save/load process. You can use the memory acceptance tests as a starting point.
