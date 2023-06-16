package analysis

import "github.com/sarchlab/akita/v3/sim"

// A Task is a task
type BufferTask struct {
	StartTime sim.VTimeInSec `json:"start_time"`
	EndTime   sim.VTimeInSec `json:"end_time"`

	ID           string       `json:"id"`
	BufferDetail interface{}  `json:"-"`
	BufferSteps  []BufferStep `json:"-"`
}

type bufferFilter func(b BufferTask) bool

type BufferStep struct {
	Time                    sim.VTimeInSec `json:"time"`
	LastBufLevel            int            `json:"last_buffer_level"`
	AverageBufLevel         float64        `json:"average_buffer_level"`
	AverageBufLevelInPeriod float64        `json:"average_buffer_level_in_period"`
}

type PortTask struct {
	StartTime sim.VTimeInSec `json:"start_time"`
	EndTime   sim.VTimeInSec `json:"end_time"`

	PortName   string      `json:"port_name"`
	Period     float64     `json:"period"`
	PortDetail interface{} `json:"-"`
	PortSteps  []PortStep  `json:"-"`
}

type portFilter func(b BufferTask) bool

type PortStep struct {
	Time              sim.VTimeInSec `json:"time"`
	MessageCount      int64          `json:"message_counts"`
	TrafficBytesCount int64          `json:"traffic_bytes_counts"`
}
