#!/usr/bin/env python3
"""Generate oracle configs+traces, run DRAMSim3 and Ramulator2, and write the
committed reference data.

This is the single source of truth for the differential experiment:
  * the canonical DDR4 parameters (CANONICAL),
  * the workload scenarios (SCENARIOS) -> also dumped to traces/scenarios.json
    so the Akita-side Go test drives the identical workload,
  * per-oracle config/trace emission, invocation, and stats parsing,
  * the normalized metric schema written to data/reference.csv.

Usage:
  python3 run_oracles.py [--dramsim3 PATH] [--ramulator2 PATH] [--workdir DIR]

Defaults look for the binaries built by oracles/build_oracles.sh
(oracles/.oracles/...). See oracles/README.md for the metric normalization and
why command *counts* (activates/reads/writes) are the exact cross-sim quantity
for these close-page scenarios while latency is timing-dependent.
"""
import argparse
import csv
import json
import os
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent

# Canonical DDR4-2400 parameters, mirroring mem/dram DDR4Spec (1 rank, 4 bank
# groups x 4 banks, x8, 8Gb-class geometry, BL8). Geometry is aligned so all
# three simulators model the same device; timing is aligned where the configs
# expose it. Close-page command counts are independent of exact timing.
CANONICAL = {
    "protocol": "DDR4",
    "bankgroups": 4,
    "banks_per_group": 4,
    "ranks": 1,
    "rows": 32768,
    "columns": 1024,
    "device_width": 8,
    "bus_width": 64,
    "BL": 8,
    "tCK_ns": 0.833,        # ~DDR4-2400
    "CL": 16, "CWL": 12, "AL": 0,
    "tRCD": 16, "tRP": 16, "tRAS": 39,
    "tRRD_S": 5, "tRRD_L": 7, "tFAW": 28,
    "tCCD_S": 4, "tCCD_L": 6,
    "tWTR_S": 4, "tWTR_L": 9, "tWR": 18, "tRTP": 9,
    "tRFC": 312, "tREFI": 9360, "tRTRS": 2,
    "cmd_queue_size": 8, "trans_queue_size": 32,
}

# channel_size (MB) that yields exactly `ranks` ranks in DRAMSim3:
#   device bits = rows*cols*banks*device_width ; rank = device_bytes * (bus/dev)
def _channel_size_mb():
    banks = CANONICAL["bankgroups"] * CANONICAL["banks_per_group"]
    dev_bits = CANONICAL["rows"] * CANONICAL["columns"] * banks * CANONICAL["device_width"]
    dev_bytes = dev_bits // 8
    rank_bytes = dev_bytes * (CANONICAL["bus_width"] // CANONICAL["device_width"])
    return (rank_bytes * CANONICAL["ranks"]) // (1024 * 1024)

# --- Workload scenarios --------------------------------------------------
# Each op is [is_write(0/1), address]. Close-page (auto-precharge) makes
# activates == #ops and reads/writes == the obvious split, independent of
# address mapping, so these are exact across all three simulators.
ROW_STRIDE = 0x20000   # 128 KiB: distinct rows in every mapping

# Refresh is a separate validation axis (roadmap P2) and a confound for command
# counts: DRAMSim3 idles for the full -c budget after the trace drains, firing
# many refreshes (one boundary even adds a stray activate). For these
# count-focused close-page scenarios we push refresh out of range so only
# access-driven commands are counted. (Ramulator2's tail-subtraction already
# cancels refresh; Akita's command counts are refresh-independent.)
REFRESH_OFF_TREFI = 100000000

def _seq(n, writes):
    return [[1 if writes else 0, i * ROW_STRIDE] for i in range(n)]

# First slice: pure-read and pure-write close-page scenarios. Command counts
# (activates == #ops, reads/writes == #ops of that type) are config- and
# mapping-independent here, so they compare exactly across all three
# simulators. Mixed-op and open-page (locality-dependent) scenarios are the
# next increment — they need either the upstream drain fix or aligned address
# encoders, tracked in the README.
SCENARIOS = [
    {"name": "cp_read_64",   "page_policy": "close", "ops": _seq(64, False)},
    {"name": "cp_read_256",  "page_policy": "close", "ops": _seq(256, False)},
    {"name": "cp_write_64",  "page_policy": "close", "ops": _seq(64, True)},
    {"name": "cp_write_256", "page_policy": "close", "ops": _seq(256, True)},
]


def expected_counts(scn):
    reads = sum(1 for w, _ in scn["ops"] if w == 0)
    writes = sum(1 for w, _ in scn["ops"] if w == 1)
    return {"activates": len(scn["ops"]), "reads": reads, "writes": writes}


# --- DRAMSim3 ------------------------------------------------------------
def dramsim3_ini(scn):
    c = CANONICAL
    row_policy = "CLOSE_PAGE" if scn["page_policy"] == "close" else "OPEN_PAGE"
    return f"""[dram_structure]
protocol = {c['protocol']}
bankgroups = {c['bankgroups']}
banks_per_group = {c['banks_per_group']}
rows = {c['rows']}
columns = {c['columns']}
device_width = {c['device_width']}
BL = {c['BL']}

[timing]
tCK = {c['tCK_ns']}
AL = {c['AL']}
CL = {c['CL']}
CWL = {c['CWL']}
tRCD = {c['tRCD']}
tRP = {c['tRP']}
tRAS = {c['tRAS']}
tRFC = {c['tRFC']}
tREFI = {REFRESH_OFF_TREFI}
tRRD_S = {c['tRRD_S']}
tRRD_L = {c['tRRD_L']}
tWTR_S = {c['tWTR_S']}
tWTR_L = {c['tWTR_L']}
tFAW = {c['tFAW']}
tWR = {c['tWR']}
tRTP = {c['tRTP']}
tCCD_S = {c['tCCD_S']}
tCCD_L = {c['tCCD_L']}
tRTRS = {c['tRTRS']}

[system]
channel_size = {_channel_size_mb()}
channels = 1
bus_width = {c['bus_width']}
address_mapping = rochrababgco
queue_structure = PER_BANK
refresh_policy = RANK_LEVEL_STAGGERED
row_buf_policy = {row_policy}
cmd_queue_size = {c['cmd_queue_size']}
trans_queue_size = {c['trans_queue_size']}

[other]
epoch_period = 1587301
output_level = 1
"""


def dramsim3_trace(scn):
    return "".join(f"0x{addr:X} {'WRITE' if w else 'READ'} 0\n"
                   for w, addr in scn["ops"])


def run_dramsim3(binary, scn, work):
    ini = work / f"{scn['name']}.ini"
    trace = work / f"{scn['name']}.ds3.trace"
    out = work / f"ds3_{scn['name']}"
    out.mkdir(parents=True, exist_ok=True)
    ini.write_text(dramsim3_ini(scn))
    trace.write_text(dramsim3_trace(scn))
    cycles = max(100000, len(scn["ops"]) * 4000)
    subprocess.run([binary, str(ini), "-c", str(cycles), "-t", str(trace),
                    "-o", str(out)], check=True,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    stats = json.loads((out / "dramsim3.json").read_text())["0"]
    return {
        "activates": int(stats["num_act_cmds"]),
        "reads": int(stats["num_read_cmds"]),
        "writes": int(stats["num_write_cmds"]),
        "avg_read_latency": float(stats.get("average_read_latency", 0.0)),
    }


# --- Ramulator2 ----------------------------------------------------------
def ramulator2_yaml(scn, trace_path, cmdcount_path):
    c = CANONICAL
    row_policy = ("ClosedRowPolicy" if scn["page_policy"] == "close"
                  else "OpenRowPolicy")
    cap = "\n      cap: 1" if scn["page_policy"] == "close" else ""
    return f"""Frontend:
  impl: LoadStoreTrace
  path: {trace_path}
  clock_ratio: 8
MemorySystem:
  impl: GenericDRAM
  clock_ratio: 3
  DRAM:
    impl: DDR4
    org:
      preset: DDR4_4Gb_x8
      channel: 1
      rank: {c['ranks']}
    timing:
      preset: DDR4_2400R
  Controller:
    impl: Generic
    Scheduler: {{impl: FRFCFS}}
    RefreshManager: {{impl: AllBank}}
    RowPolicy:
      impl: {row_policy}{cap}
    plugins:
      - ControllerPlugin:
          impl: CommandCounter
          path: {cmdcount_path}
          commands_to_count: [ACT, PRE, RD, WR, RDA, WRA, REFab]
  AddrMapper:
    impl: RoBaRaCoCh
"""


# Ramulator2's main loop stops as soon as the frontend has *injected* every
# request (src/main.cpp breaks on frontend->is_finished()); it does not drain
# memory. So commands still queued at injection-end go uncounted. To recover the
# exact per-scenario command counts we use tail-subtraction: run the real ops
# followed by a long fixed drain-suffix (so the real ops fully drain), then run
# the suffix alone; the identical trailing drain-deficit cancels in the
# difference. Verified to recover exact counts.
SUFFIX_ADDR = 0x40000000


def ramulator2_trace_lines(ops):
    return "".join(f"{'ST' if w else 'LD'} 0x{addr:X}\n" for w, addr in ops)


def _ram2_counts(binary, ops, work, tag):
    trace = work / f"{tag}.ram2.trace"
    cmdcount = work / f"{tag}.cmdcount.csv"
    yaml = work / f"{tag}.yaml"
    trace.write_text(ramulator2_trace_lines(ops))
    # page_policy carried via a throwaway scn dict (close-page for all here)
    yaml.write_text(ramulator2_yaml({"page_policy": "close"}, trace, cmdcount))
    subprocess.run([binary, "-f", str(yaml)], check=True,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    counts = {}
    for line in cmdcount.read_text().splitlines():
        if "," in line:
            name, val = line.split(",")
            counts[name.strip()] = int(val.strip())
    return counts


def run_ramulator2(binary, scn, work):
    # Type-matched drain suffix: the suffix op type matches the scenario's so
    # that type's queue deficit at sim-end is identical between the combined and
    # tail runs and cancels exactly. (A write scenario needs write suffix to
    # cycle the write-drain watermark; a read scenario needs a read suffix so
    # the read deficit cancels.) Requires pure (single-type) scenarios.
    op_types = {w for w, _ in scn["ops"]}
    assert len(op_types) == 1, "ramulator2 tail-subtraction needs pure scenarios"
    op_type = op_types.pop()
    suffix_len = len(scn["ops"]) + 400
    suffix = [[op_type, SUFFIX_ADDR] for _ in range(suffix_len)]
    combined = _ram2_counts(binary, scn["ops"] + suffix, work, f"{scn['name']}_comb")
    tail = _ram2_counts(binary, suffix, work, f"{scn['name']}_tail")

    def diff(*keys):
        return sum(combined.get(k, 0) - tail.get(k, 0) for k in keys)

    return {
        "activates": diff("ACT"),
        "reads": diff("RD", "RDA"),
        "writes": diff("WR", "WRA"),
        # Latency is not recoverable through tail-subtraction; left to the
        # fully-draining simulators (DRAMSim3 / Akita) in this slice.
        "avg_read_latency": None,
    }


def main():
    ap = argparse.ArgumentParser()
    oracles = HERE / "oracles" / ".oracles"
    ap.add_argument("--dramsim3",
                    default=str(oracles / "DRAMsim3" / "build" / "dramsim3main"))
    ap.add_argument("--ramulator2",
                    default=str(oracles / "ramulator2" / "build" / "ramulator2"))
    ap.add_argument("--workdir", default=str(HERE / "data" / "raw"))
    args = ap.parse_args()

    work = Path(args.workdir)
    work.mkdir(parents=True, exist_ok=True)

    # Dump the shared workload so the Go test drives the identical scenarios.
    (HERE / "traces").mkdir(exist_ok=True)
    (HERE / "traces" / "scenarios.json").write_text(
        json.dumps({"canonical": CANONICAL, "scenarios": SCENARIOS}, indent=2) + "\n")

    rows = []
    for scn in SCENARIOS:
        exp = expected_counts(scn)
        for sim, runner, binary in (
            ("dramsim3", run_dramsim3, args.dramsim3),
            ("ramulator2", run_ramulator2, args.ramulator2),
        ):
            if not os.path.exists(binary):
                print(f"!! {sim} binary not found: {binary}\n"
                      f"   build it with oracles/build_oracles.sh", file=sys.stderr)
                return 2
            got = runner(binary, scn, work)
            for k in ("activates", "reads", "writes"):
                if got[k] != exp[k]:
                    print(f"WARN {sim}/{scn['name']}: {k} {got[k]} != expected "
                          f"{exp[k]}", file=sys.stderr)
            lat = got["avg_read_latency"]
            rows.append({
                "scenario": scn["name"], "simulator": sim,
                "page_policy": scn["page_policy"], "num_ops": len(scn["ops"]),
                "activates": got["activates"], "reads": got["reads"],
                "writes": got["writes"],
                "avg_read_latency_cycles": "" if lat is None else round(lat, 3),
            })
            print(f"ok  {sim:11s} {scn['name']:12s} "
                  f"ACT={got['activates']} RD={got['reads']} WR={got['writes']} "
                  f"lat={'n/a' if lat is None else f'{lat:.2f}'}")

    out_csv = HERE / "data" / "reference.csv"
    with out_csv.open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0].keys()))
        w.writeheader()
        w.writerows(rows)
    print(f"\nwrote {out_csv} ({len(rows)} rows)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
