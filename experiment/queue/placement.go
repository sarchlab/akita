package queue

import (
	"math"
	"slices"

	"github.com/sarchlab/akita/v5/experiment"
)

// MaxRetries is how many times balance resubmits a retryable task.
const MaxRetries = 2

// HostLoad is what placement knows of a host's queue.
type HostLoad struct {
	Name         string
	Alive        bool // its agent runs
	Slots        int
	Used         int // slots in use: own tasks, or simulators found when more
	Pending      int
	PendingMemGB float64
	MemKnown     bool
	MemAvailGB   float64
	MemReserveGB float64
}

// LoadOf is the load of a host from its queue status; without an agent
// record the host's settings give the slots and the memory is unknown.
func LoadOf(name string, s *experiment.QueueStatus, cfg experiment.QueueConfig) HostLoad {
	l := HostLoad{Name: name}

	if s != nil && s.Agent != nil {
		a := s.Agent
		l.Alive, l.Slots, l.Used = s.Alive, a.Slots, max(a.Own, a.External)
		l.MemKnown, l.MemAvailGB, l.MemReserveGB = true, a.MemAvailGB, a.MemReserveGB
	} else {
		c := Config{QueueConfig: cfg}.WithDefaults()
		l.Slots, l.MemReserveGB = c.Slots, c.MemReserveGB
	}

	if s != nil {
		l.Used = max(l.Used, len(s.Running))
		for _, t := range s.Pending {
			l.Pending++
			l.PendingMemGB += t.MemGB
		}
	}

	return l
}

// Free is the number of slots left after the running and pending tasks.
func (l HostLoad) Free() int { return l.Slots - l.Used - l.Pending }

// fitsMem tells whether a task of memGB fits the memory left after the
// pending tasks and the reserve (true when the memory is unknown).
func (l HostLoad) fitsMem(memGB float64) bool {
	return !l.MemKnown || l.MemAvailGB-l.PendingMemGB-memGB >= l.MemReserveGB
}

// Score is how many more tasks like this one the host can start now: the
// free slots, or fewer when the memory left (after the reserve and the
// pending tasks) holds fewer.
func (l HostLoad) Score(memGB float64) float64 {
	score := float64(l.Free())
	if l.MemKnown {
		room := (l.MemAvailGB - l.MemReserveGB - l.PendingMemGB) / math.Max(memGB, 1)
		score = math.Min(score, math.Floor(room))
	}

	return score
}

// add counts a task placed on the host.
func (l *HostLoad) add(memGB float64) {
	l.Pending++
	l.PendingMemGB += memGB
}

// Place picks the host for a task among loads (not one it avoids, nor one
// in skip): the highest score, then the fewest pending tasks, then the
// first. It counts the task on that host and returns its index, or -1.
func Place(loads []HostLoad, t *experiment.Task, skip ...string) int {
	best := -1

	for i, l := range loads {
		if t.Avoids(l.Name) || slices.Contains(skip, l.Name) {
			continue
		}

		if best < 0 {
			best = i
			continue
		}

		b := loads[best]
		if s, bs := l.Score(t.MemGB), b.Score(t.MemGB); s > bs || (s == bs && l.Pending < b.Pending) {
			best = i
		}
	}

	if best >= 0 {
		loads[best].add(t.MemGB)
	}

	return best
}

// Move is a task that balance moves (or resubmits) from one host to
// another.
type Move struct {
	Task experiment.Task
	From string
	To   string
}

// PlanBalance plans one round of balance over the hosts of a project:
// first, retryable ended tasks (not yet retried, at most MaxRetries times)
// go to the best other host whose agent runs and that they do not avoid;
// then pending tasks of busy hosts move, in queue order, to hosts whose
// agent runs, whose pending/ is empty, and that have free slots and
// memory for them. statuses[i] belongs to loads[i] (nil if unreachable).
func PlanBalance(loads []HostLoad, statuses []*experiment.QueueStatus, project string) (retries, moves []Move) {
	var (
		dead    []string
		targets []int
	)

	for j, l := range loads {
		if !l.Alive {
			dead = append(dead, l.Name)
		} else if l.Pending == 0 && l.Free() > 0 {
			targets = append(targets, j)
		}
	}

	for i, s := range statuses {
		if s == nil {
			continue
		}

		for _, t := range s.Done {
			if t.Project != project || !t.Retryable || t.Retried != "" || len(t.Attempts) > MaxRetries {
				continue
			}

			r := t
			r.AvoidHosts = appendName(r.AvoidHosts, loads[i].Name)

			if j := Place(loads, &r, dead...); j >= 0 {
				retries = append(retries, Move{Task: r, From: loads[i].Name, To: loads[j].Name})
			}
		}
	}

	return retries, planMoves(loads, statuses, targets, project)
}

func planMoves(loads []HostLoad, statuses []*experiment.QueueStatus, targets []int, project string) []Move {
	var moves []Move

	for i, s := range statuses {
		if s == nil || loads[i].Pending == 0 || loads[i].Free() >= 0 {
			continue
		}

		for _, t := range s.Pending {
			if t.Project != project || loads[i].Free() >= 0 {
				continue
			}

			if j := pickTarget(loads, targets, &t); j >= 0 {
				loads[j].add(t.MemGB)
				loads[i].Pending--
				loads[i].PendingMemGB -= t.MemGB
				moves = append(moves, Move{Task: t, From: loads[i].Name, To: loads[j].Name})
			}
		}
	}

	return moves
}

func pickTarget(loads []HostLoad, targets []int, t *experiment.Task) int {
	best := -1

	for _, j := range targets {
		l := loads[j]
		if t.Avoids(l.Name) || l.Free() <= 0 || !l.fitsMem(t.MemGB) {
			continue
		}

		if best < 0 || l.Score(t.MemGB) > loads[best].Score(t.MemGB) {
			best = j
		}
	}

	return best
}

func appendName(names []string, n string) []string {
	if slices.Contains(names, n) {
		return names
	}

	return append(append([]string(nil), names...), n)
}
