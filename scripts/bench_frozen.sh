#!/usr/bin/env bash
# usage: scripts/bench_frozen.sh <commit> [runs=2] [target=8] [max_min=70] [snapshot=warrior_human_6]
# Benches with the learned files (journal/learn/fights.json, flee.json) put back as they were at
# <commit> before each run: tells a code change from the drift of what the bot learned. The files
# learned meanwhile are kept in logs/learn_after_frozen/ and the current ones restored at the end.
# Launch detached: setsid nohup scripts/bench_frozen.sh c151023 > /dev/null 2>&1 < /dev/null &
set -u
cd "$(dirname "$0")/.."
commit=$1; runs=${2:-2}; target=${3:-8}; maxmin=${4:-70}; snap=${5:-warrior_human_6}
# Both files must exist at <commit>: never bench with an empty learned table.
for f in fights flee; do
  git cat-file -e "$commit:journal/learn/$f.json" 2>/dev/null || { echo "no journal/learn/$f.json at $commit" >&2; exit 1; }
done
mkdir -p logs/learn_before_frozen logs/learn_after_frozen
cp journal/learn/fights.json journal/learn/flee.json logs/learn_before_frozen/
trap 'cp journal/learn/fights.json journal/learn/flee.json logs/learn_after_frozen/; cp logs/learn_before_frozen/*.json journal/learn/' EXIT
rm -f logs/bench_series.stop
echo "$(date '+%F %T') frozen learning from $commit" >> logs/bench_series.log
for i in $(seq 1 "$runs"); do
  [ -f logs/bench_series.stop ] && break
  for f in fights flee; do
    git show "$commit:journal/learn/$f.json" > "logs/learn_frozen_$f.json" && mv "logs/learn_frozen_$f.json" "journal/learn/$f.json" ||
      { echo "$(date '+%F %T') frozen: cannot restore $f.json from $commit" >> logs/bench_series.log; exit 1; }
  done
  scripts/bench_series.sh 1 "$target" "$maxmin" "$snap"
done
echo "$(date '+%F %T') frozen done" >> logs/bench_series.log
