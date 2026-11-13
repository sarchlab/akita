"""Determinism and serial/parallel consistency checks (see issue #86).

For each acceptance program, this script:

1. runs it twice on the serial engine with tracing on and requires the two
   traces to be identical, IDs included (serial determinism);
2. runs it once on the parallel engine and requires its trace to match the
   serial trace once task and milestone IDs are removed (consistency). IDs are
   allowed to differ because the parallel engine hands them out in goroutine
   order.

With --race it instead builds each program with the race detector and runs a
small parallel simulation, failing on any reported data race.

Run from anywhere: python3 internal/tracecompare/determinism_test.py [--race]
"""

import argparse
import glob
import os
import shutil
import subprocess
import sys
import tempfile

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))

# Each entry: package directory, arguments for the determinism run, arguments
# for the race run, and which parallel-engine checks apply. Sizes keep the
# parallel runs, which are slower than serial ones, within a few minutes.
#
# Known exceptions (serial determinism is still checked for every program):
# - NoC endpoints act as the connection for their device ports but drain and
#   fill those ports from their own (primary) tick, in the same round as the
#   devices' ticks. Same-cycle ordering therefore differs between engines, so
#   the NoC programs skip the serial/parallel comparison until that is fixed.
# - The page-migration controller reads the agents' state directly from its
#   own tick, which races under the parallel engine, so it skips the race run.
BOTH = ("parallel", "race")
PROGRAMS = [
    ("mem/acceptancetests/writebackcache", "-seed 1 -num-access 3000",
     "-seed 1 -num-access 500", BOTH),
    ("mem/acceptancetests/writethroughcache", "-seed 1 -num-access 3000",
     "-seed 1 -num-access 500", BOTH),
    ("mem/acceptancetests/dram", "-seed 1 -num-access 3000",
     "-seed 1 -num-access 500", BOTH),
    ("mem/acceptancetests/virtualmem", "-seed 1 -num-access 2000",
     "-seed 1 -num-access 300", BOTH),
    ("mem/acceptancetests/sharedl2vm", "-seed 1 -num-access 1000",
     "-seed 1 -num-access 200", BOTH),
    ("mem/acceptancetests/pagemigration", "-seed 1 -num-access 1000",
     "-seed 1 -num-access 200", ("parallel",)),
    # The NoC programs take no size flags; these two send 1,000 messages,
    # which keeps each flit-level trace near 100-200 MB.
    ("noc/acceptance/pcie_p2p", "", "", ("race",)),
    ("noc/acceptance/dgx_single_p2p", "", "", ("race",)),
]


def run(cmd, cwd):
    print("+ " + " ".join(cmd), flush=True)
    return subprocess.run(cmd, cwd=cwd).returncode == 0


def build(pkg, out, race=False):
    cmd = ["go", "build"]
    if race:
        cmd.append("-race")
    cmd += ["-o", out, "./" + pkg]
    return run(cmd, ROOT)


def traced_run(binary, args, workdir, parallel):
    """Runs binary in a fresh directory and returns the trace database path."""
    os.makedirs(workdir)
    cmd = [binary, "-trace"] + args.split()
    if parallel:
        cmd.append("-parallel")
    if not run(cmd, workdir):
        return None

    dbs = glob.glob(os.path.join(workdir, "*.sqlite3"))
    if len(dbs) != 1:
        print("expected one trace database in %s, found %d" % (workdir, len(dbs)))
        return None
    return dbs[0]


def check_determinism(pkg, args, tmp, compare, check_parallel):
    name = os.path.basename(pkg)
    binary = os.path.join(tmp, name)
    if not build(pkg, binary):
        return False

    serial1 = traced_run(binary, args, os.path.join(tmp, name + "-serial1"), False)
    serial2 = traced_run(binary, args, os.path.join(tmp, name + "-serial2"), False)
    if not (serial1 and serial2):
        return False

    print("== %s: serial vs serial (exact)" % name, flush=True)
    ok = run([compare, serial1, serial2], ROOT)
    shutil.rmtree(os.path.dirname(serial2))
    if not check_parallel:
        print("== %s: serial vs parallel skipped" % name, flush=True)
        return ok

    parallel = traced_run(binary, args, os.path.join(tmp, name + "-parallel"), True)
    if not parallel:
        return False

    print("== %s: serial vs parallel (IDs ignored)" % name, flush=True)
    ok &= run([compare, "-ignore-ids", serial1, parallel], ROOT)
    return ok


def check_race(pkg, args, tmp):
    name = os.path.basename(pkg)
    binary = os.path.join(tmp, name + "-race")
    if not build(pkg, binary, race=True):
        return False

    workdir = os.path.join(tmp, name + "-race-run")
    os.makedirs(workdir)
    print("== %s: parallel under the race detector" % name, flush=True)
    return run([binary, "-parallel"] + args.split(), workdir)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--race", action="store_true",
                        help="run parallel simulations under the race detector")
    opts = parser.parse_args()

    failed = []
    with tempfile.TemporaryDirectory() as tmp:
        compare = os.path.join(tmp, "tracecompare")
        if not opts.race and not build("internal/tracecompare/cmd/tracecompare", compare):
            sys.exit(1)

        for pkg, args, race_args, checks in PROGRAMS:
            if opts.race:
                if "race" not in checks:
                    print("== %s: race run skipped" % os.path.basename(pkg))
                    continue
                ok = check_race(pkg, race_args, tmp)
            else:
                ok = check_determinism(pkg, args, tmp, compare,
                                       "parallel" in checks)
            if not ok:
                failed.append(pkg)

    if failed:
        print("FAILED: " + ", ".join(failed))
        sys.exit(1)
    print("All checks passed.")


if __name__ == "__main__":
    main()
