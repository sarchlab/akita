package hostsys

import (
	"context"
	"os/exec"
	"strings"
)

// SlurmJobState asks squeue, then sacct, about a job of this machine's
// cluster, each with commandTimeout. It returns "queued" (pending),
// "running", or "" when the job is not in the queue, and then sacct's
// final state (e.g. "TIMEOUT") when sacct knows it. Without squeue it
// returns "", "".
func SlurmJobState(ctx context.Context, id string) (state, final string) {
	if _, err := exec.LookPath("squeue"); err != nil {
		return "", ""
	}

	if s := slurmState(runQuiet(ctx, "squeue", "-h", "-j", id, "-o", "%T")); s != "" {
		return s, ""
	}

	out := runQuiet(ctx, "sacct", "-n", "-X", "-P", "-j", id, "-o", "State")
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")

	return "", strings.TrimSpace(line)
}

// slurmState maps squeue's %T of a job to "queued", "running", or "".
func slurmState(s string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	switch strings.TrimSpace(first) {
	case "PENDING", "CONFIGURING", "REQUEUED", "REQUEUE_HOLD", "REQUEUE_FED", "RESV_DEL_HOLD":
		return "queued"
	case "RUNNING", "COMPLETING", "SUSPENDED", "STOPPED", "SIGNALING", "STAGE_OUT", "RESIZING":
		return "running"
	default:
		return ""
	}
}
