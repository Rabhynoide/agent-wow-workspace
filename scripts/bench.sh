#!/usr/bin/env bash
# usage: [FROM=<snapshot>] [SAVE=<snapshot>] [SAVE_AT="<level>:<snapshot> ..."] bench.sh [target_level=6] [max_minutes=60] [race=human] [class=warrior]
# Benchmark run: creates a fresh character, levels it with level.sh, then writes a report in
# journal/bench/ (one file per run + a line in results.csv) and deletes the character.
# FROM: start from a saved character instead of level 1 (scripts/snapshot.sh; race and class are the
# snapshot's), e.g. FROM=warrior_human_6 bench.sh 10 120. SAVE: when the target is reached, save the
# character as that snapshot before deleting it. SAVE_AT: also save snapshots at levels on the way,
# e.g. SAVE_AT="6:warrior_human_6 8:warrior_human_8" bench.sh 10 240.
# Long: run it in the background; only read the last lines (the summary).
set -u
cd "$(dirname "$0")/.."
target=${1:-6}; maxmin=${2:-60}; race=${3:-human}; class=${4:-warrior}
# WoW names are letters only, no letter three times in a row: Bench + 6 random letters.
name=Bench$(python3 -Ic 'import random,string
s=""
while len(s)<6:
    c=random.choice(string.ascii_lowercase)
    if s[-2:]!=c*2: s+=c
print(s)')
# logout retries until the play process is gone (refused in combat): otherwise the character stays
# online and can be neither saved nor deleted.
logout() {
  for _ in 1 2 3 4 5 6 7 8 9 10 11 12; do
    pgrep -f "char [p]lay" >/dev/null || { sleep 3; return 0; }
    scripts/rpc.sh session.logout '{}' >/dev/null 2>&1; sleep 5
  done
  echo "note: logout failed"
}
commit=$(git rev-parse --short HEAD)$(git diff --quiet HEAD -- module modules scripts || echo "+dirty")
logout
from=${FROM:-}
if [ -n "$from" ]; then
  scripts/snapshot.sh load "$from" "$name" >/dev/null || { echo "snapshot load failed"; exit 1; }
else
  agent-wow char create --name "$name" --race "$race" --class "$class" --gender male >/dev/null || { echo "create failed"; exit 1; }
fi
start_level=$(docker exec ac-database mysql -uroot -ppassword -N -e "select level from acore_characters.characters where name='$name'" 2>/dev/null)
scripts/start.sh "$name" || exit 1
combat_from=$(($(wc -l < logs/combat.log 2>/dev/null || echo 0) + 1))
state_from=$(($(wc -l < logs/state.log 2>/dev/null || echo 0) + 1))
start=$(date +%s)
python3 -I scripts/bench_sample.py "$name" > /dev/null 2>&1 &  # time split for the report
# rested: alive and rested before a snapshot (the character can die between the level and the logout:
# warrior_human_6 was first saved dead).
rested() { scripts/job.sh auto.recover '{}' 300 >/dev/null 2>&1; scripts/job.sh combat.rest '{}' 120 >/dev/null 2>&1; }
# SAVE_AT="6:warrior_human_6 8:warrior_human_8": checkpoints on the way (a snapshot needs the
# character offline: logout, save, start again; the minutes keep running).
code=0
for stage in ${SAVE_AT:-}; do
  lv=${stage%%:*} snapname=${stage#*:}
  [ "$lv" -lt "$target" ] && [ "$lv" -gt "${start_level:-1}" ] || continue
  left=$(( maxmin - ($(date +%s) - start) / 60 ))
  [ $left -gt 0 ] || { code=2; break; }
  scripts/level.sh "$name" "$lv" "$left" >> "logs/bench_$name.out" 2>&1
  code=$?
  [ $code -eq 0 ] || break
  scripts/rpc.sh auto.stop '{}' >/dev/null 2>&1
  rested
  logout
  scripts/snapshot.sh save "$name" "$snapname" >> "logs/bench_$name.out" 2>&1
  scripts/start.sh "$name" >> "logs/bench_$name.out" 2>&1 || { code=1; break; }
done
if [ $code -eq 0 ]; then
  left=$(( maxmin - ($(date +%s) - start) / 60 ))
  if [ $left -gt 0 ]; then
    scripts/level.sh "$name" "$target" "$left" >> "logs/bench_$name.out" 2>&1
    code=$?
  else
    code=2
  fi
fi
end=$(date +%s)
touch "logs/bench_$name.stop"
scripts/rpc.sh state.brief '{}' > "logs/bench_$name.state.json" 2>/dev/null
scripts/rpc.sh auto.stop '{}' >/dev/null 2>&1
if [ -n "${SAVE:-}" ] && [ $code -eq 0 ]; then rested; fi
python3 -I scripts/bench_report.py "$name" "$commit" "$race" "$class" "$target" "$start" "$end" "$code" "$combat_from" "$state_from" "$from" "${start_level:-1}"
logout
if [ -n "${SAVE:-}" ] && [ $code -eq 0 ]; then scripts/snapshot.sh save "$name" "$SAVE"; fi
agent-wow char delete "$name" --yes >/dev/null 2>&1 || echo "note: could not delete $name"
