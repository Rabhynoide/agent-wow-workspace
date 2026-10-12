#!/usr/bin/env bash
# usage: job.sh module.method ['{"json":"params"}'] [max_seconds]
# Starts an async job (auto.run, auto.recover, combat.kill, combat.rest, move.goto...) or resumes waiting
# on one (auto.status, combat.status, move.status), then polls <module>.status until it ends.
# Prints one line per meaningful change and the final status: one tool call instead of dozens.
# Exit code: 0 done/arrived, 1 failed/stopped, 2 still running after max_seconds (rerun with <module>.status).
set -u
dir=$(dirname "$0")
method=$1 params=${2:-{\}} max=${3:-540}
mod=${method%%.*}
# Inject wait_s=25 so the first call blocks as long as allowed.
params=$(python3 -Ic 'import sys,json;p=json.loads(sys.argv[1]);p.setdefault("wait_s",25);print(json.dumps(p))' "$params")
# final prints the last status without the progress list (its events were printed as they came).
final() {
  python3 -Ic '
import sys, json
try:
    d = json.loads(sys.argv[1]); d.pop("progress", None)
    print("final: " + json.dumps(d, separators=(",", ":")))
except Exception:
    print("final: " + sys.argv[1])' "$1"
}
start=$(date +%s)
out=$(RPC_TIMEOUT=40 "$dir/rpc.sh" "$method" "$params")
last=""
seen=0 # progress events already printed (the list only grows: each new one is printed once)
while :; do
  line=$(python3 -Ic '
import sys, json
d = json.loads(sys.argv[1])
if not isinstance(d, dict) or "state" not in d:
    print("ERR " + sys.argv[1]); sys.exit()
noise = {"elapsedS", "etaS", "x", "y", "z", "o", "distLeft", "hpPct", "manaPct", "coin", "xp", "pathLen", "progress"}
print(d["state"] + " " + json.dumps({k: v for k, v in d.items() if k not in noise and k != "state"}, separators=(",", ":")))
' "$out")
  state=${line%% *}
  [ "$line" != "$last" ] && echo "[$(( $(date +%s) - start ))s] $line"
  last=$line
  news=$(python3 -Ic '
import sys, json
try:
    p = json.loads(sys.argv[1]).get("progress") or []
except Exception:
    p = []
for e in p[int(sys.argv[2]):]:
    print("  + " + e)
print(len(p))
' "$out" "$seen")
  seen=$(printf "%s\n" "$news" | tail -n 1)
  printf "%s\n" "$news" | sed '$d'
  case $state in
    running|moving) ;;
    done|arrived|idle) final "$out"; exit 0 ;;
    *) final "$out"; exit 1 ;;
  esac
  if [ $(( $(date +%s) - start )) -ge "$max" ]; then echo "still running: rerun scripts/job.sh $mod.status"; exit 2; fi
  out=$(RPC_TIMEOUT=40 "$dir/rpc.sh" "$mod.status" '{"wait_s":25}')
done
