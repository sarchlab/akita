#!/bin/sh
# sweep.sh CAMPAIGN SIM...: a campaign, every workload x config, each
# into results/CAMPAIGN/<date>_<workload>_<config>/.
#
# REPLACE: your sweep script, if you have one. Herd treats a result
# directory that holds the file named in herd.json campaigns.detect
# (here campaign.json) as a campaign, and plots its cells.
c=results/$1; date=${1%%_*}; shift
mkdir -p "$c" && echo '{"total": 6}' > "$c/campaign.json"
for w in fir mm bfs; do
  for k in base fast; do
    "$@" -steps 2 -workload $w -config $k -metrics "$c/${date}_${w}_$k/stats/metrics.csv" &&
      akita herd record "$c/${date}_${w}_$k" || exit 1
  done
done
