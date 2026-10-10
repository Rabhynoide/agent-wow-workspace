#!/usr/bin/env bash
# usage: rpc.sh module.method ['{"json":"params"}'] [path]
# Prints the compact result (floats rounded to 0.1) or the error. Optional path keeps only part of
# the result, e.g. `rpc.sh state.brief '{}' self` or `... self.hp`.
curl -sS -m "${RPC_TIMEOUT:-60}" "http://127.0.0.1:${AGENT_WOW_WORLDRPC_PORT:-8086}/rpc" -H 'Content-Type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$1\",\"params\":${2:-{\}}}" | python3 -Ic '
import sys, json
def r(v):
    if isinstance(v, float): return round(v, 1)
    if isinstance(v, dict): return {k: r(x) for k, x in v.items()}
    if isinstance(v, list): return [r(x) for x in v]
    return v
d = json.load(sys.stdin)
out = d["result"] if "result" in d else d.get("error", d)
if "result" in d and len(sys.argv) > 1 and sys.argv[1]:
    for k in sys.argv[1].split("."):
        out = out[int(k)] if isinstance(out, list) else out.get(k)
print(json.dumps(r(out), separators=(",", ":")))' "${3:-}"
