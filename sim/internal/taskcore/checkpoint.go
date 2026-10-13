package taskcore

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// CheckpointName is reserved for the optional task service's archive payload.
const CheckpointName = "TaskTracker"

// Checkpoint exposes persistence only to simulation wiring, not on tasks.Tracker.
// Rebuilt subscriptions and port hooks are retained; observer state is not saved.
type Checkpoint struct{ Tracker *Tracker }

func (*Checkpoint) Name() string { return CheckpointName }

type savedTask struct {
	Task  Task   `json:"task"`
	Owner string `json:"owner"`
}
type savedBuffer struct {
	Port     string `json:"port"`
	MsgID    uint64 `json:"msg_id"`
	Incoming bool   `json:"incoming"`
	TaskID   uint64 `json:"task_id"`
}
type checkpointState struct {
	Tasks     []savedTask       `json:"tasks"`
	Buffers   []savedBuffer     `json:"buffers"`
	ByMessage map[uint64]uint64 `json:"by_message"`
	ByTask    map[uint64]uint64 `json:"by_task"`
}

// SaveCheckpoint records identity and correlation while the simulation is stopped.
// Task.Detail is deliberately excluded by its JSON tag; tags/milestones belong
// to observers and are not retained by the tracker.
func (c *Checkpoint) SaveCheckpoint(w io.Writer) error {
	t := c.Tracker
	t.mu.Lock()
	defer t.mu.Unlock()
	state := checkpointState{ByMessage: t.parents.byMessage, ByTask: t.parents.byTask}
	for _, e := range t.open {
		state.Tasks = append(state.Tasks, savedTask{Task: e.task, Owner: e.owner})
	}
	for key, id := range t.buffers {
		state.Buffers = append(state.Buffers, savedBuffer{
			Port: key.port, MsgID: key.msgID, Incoming: key.incoming, TaskID: id,
		})
	}
	sort.Slice(state.Tasks, func(i, j int) bool { return state.Tasks[i].Task.ID < state.Tasks[j].Task.ID })
	sort.Slice(state.Buffers, func(i, j int) bool { return state.Buffers[i].TaskID < state.Buffers[j].TaskID })
	return json.NewEncoder(w).Encode(state)
}

// LoadCheckpoint replaces lifecycle state without replaying starts or history.
// An empty object implements a checkpoint saved with tracking disabled.
func (c *Checkpoint) LoadCheckpoint(r io.Reader) error {
	var state checkpointState
	if err := json.NewDecoder(r).Decode(&state); err != nil {
		return err
	}
	t := c.Tracker
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
		open[task.ID] = &entry{task: task, owner: saved.Owner}
	}
	buffers := make(map[bufferKey]uint64, len(state.Buffers))
	for _, saved := range state.Buffers {
		key := bufferKey{saved.Port, saved.MsgID, saved.Incoming}
		e := open[saved.TaskID]
		kind := KindOutgoingBuffer
		if saved.Incoming {
			kind = KindIncomingBuffer
		}
		if !t.ports[saved.Port] || e == nil || e.task.MsgID != saved.MsgID || e.task.Kind != kind || buffers[key] != 0 {
			return fmt.Errorf("tasks: invalid checkpoint buffer %s message %d", saved.Port, saved.MsgID)
		}
		buffers[key] = saved.TaskID
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
	t.open, t.buffers = open, buffers
	t.parents = requestParents{byMessage: state.ByMessage, byTask: state.ByTask}
	return nil
}
