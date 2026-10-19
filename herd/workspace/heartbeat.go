package workspace

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

// maxHeartbeatBytes bounds the heartbeat file read.
const maxHeartbeatBytes = 20000

// ReadHeartbeat reads logs/<name>.heartbeat, a JSON object that a running
// program rewrites; nil when there is none.
func ReadHeartbeat(dir, name string) map[string]any {
	return hostsys.ReadJSONMap(SessionFile(dir, name, HeartbeatExt), maxHeartbeatBytes)
}

// Heartbeat is a heartbeat's fields that the tool uses:
// {"pid": 123, "wall": <unix s>, "finished": false, "sim_time_ns": 1.5e6,
// "progress": [Progress...], "args": [...]}, and, for compatibility,
// "wgs_completed" with "-max-wg" in args.
type Heartbeat struct {
	PID          *float64 // the process that writes it
	Wall         *float64 // when it was written
	Finished     bool
	SimTimeNS    float64
	WGsCompleted *float64 // legacy work counter
	Args         []string

	// Bars is "progress" when it decodes as progress bars (HasBars).
	Bars    []herd.Progress
	HasBars bool
	// barsFinished and barsTotal sum "progress"'s finished and total.
	barsFinished float64
	barsTotal    float64
}

// ParseHeartbeat reads the fields of a heartbeat; nil for nil.
func ParseHeartbeat(hb map[string]any) *Heartbeat {
	if hb == nil {
		return nil
	}

	h := &Heartbeat{}
	h.PID = number(hb["pid"])
	h.Wall = number(hb["wall"])
	h.Finished, _ = hb["finished"].(bool)
	h.SimTimeNS, _ = hb["sim_time_ns"].(float64)
	h.WGsCompleted = number(hb["wgs_completed"])

	raw, _ := hb["args"].([]any)
	for _, a := range raw {
		if s, ok := a.(string); ok {
			h.Args = append(h.Args, s)
		}
	}

	if p, ok := hb["progress"]; ok {
		data, err := json.Marshal(p)
		h.HasBars = err == nil && json.Unmarshal(data, &h.Bars) == nil
	}

	list, _ := hb["progress"].([]any)
	for _, item := range list {
		bar, _ := item.(map[string]any)
		f, _ := bar["finished"].(float64)
		t, _ := bar["total"].(float64)
		h.barsFinished += f
		h.barsTotal += t
	}

	return h
}

func number(v any) *float64 {
	f, ok := v.(float64)
	if !ok {
		return nil
	}

	return &f
}

// MaxWG is the "-max-wg" value of the heartbeat's arguments, or 0.
func (h *Heartbeat) MaxWG() int {
	v, ok := FlagValue(h.Args, "-max-wg")
	if !ok {
		return 0
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}

	return n
}

// Work returns the completed work of a heartbeat and its total: the legacy
// wgs_completed with -max-wg, else the sums of its progress bars.
func (h *Heartbeat) Work() (work, total float64) {
	if h.WGsCompleted != nil {
		return *h.WGsCompleted, float64(h.MaxWG())
	}

	return h.barsFinished, h.barsTotal
}

// FlagValue finds "-flag=value" or "-flag value" in a command line.
func FlagValue(args []string, flag string) (string, bool) {
	if flag == "" {
		return "", false
	}

	for i, a := range args {
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			return v, true
		}

		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}

	return "", false
}
