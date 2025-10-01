// Package queueingv5 provides buffer and pipeline implementations for the Akita
// simulation framework.
//
// This package is part of the v5 series of Akita packages that eliminate the
// interface/implementation pattern in favor of direct struct usage. This approach
// provides better type safety, improved performance, and simplified APIs.
//
// The package includes:
//   - Buffer: A FIFO queue with capacity management and hook support
//   - Pipeline: A multi-stage pipeline processor with configurable timing
//   - PipelineBuilder: A fluent API for pipeline construction
//
// Key design principles:
//   - No interfaces - use structs directly
//   - Comprehensive hook support for simulation tracing
//   - Builder pattern for complex configurations
//   - Compatibility with existing sim package features
//
// Example usage:
//
//	buffer := queueingv5.NewBuffer("MyBuffer", 100)
//	buffer.Push("data")
//	item := buffer.Pop()
//
//	pipeline := queueingv5.NewPipelineBuilder().
//		WithNumStage(5).
//		WithCyclePerStage(2).
//		WithPostPipelineBuffer(buffer).
//		Build("MyPipeline")
package queueingv5