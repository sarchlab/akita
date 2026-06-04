// Package vm provides the models for address translations
package vm

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// TranslationReq is a translation request.
type TranslationReq struct {
	messaging.MsgMeta
	VAddr        uint64
	PID          PID
	DeviceID     uint64
	TransLatency uint64
}

// TranslationRsp is a translation response carrying the physical address.
type TranslationRsp struct {
	messaging.MsgMeta
	Page Page
}

// PageMigrationInfo records the information required for the driver to perform
// a page migration.
type PageMigrationInfo struct {
	GPUReqToVAddrMap map[uint64][]uint64
}

// PageMigrationReqToDriver is a page migration request from MMU to the driver.
type PageMigrationReqToDriver struct {
	messaging.MsgMeta
	StartTime         timing.VTimeInSec
	EndTime           timing.VTimeInSec
	MigrationInfo     PageMigrationInfo
	CurrAccessingGPUs []uint64
	PID               PID
	CurrPageHostGPU   uint64
	PageSize          uint64
	RespondToTop      bool
}

// NewPageMigrationReqToDriver creates a new PageMigrationReqToDriver.
func NewPageMigrationReqToDriver(
	src, dst messaging.RemotePort,
) PageMigrationReqToDriver {
	r := PageMigrationReqToDriver{}
	r.ID = timing.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst
	r.TrafficClass = "vm.PageMigrationReqToDriver"
	return r
}

// PageMigrationRspFromDriver is a page migration response from driver to MMU.
type PageMigrationRspFromDriver struct {
	messaging.MsgMeta
	StartTime timing.VTimeInSec
	EndTime   timing.VTimeInSec
	VAddr     []uint64
	RspToTop  bool
}

// NewPageMigrationRspFromDriver creates a new PageMigrationRspFromDriver.
func NewPageMigrationRspFromDriver(
	src, dst messaging.RemotePort,
	originalReqID uint64,
) PageMigrationRspFromDriver {
	r := PageMigrationRspFromDriver{}
	r.ID = timing.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst
	r.RspTo = originalReqID
	r.TrafficClass = "vm.PageMigrationRspFromDriver"
	return r
}
