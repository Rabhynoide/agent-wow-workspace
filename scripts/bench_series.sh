#!/usr/bin/env bash
# usage: scripts/bench_series.sh <count> <target_level> <max_minutes> [snapshot]
# Runs <count> benches one after the other (scripts/bench.sh), without an agent: each one writes its
# report in journal/bench/ and a line in results.csv. Waits for a bench already running to end first.
# Progress in logs/bench_series.log. Launch it detached, e.g. 4 runs 6 → 8 from the level-6 snapshot:
#   setsid nohup scripts/bench_series.sh 4 8 70 warrior_human_6 > /dev/null 2>&1 < /dev/null &
# Stop: create logs/bench_series.stop (the running bench finishes, no new one starts).
set -u
cd "$(dirname "$0")/.."
count=${1:?count}; target=${2:?target level}; maxmin=${3:?max minutes}; snap=${4:-}
log=logs/bench_series.log
rm -f logs/bench_series.stop
echo "$(date '+%F %T') series: $count x bench $target ($maxmin min) from ${snap:-level 1}" >> "$log"
for i in $(seq 1 "$count"); do
  while pgrep -f "bash scripts/[b]ench.sh" >/dev/null; do sleep 30; done
  [ -f logs/bench_series.stop ] && { echo "$(date '+%F %T') stopped before run $i" >> "$log"; exit 0; }
  echo "$(date '+%F %T') run $i/$count" >> "$log"
  FROM="$snap" scripts/bench.sh "$target" "$maxmin" > logs/bench_run.out 2>&1 < /dev/null
  echo "$(date '+%F %T') run $i/$count ended: $(tail -1 journal/bench/results.csv)" >> "$log"
  sleep 20
done
echo "$(date '+%F %T') series done" >> "$log"
