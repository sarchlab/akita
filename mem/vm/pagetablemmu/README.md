# Memory-backed MMU

`pagetablemmu.Comp` walks radix page tables by issuing physical memory
requests. Configure one instance per GPU and another at the IOMMU. Each owns
its roots, node allocator, split translation cache (STC), and outstanding work.
Connect the memory port to the cache or memory controller that should see PTE
contention. The platform reserves a distinct, contiguous pool for each instance
and excludes all these pools from data allocation.

## Wiring example

The following configuration uses 4 KiB data pages and four levels. The example
assumes the system builder has created simulation `s` and will supply the
named ports.

```go
gpuSpec := pagetablemmu.DefaultSpec()
gpuSpec.PoolBase, gpuSpec.PoolNodes = 0x100000, 256
gpuSpec.Memory.Destinations = []messaging.RemotePort{"GPU.Memory.Top"}
gpuSpec.FaultHandler = "IOMMU.FaultIn"
gpuSpec.FilterBuckets = 256

ioSpec := pagetablemmu.DefaultSpec()
ioSpec.PoolBase, ioSpec.PoolNodes = 0x400000, 512
ioSpec.Memory.Destinations = []messaging.RemotePort{"CPU.Memory.Top"}
ioSpec.FaultHandler = "Driver.PageFault"
ioSpec.ConsultRemote = true

gpu := modeling.MakeBuilder[*pagetablemmu.Comp](gpuSpec).WithSimulation(s).Build("GPU.MMU")
io := modeling.MakeBuilder[*pagetablemmu.Comp](ioSpec).WithSimulation(s).Build("IOMMU")
for _, mmu := range []*pagetablemmu.Comp{gpu, io} {
    for _, slot := range []string{"Top", "FaultIn", "PageFault", "Memory", "Control"} {
        mmu.BindPort(slot, twowaybuffered.MakeBuilder().WithSimulation(s).Build())
    }
}
```

Bind the ports to the appropriate connections before `s.Initialize()`.
The example's GPU keeps local mappings. The IOMMU must hold every mapping
needed for fallback, including mappings present at the GPU.

| Port | Peer | Messages |
| --- | --- | --- |
| Top | TLB or address translator | Ordinary `addrtransprotocol.Req/Rsp` |
| FaultIn | Upstream MMU's PageFault | `pagefaultprotocol.Req/Rsp` |
| PageFault | Next MMU's FaultIn or driver | `pagefaultprotocol.Req/Rsp` |
| Memory | Memory hierarchy | `memprotocol.ReadReq/WriteReq` and their replies |
| Control | Mapping owner | `pagetableprotocol.Req/Rsp` |

Every incoming translation or fault uses the original PID and VA to walk the
receiver's own root. On a miss the instance sends `HasMapping=false` to its
fault handler. With `ConsultRemote`, a mapping whose owner differs from the
requesting device goes to the handler with `HasMapping=true`. Otherwise a
found mapping is immediately usable. A handler returns `Resolved` when its
mapping is already safe to use, including for remote access. This completes
the current translation. The platform can separately install a reusable PTE
for later walks. `Blocked`
returns an ordinary translation NACK carrying the original PID and page base,
which existing TLBs use to drain their MSHR.

Walks merge on PID and VPN. Pending decisions also match the requesting
device. Admission and output arbitration each get `AdmissionWidth` slots per
cycle. Input remains on the port when either the walk array or the matching
waiter list is full. A completed entry remains occupied while replies wait
for a port. Each accepted message receives its own response ID and `RspTo`.
After a NACK, a new translation starts a new transaction. The driver owns
placement, terminal unmapped diagnostics, Hold/Release and migration.

A fault handler must answer each outstanding fault before waiting for a
control ACK covering that page. The command waits for the fault decision to
drain. For an already usable remote page, reply `Resolved` before waiting
for the optional `Map` ACK. If the access must wait for migration or a usable
mapping, reply `Blocked` first, coordinate Hold and its ACKs, complete the
mapping updates and invalidation, then Release the waiting accesses.

## Page tables and updates

Set the same `Log2PageSize` on every translation component. Page-table nodes
always occupy 4 KiB and contain 512 little-endian, 8-byte entries. Walks
index successive nine-bit VPN slices, with zero extension above the virtual
address width. A hierarchy can have one to eight levels and must cover every
configured VPN bit. For example, append a fourth bank to `Banks` for five
levels. A 57-bit VA and 4 KiB pages then use all root index bits. With a
48-bit VA the fifth level has a fixed root index of zero.

Bit 0 marks a valid PTE. A valid internal entry contains its aligned child
address with bit 0 set. A leaf sets bit 1, stores the physical frame number
in bits 5–47 and its owner device ID in bits 48–63. Bits 2, 3 and 4 record
the unified, migrating and pinned attributes of `vm.Page`. The walker
returns these attributes to policy as data. Unrepresentable frame numbers
or device IDs are rejected when installing a mapping.

Send a `pagetableprotocol.RegisterRoot` command with a PID and a free
4 KiB-aligned physical `Root` in the instance's pool. This creates a fresh,
zeroed address space. After its ACK, issue `Map` with a page-aligned
`VAddr` and `Mapping` containing the aligned data PAddr, owner and policy
flags. The component derives page size, validity, VA and PID. `Remap` and
`Unmap` require an existing leaf. All three leaf operations preserve cached
non-leaf pointers, including prefixes shared by neighboring pages. `Invalidate`
takes an aligned starting VA and positive page-multiple size in bytes.
It drops overlapping STC prefixes and preserves the routing filter's hints
for installed pages. A neighboring walk can finish using a pointer it was
already fetching, but that pending read or fill cannot restore the invalidated
entry. `Teardown` drops every prefix for the PID. Each response correlates to
its command and carries an empty `Error` on success.

A command protects its own affected pages from admission and waits for active
walks and fault replies on those pages to drain. An already admitted walk on
another range can continue. Each input port admits requests in FIFO order,
so a protected request at the head delays later requests on that port. The
other input port can still admit independent work.

Each instance serializes its own commands. The updater reads the
existing path from memory, reserves the whole missing chain before modifying
it, and zeroes each new node using writes of up to 64 bytes, split at line
and memory-route boundaries. Up to eight zeroing writes for one node can be
outstanding. They share the memory issue width with walks. The MMU waits for
every write ACK before registering the root or issuing the parent pointer
write. Subsequent pointer and leaf updates wait for the preceding operation's
ACK. Control acknowledges after physical writes and internal cache/filter
maintenance. The platform coordinates TLB shootdowns and access quiescence
when changing leaf translations.

A failed map, including pool exhaustion, leaves earlier mappings and pool
ownership intact. Unmap clears a leaf and retains its intermediate nodes.
Teardown releases all owned nodes. Reuse always zeroes the frame. Page-table
memory is private to the owning MMU for the address space's lifetime.
Checkpoint the attached memory hierarchy and storage along with the MMU.

## Timing parameters

The defaults illustrate a small pipelined design. They are starting values
for experiments and have no hardware calibration.

- The default four-level walk has 16 walk entries, eight waiters per entry,
  two admissions per cycle and one physical request per cycle. The memory
  arbiter alternates between ready walk and update traffic and rotates across
  ready walkers. A dependent PTE read waits for the prior response.
- Each non-leaf level has a set-associative prefix bank. Defaults from the
  root downward are 4×2, 8×2 and 16×4 sets×ways. Each bank tags PID plus the
  entire VA prefix and stores a next-level physical address. Parallel lookup
  takes two cycles by default, with one lookup and one independent fill per
  bank per cycle. The deepest hit selects the next read. Banks use LRU and
  cache only pointers. Set both dimensions to zero to disable a bank.
- The optional SRAM filter defaults off. Each bucket holds one PID-tagged
  16-bit fingerprint. A lookup probes both deterministic candidate buckets.
  Inserts use a bounded sequence of displacements, each with the configured
  SRAM latency. The default is one operation per cycle, one cycle of latency
  and four displacements. An unresolved collision may discard a fingerprint.
  Two mapped VPNs in the same PID can share a fingerprint in the same bucket.
  Insertion keeps one copy, so unmapping either page can remove the other
  page's hint. A negative bypasses the local walk. The downstream authority
  must hold the complete mappings so it can translate a page even after this
  false negative. A positive still checks PTE memory. Teardown clears the
  PID's entries.

With simulation task tracking enabled, each incoming translation, forwarded
fault and control command has its own request task. Coalesced waiters share
the initiating request's filter or prefix lookup pipeline and PTE reads.
Their milestones distinguish waiting for shared work from performing it.
Fault response milestones apply to the requesting device's waiters. Control
milestones identify protected walks draining, the final zeroing ACK for each
node, and acknowledged pointer and leaf writes. Each physical request has a
separate child task, including overlapping zeroing writes.

The integration tests wire real buffered ports, a memory controller and
storage. They include a two-MMU topology, a real TLB NACK/retry, timed STC
contention, pool reuse, and checkpoint replay during node initialization.
