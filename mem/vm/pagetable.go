package vm

import (
	"container/list" //doubly linkedlist
	"sync"           //synchronisation
)

// PID stands for Process ID.

type PID uint32

// A Page is an entry in the page table, maintaining the information about how
// to translate a virtual address to a physical address.
type Page struct {
	PID         PID
	PAddr       uint64
	VAddr       uint64
	PageSize    uint64
	Valid       bool
	DeviceID    uint64
	Unified     bool
	IsMigrating bool
	IsPinned    bool
	AccessCount uint64
	// FaultCount  uint64
}

// A PageTable holds the a list of pages.
// we can perform these operations on Page Table
type PageTable interface {
	Insert(page Page) // first page is paramater name, second page is data  type.
	Remove(pid PID, vAddr uint64)
	Find(pid PID, Addr uint64) (Page, bool) //(page,bool) is return type
	Update(page Page)
}

// NewPageTable creates a new PageTable.
func NewPageTable(log2PageSize uint64) PageTable {
	return &pageTableImpl{
		log2PageSize: log2PageSize,
		tables:       make(map[PID]*processTable),
	}
}

// pageTableImpl is the default implementation of a Page Table
type pageTableImpl struct {
	sync.Mutex
	log2PageSize uint64
	tables       map[PID]*processTable
}

func (pt *pageTableImpl) getTable(pid PID) *processTable { // Retrieves the processTable for a specific process ID (pid). If it doesn’t exist, a new one is created.
	pt.Lock()         // ensures that access to the tables map is thread-safe.
	defer pt.Unlock() //ensures the mutex is released when the function exits.

	table, found := pt.tables[pid]
	if !found {
		table = &processTable{
			entries:      list.New(),                     //new doubly linked list
			entriesTable: make(map[uint64]*list.Element), //An empty map with uint64 keys and pointers to list elements as values.
		}
		pt.tables[pid] = table
	}

	return table
}

func (pt *pageTableImpl) alignToPage(addr uint64) uint64 {
	return (addr >> pt.log2PageSize) << pt.log2PageSize //>> : divides addr by the page size, discarding the offset within the page.
	//<< : Shifts the bits of the result back to the left, multiplying it by the page size.
}

// Insert put a new page into the PageTable
func (pt *pageTableImpl) Insert(page Page) {
	table := pt.getTable(page.PID)
	table.insert(page)
}

// Remove removes the entry in the page table that contains the target
// address.
func (pt *pageTableImpl) Remove(pid PID, vAddr uint64) {
	table := pt.getTable(pid)
	table.remove(vAddr)
}

// Find returns the page that contains the given virtual address. The bool
// return value invicates if the page is found or not.
func (pt *pageTableImpl) Find(pid PID, vAddr uint64) (Page, bool) {
	table := pt.getTable(pid)
	vAddr = pt.alignToPage(vAddr)
	return table.find(vAddr)
}

// Update changes the field of an existing page. The PID and the VAddr field
// will be used to locate the page to update.
func (pt *pageTableImpl) Update(page Page) {
	table := pt.getTable(page.PID)
	table.update(page)
}

type processTable struct { //This is a structure that models a page table for a specific process. The processTable holds the page mappings for a process.
	sync.Mutex
	entries      *list.List
	entriesTable map[uint64]*list.Element
}

/*
using a doubly linked list (entries) for managing the pages and a map (entriesTable) for fast lookups of pages by their virtual addresses.
*/

func (t *processTable) insert(page Page) {
	t.Lock()
	defer t.Unlock()

	//t.pageMustNotExist(page.VAddr)
	page.AccessCount = 0
	elem := t.entries.PushBack(page)
	t.entriesTable[page.VAddr] = elem
}

func (t *processTable) remove(vAddr uint64) {
	t.Lock()
	defer t.Unlock()

	t.pageMustExist(vAddr)

	elem := t.entriesTable[vAddr]
	t.entries.Remove(elem)
	delete(t.entriesTable, vAddr)
}

func (t *processTable) update(page Page) {
	t.Lock()
	defer t.Unlock()

	t.pageMustExist(page.VAddr)

	elem := t.entriesTable[page.VAddr]
	elem.Value = page
}

func (t *processTable) find(vAddr uint64) (Page, bool) {
	t.Lock()
	defer t.Unlock()

	elem, found := t.entriesTable[vAddr]
	if found {
		return elem.Value.(Page), true
	}

	return Page{}, false
}

func (t *processTable) pageMustExist(vAddr uint64) {
	_, found := t.entriesTable[vAddr]
	if !found {
		panic("page does not exist")
	}
}

func (t *processTable) pageMustNotExist(vAddr uint64) {
	_, found := t.entriesTable[vAddr]
	if found {
		panic("page exist")
	}
}
