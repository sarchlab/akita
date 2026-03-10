# Roadmap

## M1: Create `v5/queueing/` package with Buffer and Pipeline (budget: 6 cycles)

**Goal**: Create the new `v5/queueing/` package containing:
- Buffer interface, bufferImpl, NewBuffer, hook positions (from `v5/sim/buffer.go`)
- Pipeline interface, pipelineImpl, PipelineItem, Builder (from `v5/pipelining/`)
- All associated tests

Update ALL imports across the entire codebase:
- `sim.Buffer` → `queueing.Buffer`
- `sim.NewBuffer` → `queueing.NewBuffer`
- `sim.HookPosBufPush` / `sim.HookPosBufPop` → `queueing.HookPosBufPush` / `queueing.HookPosBufPop`
- `pipelining.Pipeline` → `queueing.Pipeline`
- `pipelining.PipelineItem` → `queueing.PipelineItem`
- `pipelining.MakeBuilder` → `queueing.MakePipelineBuilder` (or similar)
- Remove `v5/pipelining/` directory
- Remove buffer code from `v5/sim/`

Regenerate all mocks that reference Buffer or Pipeline.

**Status**: Not started

## M2: Merge CI actions (budget: 3 cycles)

**Goal**: Merge `akita_compile`, `lint`, and `akita_unit_test` jobs into a single job in `.github/workflows/akita_test.yml`.

**Status**: Not started

## Lessons Learned

(none yet)
