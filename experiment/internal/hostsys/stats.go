package hostsys

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// maxHTTPBytes bounds the body HTTPGet reads.
const maxHTTPBytes = 4 << 20

// nvidiaColumns are the columns of the nvidia-smi query: name,
// utilization, memory used, memory total.
const nvidiaColumns = 4

// Stats are a host's statistics.
type Stats struct {
	Load     []float64 // 1, 5, and 15 minute load averages
	CPUs     int
	MemTotal uint64 // bytes
	MemAvail uint64 // bytes
	GPUs     []GPU
}

// GPU is an NVIDIA GPU as nvidia-smi reports it.
type GPU struct {
	Name        string
	Util        float64 // percent
	MemUsedMiB  float64
	MemTotalMiB float64
}

// HostStats reads the load, CPU count, memory, and GPUs of this machine.
func HostStats(ctx context.Context) Stats {
	s := Stats{CPUs: runtime.NumCPU(), Load: []float64{0, 0, 0}}

	if avg, err := load.AvgWithContext(ctx); err == nil {
		s.Load = []float64{avg.Load1, avg.Load5, avg.Load15}
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		s.MemTotal, s.MemAvail = vm.Total, vm.Available
	}

	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		s.GPUs = parseNvidiaSMI(runQuiet(ctx, "nvidia-smi",
			"--query-gpu=name,utilization.gpu,memory.used,memory.total",
			"--format=csv,noheader,nounits"))
	}

	return s
}

func parseNvidiaSMI(out string) []GPU {
	var gpus []GPU

	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, ",")
		if len(parts) != nvidiaColumns {
			continue
		}

		var nums [3]float64

		ok := true

		for i := range nums {
			v, err := strconv.ParseFloat(strings.TrimSpace(parts[i+1]), 64)
			if err != nil {
				ok = false
				break
			}

			nums[i] = v
		}

		if ok {
			gpus = append(gpus, GPU{
				Name: strings.TrimSpace(parts[0]), Util: nums[0], MemUsedMiB: nums[1], MemTotalMiB: nums[2],
			})
		}
	}

	return gpus
}

// HTTPGet fetches a URL with a timeout and returns at most 4 MB of its
// body; a status other than 200 is an error.
func HTTPGet(ctx context.Context, url string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}

	return io.ReadAll(io.LimitReader(resp.Body, maxHTTPBytes))
}
