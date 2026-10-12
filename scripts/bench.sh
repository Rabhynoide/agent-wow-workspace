#!/usr/bin/env bash
# usage: bench.sh [target_level=6] [max_minutes=60] [race=human] [class=warrior]
# Benchmark run: creates a fresh character, levels it with level.sh, then writes a report in
# journal/bench/ (one file per run + a line in results.csv) and deletes the character.
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
commit=$(git rev-parse --short HEAD)$(git diff --quiet HEAD -- module modules scripts || echo "+dirty")
scripts/rpc.sh session.logout '{}' >/dev/null 2>&1; sleep 5
agent-wow char create --name "$name" --race "$race" --class "$class" --gender male >/dev/null || { echo "create failed"; exit 1; }
scripts/start.sh "$name" || exit 1
combat_from=$(($(wc -l < logs/combat.log 2>/dev/null || echo 0) + 1))
state_from=$(($(wc -l < logs/state.log 2>/dev/null || echo 0) + 1))
start=$(date +%s)
scripts/level.sh "$name" "$target" "$maxmin" > "logs/bench_$name.out" 2>&1
code=$?
end=$(date +%s)
scripts/rpc.sh state.brief '{}' > "logs/bench_$name.state.json" 2>/dev/null
scripts/rpc.sh auto.stop '{}' >/dev/null 2>&1
python3 -I scripts/bench_report.py "$name" "$commit" "$race" "$class" "$target" "$start" "$end" "$code" "$combat_from" "$state_from"
scripts/rpc.sh session.logout '{}' >/dev/null 2>&1; sleep 5
agent-wow char delete "$name" --yes >/dev/null 2>&1 || echo "note: could not delete $name"
