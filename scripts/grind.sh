#!/usr/bin/env bash
# usage: grind.sh <target_level> <creature_entry> [creature_entry...]
# Hunts the creatures in turn (4 kills each) until the level is reached: runs auto.grind, which sells,
# repairs, resurrects and trains like auto.run. Stops after 3 deaths or 25 minutes. Waits like job.sh.
set -u
dir=$(dirname "$0")
target=$1; shift
entries=$(printf '%s,' "$@"); entries=${entries%,}
exec "$dir/job.sh" auto.grind "{\"entries\":[$entries],\"until_level\":$target}" "${GRIND_MAX_S:-1560}"
