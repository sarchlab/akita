# legacy.py: maps old result names to the naming rule, for
# `akita herd name apply`. Reads names on stdin, prints "old<TAB>new"
# ("new" empty when unknown).
#
# REPLACE: only if you have result directories named by an older scheme;
# otherwise delete this file and "legacy" in herd.json. Here old names
# are <workload>-<config>-<date>.
import sys

for line in sys.stdin:
    old = line.strip()
    parts = old.split("-")
    new = f"{parts[2]}_{parts[0]}_{parts[1]}" if len(parts) == 3 else ""
    print(f"{old}\t{new}")
