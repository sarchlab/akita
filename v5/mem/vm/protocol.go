// Package vm provides the models for address translations
package vm

import (
	"reflect"

	"github.com/sarchlab/akita/v5/sim"
)

// TranslationReqPayload is the payload for a translation request.
type TranslationReqPayload struct {
	VAddr        uint64
	PID          PID
	DeviceID     uint64
	TransLatency uint64
}

// TranslationReqBuilder can build translation requests
type TranslationReqBuilder struct {
	src, dst     sim.RemotePort
	vAddr        uint64
	pid          PID
	deviceID     uint64
	transLatency uint64
}

// WithTransLatency sets the translation latency of the request to build.
func (b TranslationReqBuilder) WithTransLatency(latency uint64) TranslationReqBuilder {
	b.transLatency = latency
	return b
}

// WithSrc sets the source of the request to build.
func (b TranslationReqBuilder) WithSrc(
	src sim.RemotePort,
) TranslationReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b TranslationReqBuilder) WithDst(
	dst sim.RemotePort,
) TranslationReqBuilder {
	b.dst = dst
	return b
}

// WithVAddr sets the virtual address of the request to build.
func (b TranslationReqBuilder) WithVAddr(vAddr uint64) TranslationReqBuilder {
	b.vAddr = vAddr
	return b
}

// WithPID sets the virtual address of the request to build.
func (b TranslationReqBuilder) WithPID(pid PID) TranslationReqBuilder {
	b.pid = pid
	return b
}

// WithDeviceID sets the GPU ID of the request to build.
func (b TranslationReqBuilder) WithDeviceID(
	deviceID uint64,
) TranslationReqBuilder {
	b.deviceID = deviceID
	return b
}

// Build creates a new *sim.GenericMsg with TranslationReqPayload.
func (b TranslationReqBuilder) Build() *sim.GenericMsg {
	payload := &TranslationReqPayload{
		VAddr:        b.vAddr,
		PID:          b.pid,
		DeviceID:     b.deviceID,
		TransLatency: b.transLatency,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficClass: reflect.TypeOf(TranslationReqPayload{}).String(),
		},
		Payload: payload,
	}
}

// TranslationRspPayload is the payload for a translation response carrying the
// physical address.
type TranslationRspPayload struct {
	Page Page
}

// TranslationRspBuilder can build translation responses
type TranslationRspBuilder struct {
	src, dst sim.RemotePort
	rspTo    string
	page     Page
}

// WithSrc sets the source of the respond to build.
func (b TranslationRspBuilder) WithSrc(
	src sim.RemotePort,
) TranslationRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the respond to build.
func (b TranslationRspBuilder) WithDst(
	dst sim.RemotePort,
) TranslationRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets the request ID of the respond to build.
func (b TranslationRspBuilder) WithRspTo(rspTo string) TranslationRspBuilder {
	b.rspTo = rspTo
	return b
}

// WithPage sets the page of the respond to build.
func (b TranslationRspBuilder) WithPage(page Page) TranslationRspBuilder {
	b.page = page
	return b
}

// Build creates a new *sim.GenericMsg with TranslationRspPayload.
func (b TranslationRspBuilder) Build() *sim.GenericMsg {
	payload := &TranslationRspPayload{
		Page: b.page,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			RspTo:        b.rspTo,
			TrafficClass: reflect.TypeOf(TranslationReqPayload{}).String(),
		},
		Payload: payload,
	}
}

// PageMigrationInfo records the information required for the driver to perform
// a page migration.
type PageMigrationInfo struct {
	GPUReqToVAddrMap map[uint64][]uint64
}

// PageMigrationReqToDriverPayload is the payload for a page migration request
// from MMU to the driver.
type PageMigrationReqToDriverPayload struct {
	StartTime         sim.VTimeInSec
	EndTime           sim.VTimeInSec
	MigrationInfo     *PageMigrationInfo
	CurrAccessingGPUs []uint64
	PID               PID
	CurrPageHostGPU   uint64
	PageSize          uint64
	RespondToTop      bool
}

// NewPageMigrationReqToDriver creates a *sim.GenericMsg with
// PageMigrationReqToDriverPayload.
func NewPageMigrationReqToDriver(
	src, dst sim.RemotePort,
) *sim.GenericMsg {
	payload := &PageMigrationReqToDriverPayload{}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			Src:          src,
			Dst:          dst,
			TrafficClass: reflect.TypeOf(PageMigrationReqToDriverPayload{}).String(),
		},
		Payload: payload,
	}
}

// PageMigrationRspFromDriverPayload is the payload for a page migration
// response from driver to MMU.
type PageMigrationRspFromDriverPayload struct {
	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
	VAddr     []uint64
	RspToTop  bool
}

// NewPageMigrationRspFromDriver creates a new *sim.GenericMsg with
// PageMigrationRspFromDriverPayload.
func NewPageMigrationRspFromDriver(
	src, dst sim.RemotePort,
	originalReqID string,
) *sim.GenericMsg {
	payload := &PageMigrationRspFromDriverPayload{}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			Src:          src,
			Dst:          dst,
			RspTo:        originalReqID,
			TrafficClass: reflect.TypeOf(PageMigrationReqToDriverPayload{}).String(),
		},
		Payload: payload,
	}
}
