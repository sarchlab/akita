package pagetablemmu

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	at "github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	pf "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	pt "github.com/sarchlab/akita/v5/mem/vm/pagetableprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func newTrackedRig(t *testing.T, spec Spec) *rig {
	t.Helper()
	return newRigWithSimulation(t, sim.MakeBuilder().WithTaskTracking().Build(), spec, nil, false)
}

// Every callback runs on the fixture's serial engine.
type mmuTrace struct {
	tasks.NopObserver
	started map[uint64]tasks.Task
	ended   map[uint64]tasks.Task
	marks   map[uint64][]tasks.Milestone
}

func newMMUTrace(r *rig) *mmuTrace {
	o := &mmuTrace{started: map[uint64]tasks.Task{}, ended: map[uint64]tasks.Task{}, marks: map[uint64][]tasks.Milestone{}}
	r.s.TaskTracker().Subscribe(o, r.mmu)
	return o
}

func (o *mmuTrace) TaskStarted(t tasks.Task) { o.started[t.ID] = t }
func (o *mmuTrace) TaskEnded(t tasks.Task)   { o.ended[t.ID] = t }
func (o *mmuTrace) MilestoneAdded(m tasks.Milestone) {
	o.marks[m.TaskID] = append(o.marks[m.TaskID], m)
}

func (o *mmuTrace) request(t *testing.T, msg uint64, kind string) tasks.Task {
	t.Helper()
	for _, task := range o.started {
		if task.MsgID == msg && task.Kind == kind {
			return task
		}
	}
	t.Fatalf("missing %s task for message %d", kind, msg)
	return tasks.Task{}
}

// Verify actual child lifetimes against the intervals claimed as work and
// subtasks. Return the modeled processing phases, omitting resource waits.
func (o *mmuTrace) phases(t *testing.T, task tasks.Task) []string {
	t.Helper()
	start := task.StartTime
	var phases []string
	for _, m := range o.marks[task.ID] {
		require.Greater(t, m.Time, start, m.What)
		if m.Kind == tasks.MilestoneKindWork || m.Kind == tasks.MilestoneKindSubTask {
			found := false
			for _, child := range o.ended {
				if child.ParentID != task.ID || child.StartTime != start || child.EndTime != m.Time {
					continue
				}
				if m.Kind == tasks.MilestoneKindWork {
					require.Equal(t, tasks.KindPipeline, child.Kind)
					require.Equal(t, m.What, child.What)
				} else {
					require.Equal(t, tasks.KindReqOut, child.Kind)
				}
				found = true
			}
			require.True(t, found, "no child spans %s", m.What)
		}
		if m.Kind != tasks.MilestoneKindHardwareResource {
			phases = append(phases, string(m.Kind)+" "+m.What)
		}
		start = m.Time
	}
	return phases
}

func TestTrackedWalkUpdateAndFault(t *testing.T) {
	spec := baseSpec()
	spec.Banks, spec.VABits = spec.Banks[:1], 30
	spec.FilterBuckets, spec.LookupLatency, spec.ConsultRemote = 8, 16, true
	r := newTrackedRig(t, spec)
	r.root(r.mmu)
	o := newMMUTrace(r)
	cmd := pt.Req{Command: pt.Map, PID: 7, VAddr: 0x5000, Mapping: vm.Page{PAddr: 0x80000, DeviceID: 2}}
	mapID := r.send(r.ctrl, "Local.Control", cmd, 0)
	r.run()
	require.Empty(t, r.recv(r.ctrl).Payload.(pt.Rsp).Error)
	control := o.request(t, mapID, tasks.KindReqIn)
	require.Equal(t, []string{
		"data Local.Memory", "dependency node zeroing", "subtask publish child", "subtask write leaf"},
		o.phases(t, control))
	writes := 0
	for _, child := range o.ended {
		if child.ParentID == control.ID && child.Kind == tasks.KindReqOut && child.What == "WriteReq" {
			writes++
		}
	}
	require.Equal(t, 64+2, writes)

	leaderID := r.translate(0x5004, 3)
	engine := r.s.Engine().(*timing.SerialEngine)
	start := engine.CurrentTime()
	for i := 1; i <= 20 && r.mmu.State.Walks[0].Stage != lookupPending; i++ {
		require.NoError(t, engine.RunUntil(spec.Freq.NCyclesLater(i, start)))
	}
	require.Equal(t, lookupPending, r.mmu.State.Walks[0].Stage)
	checkRestoredLookup(t, r)
	followerID := r.translate(0x5008, 3)
	r.run()
	fault := r.recv(r.policy)
	require.Zero(t, r.policy.IncomingBuf().Size())
	leader, follower := o.request(t, leaderID, tasks.KindReqIn), o.request(t, followerID, tasks.KindReqIn)
	require.NotEqual(t, leader.ID, follower.ID)
	request := o.request(t, fault.ID, tasks.KindReqOut)
	require.Equal(t, leader.ID, request.ParentID)

	// A covering update must wait for both faulting requests to retire.
	cmd.Command = pt.Remap
	remapID := r.send(r.ctrl, "Local.Control", cmd, 0)
	r.run()
	require.Zero(t, r.ctrl.IncomingBuf().Size())
	page := fault.Payload.(pf.Req).Mapping
	r.send(r.policy, fault.Src, pf.Rsp{Result: pf.Resolved, Mapping: page}, fault.ID)
	r.run()
	for _, id := range []uint64{leaderID, followerID} {
		msg := r.recv(r.request)
		require.Equal(t, id, msg.RspTo)
		require.Equal(t, at.Rsp{Page: page}, msg.Payload)
	}
	require.Empty(t, r.recv(r.ctrl).Payload.(pt.Rsp).Error)
	require.Equal(t, []string{"work Local.filter_lookup", "work Local.prefix_lookup",
		"data Local.Memory", "data Local.Memory", "translation Local.PageFault"}, o.phases(t, leader))
	require.Equal(t, []string{"dependency Local.prefix_lookup",
		"data Local.Memory", "data Local.Memory", "translation Local.PageFault"}, o.phases(t, follower))
	require.Equal(t, []string{"dependency drain walks", "data Local.Memory", "data Local.Memory", "subtask write leaf"},
		o.phases(t, o.request(t, remapID, tasks.KindReqIn)))
	require.Equal(t, o.ended[request.ID].EndTime, o.marks[leader.ID][len(o.marks[leader.ID])-1].Time)
	for id := range o.started {
		require.Contains(t, o.ended, id, "unclosed MMU or port task")
	}
}

// Branch the run while the traced prefix lookup is still in the pipeline.
// A fresh observer sees the original child's end and the parent's work milestone.
func checkRestoredLookup(t *testing.T, r *rig) {
	t.Helper()
	w := &r.mmu.State.Walks[0]
	path := filepath.Join(t.TempDir(), "lookup.tar.gz")
	require.NoError(t, r.s.SaveCheckpoint(path))
	restored := newTrackedRig(t, r.mmu.Spec)
	require.NoError(t, restored.s.LoadCheckpoint(path))
	trace := newMMUTrace(restored)
	restored.run()
	fault := restored.recv(restored.policy)
	answer := pf.Rsp{Result: pf.Resolved, Mapping: fault.Payload.(pf.Req).Mapping}
	restored.send(restored.policy, fault.Src, answer, fault.ID)
	restored.run()
	require.Equal(t, w.Waiters[0].ID, restored.recv(restored.request).RspTo)
	child := trace.ended[w.LookupTask]
	require.Equal(t, tasks.KindPipeline, child.Kind)
	require.Equal(t, w.Waiters[0].Task, child.ParentID)
	first := trace.marks[child.ParentID][0]
	require.Equal(t, tasks.MilestoneKindWork, first.Kind)
	require.Equal(t, child.What, first.What)
	require.Equal(t, child.EndTime, first.Time)
}
