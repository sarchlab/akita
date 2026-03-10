# Roadmap

## Project Goal

Redefine the component model in Akita V5 following the Spec/State/Ports/Middlewares pattern described in `migration.md`. Create a `modeling` package, refactor builders to use `WithSpec`, implement simulation save/load (serialization), and create an acceptance test for save/load.

## Milestones

### M1: Create `modeling` package with Component struct (Spec/State pattern) — Budget: 6 cycles
**Status:** ✅ Complete (PR #10 merged to main)

Delivered: `v5/modeling/` package with generic `Component[S, T]`, `Builder`, `ValidateSpec`, `ValidateState`.

### M2: Refactor `idealmemcontroller` to use the modeling package — Budget: 6 cycles
**Status:** ✅ Complete (PR #11 merged to ares/m1-modeling-package)

Delivered: `idealmemcontroller` uses `modeling.Component[Spec, State]`, tick-driven countdowns, `WithSpec()` builder, all tests pass.

### M3: Implement simulation Save/Load — Budget: TBD (pending analysis)
**Status:** Planning

Add `Save(filename string)` and `Load(filename string)` to the simulation. Requires:
- Serializing component Specs and States
- Serializing engine time and tick scheduler state
- Serializing port buffer contents (messages in transit)
- Serializing storage data
- Saving/restoring ID generator state
- Handling message interface serialization (type registry)

### M4: Save/Load acceptance test — Budget: TBD (pending M3 scope)
**Status:** Not started

## Lessons Learned

- M1 took 4 cycles (budget 6) — modeling package was well-scoped
- M2 took 6 cycles (budget 6) — on target but needed fix round for verification failures
- Verification catches real issues: StorageRef, AddrConvKind, CurrentCmdID were missing from initial implementation
- Workers need explicit constraints about what NOT to modify
