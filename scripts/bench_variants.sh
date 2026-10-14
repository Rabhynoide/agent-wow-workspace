#!/usr/bin/env bash
# usage: scripts/bench_variants.sh [runs=2] [target=8] [max_min=70] [snapshot=warrior_human_6]
# A/B series of the caution knobs (NOTES « Protocole de mesure »): for each variant, writes
# journal/learn/tuning.json = defaults + one key changed, runs <runs> benches (bench_series.sh),
# then restores the original file. Progress in logs/bench_series.log. Launch it detached:
#   setsid nohup scripts/bench_variants.sh > /dev/null 2>&1 < /dev/null &
# Stop: touch logs/bench_series.stop (the running bench ends, the original tuning is restored).
set -u
cd "$(dirname "$0")/.."
runs=${1:-2}; target=${2:-8}; maxmin=${3:-70}; snap=${4:-warrior_human_6}
tune=journal/learn/tuning.json
cp "$tune" logs/tuning.json.orig
trap 'cp logs/tuning.json.orig "$tune"' EXIT
for variant in "A:" "B:postpone_after_flights=2" "C:crowd_flee=0" "D:chase_margin=5" "E:avoid=false"; do
  name=${variant%%:*} change=${variant#*:}
  python3 -I - "$change" logs/tuning.json.orig "$tune" <<'EOF'
import json, sys
change, src, dst = sys.argv[1:4]
t = json.load(open(src))
if change:
    k, v = change.split("=")
    t[k] = json.loads(v)
json.dump(t, open(dst, "w"), indent=1)
EOF
  echo "$(date '+%F %T') variant $name (${change:-reference})" >> logs/bench_series.log
  scripts/bench_series.sh "$runs" "$target" "$maxmin" "$snap"
  [ -f logs/bench_series.stop ] && break
done
echo "$(date '+%F %T') variants done" >> logs/bench_series.log
