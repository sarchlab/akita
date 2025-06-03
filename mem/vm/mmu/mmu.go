package mmu

import (
	"log"
	"reflect"

	"fmt"

	"github.com/sarchlab/akita/v3/mem/vm"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

type transaction struct {
	req       *vm.TranslationReq
	page      vm.Page
	cycleLeft int
	migration *vm.PageMigrationReqToDriver
}

// MMU is the default mmu implementation. It is also an akita Component.
type MMU struct {
	sim.TickingComponent

	topPort       sim.Port
	migrationPort sim.Port

	MigrationServiceProvider sim.Port

	topSender sim.BufferedSender

	pageTable           vm.PageTable
	latency             int
	maxRequestsInFlight int

	walkingTranslations      []transaction
	migrationQueue           []transaction
	migrationQueueSize       int
	currentOnDemandMigration transaction
	isDoingMigration         bool

	// DELETE THIS LATE
	// paTable *vm.PATable
	// paCache *vm.PACache

	paTable *vm.PATable
	paCache *vm.PACache

	// END

	toRemoveFromPTW        []int
	PageAccessedByDeviceID map[uint64][]uint64
}

// Tick defines how the MMU update state each cycle
func (mmu *MMU) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = mmu.topSender.Tick(now) || madeProgress
	madeProgress = mmu.sendMigrationToDriver(now) || madeProgress
	madeProgress = mmu.walkPageTable(now) || madeProgress
	madeProgress = mmu.processMigrationReturn(now) || madeProgress
	madeProgress = mmu.parseFromTop(now) || madeProgress

	return madeProgress
}

func (mmu *MMU) walkPageTable(now sim.VTimeInSec) bool {
	madeProgress := false
	for i := 0; i < len(mmu.walkingTranslations); i++ { //A for loop begins to iterate through the walkingTranslations slice.
		//Each element in walkingTranslations represents a transaction (a request to walk the page table).
		if mmu.walkingTranslations[i].cycleLeft > 0 {
			mmu.walkingTranslations[i].cycleLeft--
			madeProgress = true
			//fmt.Println("inside if: ", mmu.walkingTranslations[i].page)
			continue
		}
		//fmt.Println("PID: ", mmu.walkingTranslations[i].req.PID, "VAddr: ", mmu.walkingTranslations[i].req.VAddr)

		madeProgress = mmu.finalizePageWalk(now, i) || madeProgress //cycleleft =0. It means the transaction is ready to be finalized
	}

	tmp := mmu.walkingTranslations[:0]
	for i := 0; i < len(mmu.walkingTranslations); i++ {
		if !mmu.toRemove(i) {
			tmp = append(tmp, mmu.walkingTranslations[i])
		}
	}
	mmu.walkingTranslations = tmp
	mmu.toRemoveFromPTW = nil

	return madeProgress
}

/*
The function processes each transaction in walkingTranslations, reducing cycleLeft by one for in-progress transactions.
If a transaction is completed (i.e., cycleLeft becomes 0), it calls finalizePageWalk to finish the transaction.
After processing all transactions, it filters out the ones that need to be removed using the toRemove function.
The walkingTranslations slice is updated with the remaining transactions, and the function returns true if any progress was made
*/

func (mmu *MMU) finalizePageWalk( //It finalizes a page walk for the transaction at walkingIndex in the walkingTranslations array.
	now sim.VTimeInSec,
	walkingIndex int,
) bool {
	req := mmu.walkingTranslations[walkingIndex].req      //req is of type *vm.TranslationReq and contains information like the PID (Process ID) and virtual address (VAddr) being translated.
	page, found := mmu.pageTable.Find(req.PID, req.VAddr) //find page in pagetable

	if !found { //if page not found
		panic("page not found")
	}

	mmu.walkingTranslations[walkingIndex].page = page //if page found. Add this to walkingTranslations at walkingIndex (whose cyceltime was 0)

	if page.IsMigrating {
		return mmu.addTransactionToMigrationQueue(walkingIndex)
	}

	if mmu.pageNeedMigrate(mmu.walkingTranslations[walkingIndex]) {
		return mmu.addTransactionToMigrationQueue(walkingIndex)
	}

	//fmt.Printf("Page %x hit on device %d \n", page.VAddr, req.DeviceID)
	return mmu.doPageWalkHit(now, walkingIndex) //If the page does not need migration and is not migrating, the page walk is considered a "hit."
}

func (mmu *MMU) addTransactionToMigrationQueue(walkingIndex int) bool {
	if len(mmu.migrationQueue) >= mmu.migrationQueueSize {
		return false //return false to indicate that the transaction cannot be added to the migration queue because it is full.
	}

	mmu.toRemoveFromPTW = append(mmu.toRemoveFromPTW, walkingIndex) //Adds the index of the transaction to toRemoveFromPTW, indicating it will be removed from the ongoing page table walks.
	mmu.migrationQueue = append(mmu.migrationQueue,
		mmu.walkingTranslations[walkingIndex]) //add tranction into migrationQueue

	page := mmu.walkingTranslations[walkingIndex].page // get page
	page.IsMigrating = true                            //make ismigrating flag to true
	mmu.pageTable.Update(page)                         //make an update in PageTable

	//print page migration queue print on which device that page is
	fmt.Printf("Page %x added to migration queue. page available on gpu =  %d \n", page.VAddr, page.DeviceID)
	//fmt.Printf("Page %d added to migration queue. page availble on gpu =  %d \n", page.VAddr, page.DeviceID)

	return true
}

func (mmu *MMU) pageNeedMigrate(walking transaction) bool {
	if walking.req.DeviceID == walking.page.DeviceID { // Compares the DeviceID of the request (walking.req.DeviceID) with the DeviceID of the page (walking.page.DeviceID).
		// If the page is already located on the same device making the request, migration is unnecessary, so the function returns false.
		fmt.Printf("Page %x is already on the device %d. No Migration\n", walking.page.VAddr, walking.req.DeviceID)
		return false
	}

	if !walking.page.Unified { // if Unified no need to migrate
		fmt.Printf("Page %x is not unified. No Migration \n", walking.page.VAddr)
		return false
	}

	if walking.page.IsPinned { //if page is pinned -- no need to migrate
		fmt.Printf("Page %x is pinned on GPU =%d. No Migration \n", walking.page.VAddr, walking.page.DeviceID)
		return false
	}

	walking.page.AccessCount++
	fmt.Printf("Page %x accessed by GPU %d and Access count =%d \n", walking.page.VAddr, walking.page.DeviceID, walking.page.AccessCount)
	migrationThreshold := uint64(256) //Threshold for the number of accesses before migration is triggered
	if walking.page.AccessCount < migrationThreshold {
		return false
	}
	fmt.Printf("Page %x reached Migration threshold . Migrating to GPU = %d \n", walking.page.VAddr, walking.req.DeviceID)

	return true
}

func (mmu *MMU) doPageWalkHit(
	now sim.VTimeInSec,
	walkingIndex int,
) bool {
	if !mmu.topSender.CanSend(1) {
		return false
	}
	walking := mmu.walkingTranslations[walkingIndex]

	//HandlePageFault(Page, pid vm.PID, readWrite bool, PA_Table *PATable) {

	mmu.paCache.HandlePageFault(walking.page.VAddr, walking.req.Write, mmu.paTable) //Handle page fault
	mmu.paCache.PrintPACache()                                                      //print page cache
	mmu.paCache.PrintPATable(mmu.paTable)                                           //print page table

	//vpn uint64, readWrite bool, PA_Table *PATable
	//print page hit and repsonse
	fmt.Printf("Page %x hit on device %d. Sending response to device %d. R/w =%t .  No Migration\n", walking.page.VAddr, walking.page.DeviceID, walking.req.DeviceID, walking.req.Write)

	rsp := vm.TranslationRspBuilder{}.
		WithSendTime(now).
		WithSrc(mmu.topPort).
		WithDst(walking.req.Src).
		WithRspTo(walking.req.ID).
		WithPage(walking.page).
		Build()

	mmu.topSender.Send(rsp)
	mmu.toRemoveFromPTW = append(mmu.toRemoveFromPTW, walkingIndex)

	tracing.TraceReqComplete(walking.req, mmu)

	return true
}

func (mmu *MMU) sendMigrationToDriver(
	now sim.VTimeInSec,
) (madeProgress bool) {
	if len(mmu.migrationQueue) == 0 {
		return false
	} //If the migrationQueue is empty, there are no pending migrations to handle. The function returns false (no progress made).

	trans := mmu.migrationQueue[0]                        //Retrieves the first transaction (trans) from the migration queue.
	req := trans.req                                      //get req
	page, found := mmu.pageTable.Find(req.PID, req.VAddr) //find page in PageTable
	if !found {
		panic("page not found")
	}
	trans.page = page //if page found update into trans

	if req.DeviceID == page.DeviceID || page.IsPinned {
		//If: The page is already located on the requesting device (req.DeviceID == page.DeviceID).
		//    The page is pinned (page.IsPinned).
		mmu.sendTranlationRsp(now, trans)                        //Sends a response back to the requester (sendTranlationRsp).
		mmu.migrationQueue = mmu.migrationQueue[1:]              //Removes the transaction from the migration queue.
		mmu.markPageAsNotMigratingIfNotInTheMigrationQueue(page) //Marks the page as no longer migrating (if applicable).

		return true //		Returns true, indicating progress was made.

	}

	if mmu.isDoingMigration { //If the MMU is already handling a migration, it cannot start a new one. Returns false (no progress).
		return false
	}

	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID],
			trans.req.VAddr) //add page VAddr into GPUReqToVAddrMap[

	mmu.PageAccessedByDeviceID[page.VAddr] =
		append(mmu.PageAccessedByDeviceID[page.VAddr], page.DeviceID) //add gpu id into map pageAccessedByDeviceID

	migrationReq := vm.NewPageMigrationReqToDriver(
		now, mmu.migrationPort, mmu.MigrationServiceProvider) //parameters -time,src and dst. Constructs a new page migration request.
	migrationReq.PID = page.PID
	migrationReq.PageSize = page.PageSize
	migrationReq.CurrPageHostGPU = page.DeviceID
	migrationReq.MigrationInfo = migrationInfo
	migrationReq.CurrAccessingGPUs = unique(mmu.PageAccessedByDeviceID[page.VAddr])
	//migrationReq.CurrAccessingGPUs = mmu.PageAccessedByDeviceID[page.VAddr]
	migrationReq.RespondToTop = true

	//print migrationreq

	//fmt.Printf("\nMigrationReqTODriver: PID=%d, PageSize=%d, CurrPageHostGPU=%d, CurrAccessingGPUs=%v,MigrationInfo=%v, RespondToTop=%t\n", 	migrationReq.PID, migrationReq.PageSize, migrationReq.CurrPageHostGPU, migrationReq.CurrAccessingGPUs, migrationReq.MigrationInfo, migrationReq.RespondToTop)
	//

	err := mmu.migrationPort.Send(migrationReq) //Sends the migration request through the migrationPort.
	if err != nil {                             //If an error occurs (e.g., port is busy), it returns false (no progress).
		return false
	}

	trans.page.IsMigrating = true        //Marks the page as being migrated by setting
	trans.page.AccessCount = 0           //Resets the access count to 0.
	mmu.pageTable.Update(trans.page)     // updating the page table with page is migrating.
	trans.migration = migrationReq       // tranction has migration with type *vm.PageMigrationReqToDriver. migrationReq is request to driver
	mmu.isDoingMigration = true          //update MMU state- Indicates the MMU is actively handling a migration
	mmu.currentOnDemandMigration = trans //Tracks the ongoing migration.

	// fmt.Printf("\nOndemand MigrationReq: PID=%d, PageSize=%d, CurrPageHostGPU=%d, MigrationInfo=%v, CurrAccessingGPUs=%v, RespondToTop=%t\n",
	// mmu.currentOnDemandMigration.PID, mmu.currentOnDemandMigration.PageSize, mmu.currentOnDemandMigration.CurrPageHostGPU, mmu.currentOnDemandMigration.MigrationInfo, mmu.currentOnDemandMigration.CurrAccessingGPUs, mmu.currentOnDemandMigration.RespondToTop)

	mmu.migrationQueue = mmu.migrationQueue[1:] //Removes the transaction from the migrationQueue.

	return true //MMU successfully initiated a migration and made progress.
}

// Purpose: Ensure that pages marked as "migrating" (IsMigrating = true) are still part of the migrationQueue
func (mmu *MMU) markPageAsNotMigratingIfNotInTheMigrationQueue(
	page vm.Page,
) vm.Page { // The page object to be checked and potentially updated
	inQueue := false
	for _, t := range mmu.migrationQueue { //Check If Page Is In Queue
		if page.PAddr == t.page.PAddr { //If a match is found, the page is still being migrated, so inQueue is set to true, and the loop exits early (break)
			inQueue = true
			break
		}
	}

	if !inQueue { //If the page is not in the queue
		page.IsMigrating = false //update IsMigrating and update into pageTable
		mmu.pageTable.Update(page)
		return page //Returns the updated page.
	}

	return page //If the page is still in the queue (inQueue == true), it simply returns the unmodified page object.
}

func (mmu *MMU) sendTranlationRsp(
	now sim.VTimeInSec,
	trans transaction,
) (madeProgress bool) {
	req := trans.req
	page := trans.page

	rsp := vm.TranslationRspBuilder{}.
		WithSendTime(now).
		WithSrc(mmu.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	mmu.topSender.Send(rsp)

	return true
}

// When the migration is finished, the MMU processes the migration's return, updates the relevant page's state, and sends a response back to the requester.
func (mmu *MMU) processMigrationReturn(now sim.VTimeInSec) bool {
	item := mmu.migrationPort.Peek() //Checks if there is a message waiting in the migration port (indicating a migration has returned).
	if item == nil {
		return false // If no message is available, return false, indicating no progress was made in this cycle.
	}

	if !mmu.topSender.CanSend(1) { //Verifies if the MMU's topSender can send at least one message.
		return false //If the topSender is currently full or busy, return false.
	}

	req := mmu.currentOnDemandMigration.req               //Retrieves the request associated with the current migration.
	page, found := mmu.pageTable.Find(req.PID, req.VAddr) //Looks up the corresponding page in the page table using the process ID (PID) and virtual address (VAddr) from the request.
	if !found {                                           //If the page is not found, it is an unexpected error, and the program panics.

		panic("page not found")
	}

	rsp := vm.TranslationRspBuilder{}.
		WithSendTime(now).
		WithSrc(mmu.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build() //finalize and create response object
	mmu.topSender.Send(rsp) // Sends the response using the topSender.

	// print migration complete for page
	//fmt.Printf("Migration Complete for Page %x. Sending response to device %d\n", page.VAddr, req.DeviceID)
	mmu.isDoingMigration = false //Marks that the MMU is no longer performing a migration, freeing it up for new requests.

	page = mmu.markPageAsNotMigratingIfNotInTheMigrationQueue(page) //Checks if the page is still in the migration queue. If not, updates the page's state to indicate it is no longer migrating
	//page.IsPinned = true  // Marks the page as "pinned," meaning it cannot be migrated further.
	mmu.pageTable.Update(page) //Update pagetable with all these changes

	mmu.migrationPort.Retrieve(now) //remove processed migration request

	return true //indicates that the function successfully made progress during this cycle
}

func (mmu *MMU) parseFromTop(now sim.VTimeInSec) bool {

	if len(mmu.walkingTranslations) >= mmu.maxRequestsInFlight { //If the number of requests currently being processed (walkingTranslations) has reached
		//the maximum allowed (maxRequestsInFlight), the function returns false, indicating no progress can be made.
		return false
	}

	req := mmu.topPort.Retrieve(now) //Retrieves a new request from the top port queue

	if req == nil { //if no request returns false
		return false
	}

	tracing.TraceReqReceive(req, mmu) //do log

	switch req := req.(type) { //check type of request
	case *vm.TranslationReq: //if its translationrequest call startwalking
		mmu.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (mmu *MMU) startWalking(req *vm.TranslationReq) { //Adds the transaction to the list of ongoing operations.
	translationInPipeline := transaction{
		req:       req,
		cycleLeft: mmu.latency, //pagewalk Latency
	}
	// print new request added to walkingtranslations
	fmt.Printf("New Request Added to WalkingTranslations: PID=%d, VAddr=%x, r/w =%t, DeviceID=%d \n", req.PID, req.VAddr, req.Write, req.DeviceID)

	mmu.walkingTranslations = append(mmu.walkingTranslations, translationInPipeline) //Adds the transaction to the walkingTranslations slice, which tracks all ongoing page table walks.
}

func (mmu *MMU) toRemove(index int) bool {
	for i := 0; i < len(mmu.toRemoveFromPTW); i++ {
		remove := mmu.toRemoveFromPTW[i]
		if remove == index {
			return true
		}
	}
	return false
}

func unique(intSlice []uint64) []uint64 {
	keys := make(map[int]bool)
	list := []uint64{}
	for _, entry := range intSlice {
		if _, value := keys[int(entry)]; !value {
			keys[int(entry)] = true
			list = append(list, entry)
		}
	}
	return list
}
