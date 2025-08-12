// Package vm provides the models for address translations
package vm

import (
	"reflect"

	"github.com/sarchlab/akita/v4/sim"
)

// A TranslationReq asks the receiver component to translate the request.
type TranslationReq struct {
	sim.MsgMeta

	VAddr    uint64
	PID      PID
	DeviceID uint64
}

// Meta returns the meta data associated with the message.
func (r *TranslationReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned TranslationReq with different ID
func (r *TranslationReq) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GenerateRsp generates response to original translation request
func (r *TranslationReq) GenerateRsp(page Page) sim.Rsp {
	rsp := TranslationRspBuilder{}.
		WithSrc(r.Dst).
		WithDst(r.Src).
		WithRspTo(r.ID).
		WithPage(page).
		Build()

	return rsp
}

// TranslationReqBuilder can build translation requests
type TranslationReqBuilder struct {
	src, dst sim.RemotePort
	vAddr    uint64
	pid      PID
	deviceID uint64
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

// Build creates a new TranslationReq
func (b TranslationReqBuilder) Build() *TranslationReq {
	r := &TranslationReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.VAddr = b.vAddr
	r.PID = b.pid
	r.DeviceID = b.deviceID
	r.TrafficClass = reflect.TypeOf(TranslationReq{}).String()

	return r
}

// A TranslationRsp is the respond for a TranslationReq. It carries the physical
// address.
type TranslationRsp struct {
	sim.MsgMeta

	RespondTo string // The ID of the request it replies
	Page      Page
}

// Meta returns the meta data associated with the message.
func (r *TranslationRsp) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned TranslationRsp with different ID
func (r *TranslationRsp) Clone() sim.Msg {
	cloneMsg := *r
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// GetRspTo returns the request ID that the respond is responding to.
func (r *TranslationRsp) GetRspTo() string {
	return r.RespondTo
}

// TranslationRspBuilder can build translation requests
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

// Build creates a new TranslationRsp
func (b TranslationRspBuilder) Build() *TranslationRsp {
	r := &TranslationRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.RespondTo = b.rspTo
	r.Page = b.page
	r.TrafficClass = reflect.TypeOf(TranslationReq{}).String()

	return r
}

// PageMigrationInfo records the information required for the driver to perform
// a page migration.
type PageMigrationInfo struct {
	GPUReqToVAddrMap map[uint64][]uint64
}

// PageMigrationReqToDriver is a req to driver from MMU to start page migration
// process
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

// Meta returns the meta data associated with the message.
func (m *PageMigrationReqToDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns cloned PageMigrationReqToDriver with different ID
func (m *PageMigrationReqToDriver) Clone() sim.Msg {
	return m
}

func (m *PageMigrationReqToDriver) GenerateRsp() sim.Rsp {
	rsp := NewPageMigrationRspFromDriver(m.Dst, m.Src, m)

	return rsp
}

// NewPageMigrationReqToDriver creates a PageMigrationReqToDriver.
func NewPageMigrationReqToDriver(
	src, dst sim.RemotePort,
) *PageMigrationReqToDriver {
	cmd := new(PageMigrationReqToDriver)
	cmd.Src = src
	cmd.Dst = dst
	cmd.TrafficClass = reflect.TypeOf(PageMigrationReqToDriver{}).String()

	return cmd
}

// PageMigrationRspFromDriver is a rsp from driver to MMU marking completion of
// migration
type PageMigrationRspFromDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
	VAddr     []uint64
	RspToTop  bool

	OriginalReq sim.Msg
}

// Meta returns the meta data associated with the message.
func (m *PageMigrationRspFromDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns cloned PageMigrationRspFromDriver with different ID
func (m *PageMigrationRspFromDriver) Clone() sim.Msg {
	return m
}

func (m *PageMigrationRspFromDriver) GetRspTo() string {
	return m.OriginalReq.Meta().ID
}

// NewPageMigrationRspFromDriver creates a new PageMigrationRspFromDriver.
func NewPageMigrationRspFromDriver(
	src, dst sim.RemotePort,
	originalReq sim.Msg,
) *PageMigrationRspFromDriver {
	cmd := new(PageMigrationRspFromDriver)
	cmd.Src = src
	cmd.Dst = dst
	cmd.OriginalReq = originalReq
	cmd.TrafficClass = reflect.TypeOf(PageMigrationReqToDriver{}).String()

	return cmd
}
