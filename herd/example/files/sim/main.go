// sim is a fake simulator: it sleeps for -steps seconds and writes made-up
// metrics.
//
// REPLACE: your simulator. Herd needs nothing from it; two things are
// optional:
//   - progress: rewrite the JSON file named by $HERD_HEARTBEAT (the block
//     marked "heartbeat" below) to get a progress bar in the web UI;
//   - results: write into results/<run>/ (here, the directory two levels
//     above -metrics), so that pull, collect, and the web UI find them.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	workload := flag.String("workload", "fir", "workload")
	config := flag.String("config", "base", "configuration")
	steps := flag.Int("steps", 5, "seconds to run")
	metrics := flag.String("metrics", "", "results/<run>/stats/metrics.csv")
	flag.Parse()

	start := time.Now().Unix()
	for i := 0; i <= *steps; i++ {
		heartbeat(*workload, start, i, *steps)
		fmt.Printf("%s/%s: step %d of %d\n", *workload, *config, i, *steps)

		if i < *steps {
			time.Sleep(time.Second)
		}
	}

	if *metrics != "" {
		writeResults(*metrics, *workload, *config)
	}
}

// beat is the heartbeat file; Progress holds the bars.
type beat struct {
	PID      int   `json:"pid"`
	Wall     int64 `json:"wall"`
	Finished bool  `json:"finished"`
	Progress []bar `json:"progress"`
}

// bar is one progress bar: Finished of Total.
type bar struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartTime int64  `json:"start_time"`
	Total     int    `json:"total"`
	Finished  int    `json:"finished"`
}

// heartbeat reports progress: one bar, step of steps.
func heartbeat(workload string, start int64, step, steps int) {
	hb := os.Getenv("HERD_HEARTBEAT")
	if hb == "" {
		return
	}

	data, err := json.Marshal(beat{
		PID: os.Getpid(), Wall: time.Now().Unix(), Finished: step == steps,
		Progress: []bar{{ID: "steps", Name: workload, StartTime: start, Total: steps, Finished: step}},
	})
	if err != nil {
		return
	}

	_ = os.WriteFile(hb+".tmp", data, 0o644)
	_ = os.Rename(hb+".tmp", hb)
}

// writeResults writes the made-up total_time: the metrics file, and
// summary.csv, run.json, and meta.json in the run directory.
func writeResults(metrics, workload, config string) {
	t := map[string]float64{"fir": 3, "mm": 8, "bfs": 5}[workload]
	if config == "fast" {
		t /= map[string]float64{"fir": 1.2, "mm": 2, "bfs": 1.5}[workload]
	}

	run := filepath.Dir(filepath.Dir(metrics))
	_ = os.MkdirAll(filepath.Dir(metrics), 0o755)
	_ = os.WriteFile(metrics, fmt.Appendf(nil, ",where,what,value\n0,driver,total_time,%g\n", t), 0o644)
	_ = os.WriteFile(filepath.Join(run, "summary.csv"),
		fmt.Appendf(nil, "workload,config,total_time\n%s,%s,%g\n", workload, config, t), 0o644)
	_ = os.WriteFile(filepath.Join(run, "run.json"),
		fmt.Appendf(nil, `{"metrics": {"total_time": %g}}`+"\n", t), 0o644)
	_ = os.WriteFile(filepath.Join(run, "meta.json"), []byte(`{"exit_code": 0}`+"\n"), 0o644)
}
