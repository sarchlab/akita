package queue

import (
	"testing"

	"github.com/sarchlab/akita/v5/experiment"
)

func load(name string, slots, used, pending int, avail float64) HostLoad {
	return HostLoad{Name: name, Alive: true, Slots: slots, Used: used, Pending: pending,
		MemKnown: true, MemAvailGB: avail, MemReserveGB: 8}
}

func TestPlaceScores(t *testing.T) {
	loads := []HostLoad{load("a", 16, 10, 0, 500), load("b", 16, 2, 0, 500), load("c", 16, 0, 0, 40)}
	tk := &experiment.Task{Name: "x", MemGB: 20}

	// c has the most free slots but memory for one 20 GB task; b has 14.
	if i := Place(loads, tk); loads[i].Name != "b" || loads[i].Pending != 1 {
		t.Fatalf("placed on %s", loads[i].Name)
	}

	tk.AvoidHosts = []string{"b"}
	if i := Place(loads, tk); loads[i].Name != "a" {
		t.Fatalf("avoiding b: placed on %s", loads[i].Name)
	}

	tie := []HostLoad{load("p", 4, 0, 2, 100), load("q", 4, 1, 1, 100)}
	if i := Place(tie, &experiment.Task{Name: "y"}); tie[i].Name != "q" {
		t.Fatalf("tie: placed on %s, want the one with fewer pending", tie[i].Name)
	}

	if i := Place(loads, &experiment.Task{AvoidHosts: []string{"a", "b", "c"}}); i != -1 {
		t.Fatalf("placed a task that avoids every host on %d", i)
	}
}

func TestLoadOf(t *testing.T) {
	s := &experiment.QueueStatus{
		Alive:   true,
		Agent:   &experiment.AgentInfo{Slots: 8, Own: 2, External: 5, MemAvailGB: 100, MemReserveGB: 4},
		Pending: []experiment.Task{{MemGB: 3}, {MemGB: 2}}, Running: []experiment.Task{{}, {}},
	}

	l := LoadOf("h", s, experiment.QueueConfig{})
	if l.Used != 5 || l.Pending != 2 || l.PendingMemGB != 5 || l.Free() != 1 || !l.Alive {
		t.Fatalf("load = %+v", l)
	}

	l = LoadOf("h", &experiment.QueueStatus{}, experiment.QueueConfig{Slots: 3})
	if l.Slots != 3 || l.MemKnown || l.Alive || l.Score(100) != 3 {
		t.Fatalf("load without an agent = %+v", l)
	}
}

func pendingTasks(names ...string) []experiment.Task {
	out := make([]experiment.Task, len(names))
	for i, n := range names {
		out[i] = experiment.Task{Project: "p", Name: n}
	}

	return out
}

func TestPlanBalance(t *testing.T) {
	busy := &experiment.QueueStatus{Pending: pendingTasks("w1", "w2", "w3", "w4")}
	busy.Pending[1].AvoidHosts = []string{"idle"}
	busy.Pending[2].Project = "other"
	busy.Done = []experiment.Task{
		{Project: "p", Name: "bus", Retryable: true, Signal: "SIGBUS",
			Attempts: []experiment.Attempt{{Host: "busy", Signal: "SIGBUS"}}},
		{Project: "p", Name: "done-retried", Retryable: true, Retried: "idle"},
		{Project: "p", Name: "tired", Retryable: true, Attempts: make([]experiment.Attempt, MaxRetries+1)},
	}
	idle := &experiment.QueueStatus{}
	full := &experiment.QueueStatus{Pending: pendingTasks("f1")}

	loads := []HostLoad{load("busy", 4, 4, 4, 100), load("idle", 4, 0, 0, 100), load("full", 1, 1, 1, 100)}
	loads[2].Alive = true

	retries, moves := PlanBalance(loads, []*experiment.QueueStatus{busy, idle, full}, "p")

	if len(retries) != 1 || retries[0].Task.Name != "bus" || retries[0].To != "idle" ||
		!retries[0].Task.Avoids("busy") {
		t.Fatalf("retries = %+v", retries)
	}

	// idle has 4 slots: the retry takes one, w1 and w4 two (w2 avoids idle,
	// w3 is another project's), and full's f1 the last.
	got := make([]string, 0, len(moves))
	for _, m := range moves {
		if m.To != "idle" {
			t.Errorf("move %+v", m)
		}

		got = append(got, m.From+"/"+m.Task.Name)
	}

	if len(got) != 3 || got[0] != "busy/w1" || got[1] != "busy/w4" || got[2] != "full/f1" {
		t.Fatalf("moved %v", got)
	}
}

func TestPlanBalanceSkipsDeadAndBusyTargets(t *testing.T) {
	busy := &experiment.QueueStatus{Pending: pendingTasks("w1", "w2")}
	loads := []HostLoad{load("busy", 1, 1, 2, 100), load("dead", 8, 0, 0, 100), load("queued", 8, 0, 1, 100)}
	loads[1].Alive = false
	statuses := []*experiment.QueueStatus{busy, {}, {Pending: pendingTasks("q1")}}

	if _, moves := PlanBalance(loads, statuses, "p"); len(moves) != 0 {
		t.Fatalf("moves = %+v, want none (dead agent, non-empty pending)", moves)
	}

	loads = []HostLoad{load("busy", 1, 1, 2, 100), load("small", 8, 0, 0, 10)}
	busy.Pending[0].MemGB = 5

	if _, moves := PlanBalance(loads, []*experiment.QueueStatus{busy, {}}, "p"); len(moves) != 1 ||
		moves[0].Task.Name != "w2" {
		t.Fatalf("moves = %+v, want only w2 (w1 does not fit the memory)", moves)
	}
}
