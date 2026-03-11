// Package vm provides the models for address translations
package vm

import (
	"github.com/sarchlab/akita/v5/sim"
)

// TranslationReq is a translation request.
type TranslationReq struct {
	sim.MsgMeta
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

// Build creates a new TranslationReq.
func (b TranslationReqBuilder) Build() *TranslationReq {
	r := &TranslationReq{
		VAddr:        b.vAddr,
		PID:          b.pid,
		DeviceID:     b.deviceID,
		TransLatency: b.transLatency,
	}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficClass = "vm.TranslationReq"
	return r
}

// TranslationRsp is a translation response carrying the physical address.
type TranslationRsp struct {
	sim.MsgMeta
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

// Build creates a new TranslationRsp.
func (b TranslationRspBuilder) Build() *TranslationRsp {
	r := &TranslationRsp{
		Page: b.page,
	}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.RspTo = b.rspTo
	r.TrafficClass = "vm.TranslationRsp"
	return r
}

// PageMigrationInfo records the information required for the driver to perform
// a page migration.
type PageMigrationInfo struct {
	GPUReqToVAddrMap map[uint64][]uint64
}

// PageMigrationReqToDriver is a page migration request from MMU to the driver.
type PageMigrationReqToDriver struct {
	sim.MsgMeta
	StartTime         sim.VTimeInSec
	EndTime           sim.VTimeInSec
	MigrationInfo     *PageMigrationInfo
	CurrAccessingGPUs []uint64
	PID               PID
	CurrPageHostGPU   uint64
	PageSize          uint64
	RespondToTop      bool
}

// NewPageMigrationReqToDriver creates a new PageMigrationReqToDriver.
func NewPageMigrationReqToDriver(
	src, dst sim.RemotePort,
) *PageMigrationReqToDriver {
	r := &PageMigrationReqToDriver{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst
	r.TrafficClass = "vm.PageMigrationReqToDriver"
	return r
}

// PageMigrationRspFromDriver is a page migration response from driver to MMU.
type PageMigrationRspFromDriver struct {
	sim.MsgMeta
	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
	VAddr     []uint64
	RspToTop  bool
}

// NewPageMigrationRspFromDriver creates a new PageMigrationRspFromDriver.
func NewPageMigrationRspFromDriver(
	src, dst sim.RemotePort,
	originalReqID string,
) *PageMigrationRspFromDriver {
	r := &PageMigrationRspFromDriver{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst
	r.RspTo = originalReqID
	r.TrafficClass = "vm.PageMigrationRspFromDriver"
	return r
}
