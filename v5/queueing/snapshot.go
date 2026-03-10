package queueing

// PipelineStageSnapshot captures one non-nil pipeline slot.
type PipelineStageSnapshot struct {
	Lane      int          `json:"lane"`
	Stage     int          `json:"stage"`
	Elem      PipelineItem `json:"elem"`
	CycleLeft int          `json:"cycle_left"`
}

// SnapshotPipeline returns all non-nil elements in the pipeline with their
// positions.
func SnapshotPipeline(p Pipeline) []PipelineStageSnapshot {
	impl := p.(*pipelineImpl)

	var snapshots []PipelineStageSnapshot

	for lane := 0; lane < impl.width; lane++ {
		for stage := 0; stage < impl.numStage; stage++ {
			info := impl.stages[lane][stage]
			if info.elem != nil {
				snapshots = append(snapshots, PipelineStageSnapshot{
					Lane:      lane,
					Stage:     stage,
					Elem:      info.elem,
					CycleLeft: info.cycleLeft,
				})
			}
		}
	}

	return snapshots
}

// RestorePipeline clears the pipeline and restores elements to their saved
// positions.
func RestorePipeline(p Pipeline, snapshots []PipelineStageSnapshot) {
	impl := p.(*pipelineImpl)
	impl.Clear()

	for _, snap := range snapshots {
		impl.stages[snap.Lane][snap.Stage] = pipelineStageInfo{
			elem:      snap.Elem,
			cycleLeft: snap.CycleLeft,
		}
	}
}

// SnapshotBuffer returns all elements currently in the buffer.
func SnapshotBuffer(b Buffer) []interface{} {
	impl := b.(*bufferImpl)

	result := make([]interface{}, len(impl.elements))
	copy(result, impl.elements)

	return result
}

// RestoreBuffer clears the buffer and sets its elements to the provided slice.
func RestoreBuffer(b Buffer, elements []interface{}) {
	impl := b.(*bufferImpl)

	impl.elements = make([]interface{}, len(elements))
	copy(impl.elements, elements)
}
