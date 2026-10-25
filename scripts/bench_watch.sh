#!/usr/bin/env bash
# usage: scripts/bench_watch.sh [max_deaths=2] [stall_min=10]
# Watches the running bench and exits ONLY on an anomaly (or when the bench ends), so that an agent
# running it in the background is woken once, not polled: costs nothing while the run goes well.
# Anomalies: max_deaths deaths (teleports to the graveyard in logs/state.log), no new line in the
# level log for stall_min minutes, a STOP or a lost session. Prints one line saying why.
set -u
cd "$(dirname "$0")/.."
maxdeaths=${1:-2}; stall=$(( ${2:-10} * 60 ))
state_from=$(($(wc -l < logs/state.log 2>/dev/null || echo 0) + 1))
started=$(date +%s)
# The bench's level log: the newest one written after we started (wait up to 10 min for it: start.sh rebuilds the modules).
log=""
while [ -z "$log" ]; do
  for f in $(ls -t logs/level_Bench*.log 2>/dev/null); do
    n=$(basename "$f" .log); n=${n#level_}
    # Not a bench that already ended or was stopped by hand: its character must be playing.
    [ "$(stat -c %Y "$f")" -ge $((started - 120)) ] && [ ! -f "logs/bench_$n.state.json" ] && pgrep -f "char [p]lay $n " >/dev/null && log=$f
    break
  done
  [ -z "$log" ] && { [ $(( $(date +%s) - started )) -gt 600 ] && { echo "watch: no bench level log after 10 min"; exit 1; }; sleep 15; }
done
name=$(basename "$log" .log); name=${name#level_}
while true; do
  sleep 30
  # Between two SAVE_AT stages guidecheck is not running but bench.sh is (logout, snapshot, start).
  if [ -f "logs/bench_$name.state.json" ] || { ! pgrep -f "guidecheck -play -char $name" >/dev/null && ! pgrep -f "[b]ench.sh" >/dev/null; }; then
    echo "watch: bench $name ended ($(tail -1 "$log" | cut -c1-160))"; exit 0
  fi
  deaths=$(tail -n +"$state_from" logs/state.log 2>/dev/null | grep "msg=teleported" | grep -vc "dead=false")
  if [ "$deaths" -ge "$maxdeaths" ]; then
    echo "watch: $name died $deaths times ($(grep -E 'DEATH|died' "$log" | tail -1 | cut -c1-160))"; exit 1
  fi
  if grep -qE "STOP|disconnected|context canceled" "$log"; then
    echo "watch: $name stopped: $(grep -E 'STOP|disconnected|context canceled' "$log" | tail -1 | cut -c1-200)"; exit 1
  fi
  idle=$(( $(date +%s) - $(stat -c %Y "$log") ))
  if [ "$idle" -ge "$stall" ]; then
    echo "watch: $name silent for $((idle / 60)) min (last: $(tail -1 "$log" | cut -c1-160))"; exit 1
  fi
done
