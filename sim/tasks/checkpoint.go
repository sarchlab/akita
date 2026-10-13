package tasks

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type savedTask struct {
	Task      Task   `json:"task"`
	Owner     string `json:"owner"`
	Component string `json:"component,omitempty"`
}
type checkpointState struct {
	Tasks     []savedTask       `json:"tasks"`
	ByMessage map[uint64]uint64 `json:"by_message"`
	ByTask    map[uint64]uint64 `json:"by_task"`
}

// SaveCheckpoint records identity and correlation while the simulation is stopped.
// Task.Detail is deliberately excluded by its JSON tag; tags/milestones belong
// to observers and are not retained by the tracker.
func (t *Tracker) SaveCheckpoint(w io.Writer) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := checkpointState{ByMessage: t.parents.byMessage, ByTask: t.parents.byTask}
	for _, e := range t.open {
		state.Tasks = append(state.Tasks, savedTask{Task: e.task, Owner: e.owner, Component: e.component})
	}
	sort.Slice(state.Tasks, func(i, j int) bool { return state.Tasks[i].Task.ID < state.Tasks[j].Task.ID })
	return json.NewEncoder(w).Encode(state)
}

// LoadCheckpoint replaces lifecycle state without replaying starts or history.
// Call while the rebuilt simulation is stopped, with fresh observers. Existing subscriptions are kept.
// An empty object implements a checkpoint saved with tracking disabled.
func (t *Tracker) LoadCheckpoint(r io.Reader) error {
	var state checkpointState
	if err := json.NewDecoder(r).Decode(&state); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	open := make(map[uint64]*entry, len(state.Tasks))
	for _, saved := range state.Tasks {
		task := saved.Task
		if task.ID == 0 || saved.Owner == "" || task.Kind == "" || task.What == "" || task.Location == "" {
			return fmt.Errorf("tasks: invalid checkpoint task %d", task.ID)
		}
		if open[task.ID] != nil {
			return fmt.Errorf("tasks: duplicate checkpoint task %d", task.ID)
		}
		open[task.ID] = &entry{task: task, owner: saved.Owner, component: saved.Component}
	}
	for id, msgID := range state.ByTask {
		e := open[id]
		if e == nil || e.task.MsgID != msgID || (e.task.Kind != KindReqOut && e.task.Kind != KindFlitTransport) {
			return fmt.Errorf("tasks: invalid checkpoint parent task %d", id)
		}
	}
	for msgID, id := range state.ByMessage {
		if msgID == 0 || state.ByTask[id] != msgID {
			return fmt.Errorf("tasks: invalid checkpoint message parent %d", msgID)
		}
	}
	t.open = open
	t.parents = requestParents{byMessage: state.ByMessage, byTask: state.ByTask}
	return nil
}
