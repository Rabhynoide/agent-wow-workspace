#!/usr/bin/env bash
# usage: grind.sh <target_level> <entry> [entry...]
# Cycles over the creature entries with combat.kill (4 kills each), recovers after a death and stops at
# the target level, after 3 deaths, or after 25 minutes. Prints one line per round.
set -u
dir=$(dirname "$0")
target=$1; shift
entries=("$@")
deaths=0; start=$(date +%s); round=0
brief() { "$dir/rpc.sh" state.brief '{}' self | python3 -Ic 'import sys,json;d=json.load(sys.stdin);print(d["level"],d["xp"],d["nextXp"],d["money"],"dead" if d.get("dead") else "alive")'; }
while :; do
  read -r lvl xp next money alive < <(brief)
  echo "[$(( $(date +%s) - start ))s] level $lvl xp $xp/$next money $money $alive deaths $deaths"
  [ "$lvl" -ge "$target" ] && { echo "target level reached"; exit 0; }
  [ "$deaths" -ge 3 ] && { echo "stopping: 3 deaths"; exit 1; }
  [ $(( $(date +%s) - start )) -ge 1500 ] && { echo "stopping: time limit"; exit 2; }
  if [ "$alive" = dead ]; then
    deaths=$((deaths+1)); "$dir/job.sh" auto.recover '{}' 300 | tail -1; "$dir/job.sh" combat.rest '{}' 120 | tail -1; continue
  fi
  e=${entries[$((round % ${#entries[@]}))]}; round=$((round+1))
  "$dir/job.sh" combat.kill "{\"entry\":$e,\"count\":4,\"radius\":150,\"max_s\":240}" 270 | tail -1 | cut -c1-200
  "$dir/job.sh" combat.rest '{}' 100 >/dev/null
done
