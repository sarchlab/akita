// Package vm provides the models for address translations
package vm

import (
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A TranslationReq asks the receiver component to translate the request.
type TranslationReq struct {
	modeling.MsgMeta
	VAddr    uint64
	PID      PID
	DeviceID uint64
}

// Meta returns the meta data associated with the message.
func (r TranslationReq) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned TranslationReq with different ID
func (r TranslationReq) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GenerateRsp generates response to original translation request
func (r TranslationReq) GenerateRsp(page Page) modeling.Rsp {
	rsp := TranslationRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: r.Dst,
			Dst: r.Src,
		},
		RespondTo: r.ID,
		Page:      page,
	}

	return rsp
}

// A TranslationRsp is the respond for a TranslationReq. It carries the physical
// address.
type TranslationRsp struct {
	modeling.MsgMeta
	RespondTo string // The ID of the request it replies
	Page      Page
}

// Meta returns the meta data associated with the message.
func (r TranslationRsp) Meta() modeling.MsgMeta {
	return r.MsgMeta
}

// Clone returns cloned TranslationRsp with different ID
func (r TranslationRsp) Clone() modeling.Msg {
	cloneMsg := r
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// GetRspTo returns the request ID that the respond is responding to.
func (r TranslationRsp) GetRspTo() string {
	return r.RespondTo
}

// PageMigrationInfo records the information required for the driver to perform
// a page migration.
type PageMigrationInfo struct {
	GPUReqToVAddrMap map[uint64][]uint64
}

// PageMigrationReqToDriver is a req to driver from MMU to start page migration
// process
type PageMigrationReqToDriver struct {
	modeling.MsgMeta

	StartTime         timing.VTimeInSec
	EndTime           timing.VTimeInSec
	MigrationInfo     *PageMigrationInfo
	CurrAccessingGPUs []uint64
	PID               PID
	CurrPageHostGPU   uint64
	PageSize          uint64
	RespondToTop      bool
}

// Meta returns the meta data associated with the message.
func (m PageMigrationReqToDriver) Meta() modeling.MsgMeta {
	return m.MsgMeta
}

// Clone returns cloned PageMigrationReqToDriver with different ID
func (m PageMigrationReqToDriver) Clone() modeling.Msg {
	return m
}

func (m PageMigrationReqToDriver) GenerateRsp() modeling.Rsp {
	rsp := NewPageMigrationRspFromDriver(m.Dst, m.Src, m)

	return rsp
}

// NewPageMigrationReqToDriver creates a PageMigrationReqToDriver.
func NewPageMigrationReqToDriver(
	src, dst modeling.RemotePort,
) *PageMigrationReqToDriver {
	cmd := new(PageMigrationReqToDriver)
	cmd.Src = src
	cmd.Dst = dst

	return cmd
}

// PageMigrationRspFromDriver is a rsp from driver to MMU marking completion of
// migration
type PageMigrationRspFromDriver struct {
	modeling.MsgMeta

	StartTime timing.VTimeInSec
	EndTime   timing.VTimeInSec
	VAddr     []uint64
	RspToTop  bool

	OriginalReq modeling.Msg
}

// Meta returns the meta data associated with the message.
func (m PageMigrationRspFromDriver) Meta() modeling.MsgMeta {
	return m.MsgMeta
}

// Clone returns cloned PageMigrationRspFromDriver with different ID
func (m PageMigrationRspFromDriver) Clone() modeling.Msg {
	return m
}

func (m *PageMigrationRspFromDriver) GetRspTo() string {
	return m.OriginalReq.Meta().ID
}

// NewPageMigrationRspFromDriver creates a new PageMigrationRspFromDriver.
func NewPageMigrationRspFromDriver(
	src, dst modeling.RemotePort,
	originalReq modeling.Msg,
) *PageMigrationRspFromDriver {
	cmd := new(PageMigrationRspFromDriver)
	cmd.Src = src
	cmd.Dst = dst
	cmd.OriginalReq = originalReq

	return cmd
}
