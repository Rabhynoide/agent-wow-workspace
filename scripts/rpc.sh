#!/usr/bin/env bash
# usage: rpc.sh method '{"json":"params"}'  -> prints compact result or error
curl -sS -m "${RPC_TIMEOUT:-60}" "http://127.0.0.1:${AGENT_WOW_WORLDRPC_PORT:-8086}/rpc" -H 'Content-Type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$1\",\"params\":${2:-{\}}}" | python3 -c 'import sys,json
d=json.load(sys.stdin); print(json.dumps(d.get("result",d.get("error")),separators=(",",":")))'
