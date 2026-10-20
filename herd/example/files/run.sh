#!/bin/sh
# run.sh NAME SIM...: one run of SIM, with its results in results/NAME/.
#
# REPLACE: how you start one simulation. NAME follows the naming rule of
# herd.json (<date>_<workload>_<config>), so the script reads the
# workload and the configuration from it. SIM is the simulator command
# ("go run ./sim" in a session, "{bin:sim}" in a queued task).
name=$1; shift
IFS=_ read -r date workload config <<END
$name
END
"$@" -workload "$workload" -config "$config" -metrics "results/$name/stats/metrics.csv" &&
  # Optional: write results/NAME/run.json, which the web UI shows.
  akita herd record "results/$name" "Command=$* -workload $workload -config $config"
