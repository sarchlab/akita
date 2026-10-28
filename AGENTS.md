# Repository architecture

Follow [the v5 architecture rules](doc/architecture.md) when changing public APIs,
component configuration, lifecycle, routing, or package layout. Keep runtime
validation, inspector output, examples, and migration documentation aligned.

## Checkpoints

A checkpoint must restore exactly when it is loaded by the same executable on
the same platform. Across executables, for example after a code or configuration
change, loading is best effort: if it loads, it is used; otherwise loading
reports an error. Breaking checkpoint compatibility is never a concern when
changing code, and checkpoints carry no version or configuration-hash checks.
