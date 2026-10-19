// Package queue is the task queue of akita herd hosts: the files under
// <root>/.akita-herd/ (pending, running, and done tasks, the lock, the
// agent's record, uploaded binaries), the operations that move tasks
// between them atomically, the agent that runs a host's queue, and the
// builds of binaries that tasks name.
package queue

// Dir is the directory of the queue under a host's root, and of the build
// cache in a project.
const Dir = ".akita-herd"

// BinDir holds binaries by content: bin/<sha12>/<file>.
const BinDir = "bin"
