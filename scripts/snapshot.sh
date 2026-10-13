#!/usr/bin/env bash
# usage: scripts/snapshot.sh save <character> <snapshot>   (character offline)
#        scripts/snapshot.sh load <snapshot> <new name>
#        scripts/snapshot.sh list
# Character snapshots with the server's own player dump (.pdump write / .pdump load, over SOAP:
# scripts/soap.sh): items, spells, quests, talents, reputation, position. Kept in
# journal/snapshots/<snapshot>.pdump with the owning account in <snapshot>.account. Used by bench.sh
# (FROM=<snapshot>: start a run at a given level instead of level 1).
set -u
cd "$(dirname "$0")/.."
dir=journal/snapshots
mkdir -p "$dir"
db() { docker exec ac-database mysql -uroot -ppassword -N -e "$1" 2>/dev/null; }
case "${1:-}" in
save)
  char=$2 snap=$3
  [ "$(db "select online from acore_characters.characters where name='$char'")" = "0" ] || { echo "snapshot: $char is unknown or online"; exit 1; }
  # health 0 = dead body, player flag 0x10 = ghost: every run would start with a corpse run.
  [ "$(db "select health > 0 and playerFlags & 16 = 0 from acore_characters.characters where name='$char'")" = "1" ] || { echo "snapshot: $char is dead"; exit 1; }
  scripts/soap.sh "pdump write $snap.sql $char" || exit 1
  docker cp "ac-worldserver:/azerothcore/$snap.sql" "$dir/$snap.pdump" >/dev/null && docker exec ac-worldserver rm -f "/azerothcore/$snap.sql"
  db "select a.username from acore_auth.account a join acore_characters.characters c on c.account=a.id where c.name='$char'" > "$dir/$snap.account"
  echo "saved $char as $dir/$snap.pdump ($(db "select concat('level ', level, ', ', money, ' copper') from acore_characters.characters where name='$char'"))"
  ;;
load)
  snap=$2 name=$3
  [ -f "$dir/$snap.pdump" ] || { echo "snapshot: no $dir/$snap.pdump"; exit 1; }
  docker cp "$dir/$snap.pdump" "ac-worldserver:/azerothcore/$snap.sql" >/dev/null || exit 1
  out=$(scripts/soap.sh "pdump load $snap.sql $(cat "$dir/$snap.account") $name"); code=$?
  docker exec ac-worldserver rm -f "/azerothcore/$snap.sql"
  echo "$out"
  [ $code -eq 0 ] && [ -n "$(db "select guid from acore_characters.characters where name='$name'")" ] || { echo "snapshot: load failed"; exit 1; }
  ;;
list)
  for f in "$dir"/*.pdump; do [ -e "$f" ] && echo "$(basename "$f" .pdump) ($(cat "${f%.pdump}.account"))"; done
  ;;
*)
  sed -n 2,4p "$0"; exit 2
  ;;
esac
