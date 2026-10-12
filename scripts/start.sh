#!/usr/bin/env bash
# usage: start.sh [character]   (default Claudia)
# Checks auth and the worldrpc port, starts `agent-wow char play` in the background if no session
# answers, and waits for "world session ready". Prints a single status line.
set -u
cd "$(dirname "$0")/.."
char=${1:-Claudia}
port=${AGENT_WOW_WORLDRPC_PORT:-8086}
if scripts/rpc.sh state.brief '{}' self.level 2>/dev/null | grep -qE '^[0-9]+$'; then
  echo "already running on port $port"; exit 0
fi
agent-wow auth status >/dev/null 2>&1 || { echo "auth invalid: run agent-wow auth login"; exit 1; }
if ss -ltn 2>/dev/null | grep -q ":$port "; then
  echo "port $port busy (another agent?): export AGENT_WOW_WORLDRPC_PORT=<free port> and rerun"; exit 1
fi
setsid nohup agent-wow char play "$char" --module-timeout 8m > logs/play.log 2>&1 < /dev/null &
for _ in $(seq 1 100); do
  sleep 5
  if grep -q "world session ready" logs/play.log; then
    # Each start rebuilds the module images: drop the replaced ones (dangling) and keep the build
    # cache under 2 GB, or they fill the disk (2026-10-12). Images in use are never touched.
    (docker image prune -f; docker builder prune -f --max-used-space 2gb) > logs/docker_prune.log 2>&1 &
    # Dashboard (reads the bot's logs on each refresh), on the LAN address only (not the VPN or the
    # Docker networks). Port 8790: free on this server (8090 neighbours many services, see NOTES).
    lan=$(ip -4 -o addr show "${DASHBOARD_IF:-ens192}" 2>/dev/null | awk '{split($4, a, "/"); print a[1]; exit}')
    if [ -n "$lan" ] && ! ss -ltn 2>/dev/null | grep -q ":8790 "; then
      setsid nohup python3 -I scripts/dashboard.py 8790 "$lan" > logs/dashboard.log 2>&1 < /dev/null &
    fi
    echo "ready ($char, port $port, dashboard http://${lan:-127.0.0.1}:8790)"; exit 0
  fi
  if ! pgrep -f "agent-wow char play $char" >/dev/null; then
    echo "char play exited:"; grep -iE "error|fail" logs/play.log | tail -5; exit 1
  fi
done
echo "not ready after 8 min, see logs/play.log"; exit 1
