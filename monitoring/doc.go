// Package monitoring provides a solution that allows monitoring simulation
// externally.
package monitoring

//go:generate protoc -I=./ --go_out=./ ./profile.proto
