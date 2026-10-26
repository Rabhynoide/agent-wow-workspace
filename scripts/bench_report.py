"""Report of a bench.sh run: reads logs/level_<name>.log, logs/bench_<name>.out, the final state and
logs/combat.log (from a line), writes journal/bench/<date>_<name>.md and appends results.csv."""
import collections, datetime, json, os, re, sys

name, commit, race, cls, target, start, end, code, combat_from, state_from = sys.argv[1:11]
snap = sys.argv[11] if len(sys.argv) > 11 else ""  # FROM: the run started from this snapshot
first_level = int(sys.argv[12]) if len(sys.argv) > 12 and sys.argv[12].isdigit() else 1
start, end, code, combat_from, state_from = int(start), int(end), int(code), int(combat_from), int(state_from)
root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
read = lambda p: open(os.path.join(root, p), errors="replace").read().splitlines() if os.path.exists(os.path.join(root, p)) else []
level_log = [l for l in read(f"logs/level_{name}.log") if l.strip()]
out = [l for l in read(f"logs/bench_{name}.out") if l.strip()]
combat = read("logs/combat.log")[combat_from - 1:]
try:
    state = json.load(open(os.path.join(root, f"logs/bench_{name}.state.json")))
except Exception:
    state = {}
me = state.get("self", {})

minutes = (end - start) / 60
ts = lambda l: datetime.datetime.strptime(l[:19], "%Y-%m-%d %H:%M:%S")
t0 = ts(level_log[0]) if level_log else None
levels = []  # (level, minutes since start)
for l in level_log:
    m = re.search(r"\] LEVEL (\d+)", l)
    if m and t0:
        levels.append((int(m.group(1)), (ts(l) - t0).total_seconds() / 60))
done = sorted({int(q) for l in level_log for m in re.finditer(r"done \[([\d ]+)\]", l) for q in m.group(1).split()})
skips = [re.sub(r"^.*SKIP ", "", l) for l in level_log if " SKIP " in l]
# Each death ends with a teleport to the graveyard (the bench never uses the hearthstone).
deaths = sum(1 for l in read("logs/state.log")[state_from - 1:] if "msg=teleported" in l and "dead=false" not in l)
walk_fights = sum(1 for l in level_log if "attacked on the way" in l)
vendor = sum(1 for l in level_log if "visiting a vendor" in l or "buying food" in l)
grinds = sum(1 for l in level_log if " grind to " in l)
stops = [l.split("] ", 1)[-1] for l in out + level_log if "STOP" in l or "time limit" in l or "route finished" in l or re.search(r"level \d+ reached", l)]
fails = collections.Counter(re.sub(r".*spell=(\d+) result=(\d+).*", r"\1/\2", l) for l in combat if "cast failed" in l)
meals = sum(1 for l in combat if "consuming" in l)
fights = [l for l in combat if 'msg=fight ' in l]
lost = [re.sub(r'.*?(target=.*)', r'\1', l) for l in fights if "result=died" in l]
# Flights (combat/flee.go): the "fleeing" line (health, loss expected, attackers), then "fled" (safe).
fled = [re.sub(r'.*?(hp=.*)', r'\1', l) for l in combat if "msg=fleeing " in l]
escaped = sum(1 for l in combat if "msg=fled " in l)
# Flights by health at their start, and deaths before the next "fled" (combat/dps.go, 2026-10-25: under
# 30 % 41 % of the flights died, over 45 % 4-9 %).
flight_hp = collections.defaultdict(lambda: [0, 0])
_start = None
for l in combat:
    if "msg=fleeing " in l:
        _start = int(re.search(r" hp=(\d+)", l).group(1))
    elif _start is not None and ("msg=fled " in l or "result=died" in l):
        b = "<30" if _start < 30 else "30-44" if _start < 45 else "45-74" if _start < 75 else "75+"
        flight_hp[b][0] += 1
        flight_hp[b][1] += "result=died" in l
        _start = None
# Fled zones (combat/zones.go): spawn points the grind skipped, pull choices that left monsters there.
# Multi-target fights (combat/focus.go, near_attackers): target switches and spells cast with 2+ attackers.
switches = sum(1 for l in combat if "msg=\"switching target\"" in l)
# Lost fights held to finish a monster before running (combat lateHold, 2026-10-25).
holds = sum(1 for l in combat if "late flee: holding" in l)
group_casts = collections.Counter(re.sub(r'.*spell="?([^"]+?)"? attackers.*', r'\1', l) for l in combat if "msg=\"group cast\"" in l)
group_fights = sum(1 for l in fights if not re.search(r"attackers_max=[01]\b", l))
zone_spawns = sum(1 for l in level_log if "near where we fled from" in l)
# Adaptive pull size (combat/pullsize.go): size changes and pulls of several monsters on purpose.
sizes = [m.group(1) + " (" + m.group(2).strip('"') + ")" for l in combat if (m := re.search(r'msg="pull size" size=(\d+) why=("[^"]*"|\S+)', l))]
group_pulls = collections.Counter(int(m.group(1)) for l in combat if (m := re.search(r'msg="group pull".* monsters=(\d+)', l)))
# Caution knobs in effect (journal/learn/tuning.json) and borderline flight decisions (combat/flee.go).
try:
    tuning = json.load(open(os.path.join(root, "journal/learn/tuning.json")))
except Exception:
    tuning = "défauts"
decisions = collections.Counter(re.sub(r'.*stay=(\w+) how=(\w+).*', r'\2/\1', l) for l in combat if 'msg="flee decision"' in l)
stays = [l for l in combat if 'msg="flee outcome"' in l and "stayed=true" in l]
stay_deaths = sum(1 for l in stays if "result=died" in l)
potions = sum(1 for l in combat if "drinking potion" in l)
buff_items = sum(1 for l in combat if "using buff item" in l)
zone_pulls = sum(1 for l in combat if "msg=\"pull choice\"" in l and "in_fled_zone=" in l and not re.search(r"in_fled_zone=0\b", l))
gave_up = sum(1 for l in combat if "giving up target" in l)
# Blows taken by side (front / back / unknown): blows, damage, dodged, parried, blocked, missed (melee.go).
blows = {k: [0] * 6 for k in ("front", "back", "unknown")}
for l in combat:
    if 'msg="blows taken"' in l:
        for k in blows:
            if m := re.search(k + r"=\"?\[([\d ]+)\]", l):
                blows[k] = [a + int(b) for a, b in zip(blows[k], m.group(1).split())]
def blow_line(k):
    b = blows[k]
    avoided = b[2] + b[3] + b[4] + b[5]
    return f"{k} {b[0]} coups ({b[1]} dégâts, évités {avoided} : esquive {b[2]}, parade {b[3]}, blocage {b[4]}, raté {b[5]})"
# Encounters (combat/encounter.go): monsters fought from entering combat to leaving it, and the result.
encounters = [m for l in combat if (m := re.search(r'msg=encounter foes=(\d+) s=(\d+) .*result=(\w+)', l))]
enc_by = collections.Counter((min(int(m.group(1)), 5), m.group(3)) for m in encounters)
def enc_line():
    if not encounters:
        return "—"
    parts = []
    for n in sorted({k[0] for k in enc_by}):
        parts.append(f"{n}{'+' if n == 5 else ''}: " + ", ".join(f"{r} {enc_by[(n, r)]}" for r in ("won", "fled", "died") if enc_by[(n, r)]))
    return f"{len(encounters)} ; par nombre de monstres : " + " ; ".join(parts)
defense = [re.sub(r'.*msg=defense ', '', l) for l in combat if "msg=defense " in l]
# Rests: one line each (combat rest): level, health from/to, seconds, food.
rests = [m for l in combat if (m := re.search(r'msg=rest level=(\d+) hp_from=(\d+) hp_to=(\d+) s=(\d+) ate=(\w+)', l))]
def rest_line(ate):
    r = [m for m in rests if m.group(5) == ate]
    if not r:
        return f"{'avec' if ate == 'true' else 'sans'} repas 0"
    gain = sum(int(m.group(3)) - int(m.group(2)) for m in r)
    secs = sum(int(m.group(4)) for m in r)
    return f"{'avec' if ate == 'true' else 'sans'} repas {len(r)} ({secs} s, {gain / max(secs, 1):.1f} %/s)"
def rest_by_level():
    by = collections.defaultdict(lambda: [0, 0])
    for m in rests:
        by[int(m.group(1))][0] += 1
        by[int(m.group(1))][1] += int(m.group(4))
    return " ; ".join(f"{lv}: {n} ({secs} s)" for lv, (n, secs) in sorted(by.items())) or "—"
rest_before_pull = sum(1 for l in combat if 'msg="resting before pull"' in l)
hopeless = sum(1 for l in combat if 'msg="flight hopeless: staying"' in l)
# Chests, vendors on the way, junk thrown away (auto/chest.go, auto/town.go, town/room.go, 2026-10-23):
# auto.log and town.log hold every run, keep the lines of this one by time.
def in_run(l):
    m = re.match(r"time=(\S+)", l)
    if not m:
        return False
    t = datetime.datetime.fromisoformat(m.group(1).replace("Z", "+00:00")).timestamp()
    return start <= t <= end
auto_log = [l for l in read("logs/auto.log") if in_run(l)]
town_log = [l for l in read("logs/town.log") if in_run(l)]
chests_seen = sum(1 for l in auto_log if 'msg="chest seen"' in l)
chests_opened = [re.sub(r'.*name="([^"]+)" items=(.*)', r'\1 \2', l) for l in auto_log if 'msg="chest opened"' in l]
chests_skipped = sum(1 for l in auto_log if 'msg="chest skipped"' in l or 'msg="chest not opened"' in l)
vendor_stops = sum(1 for l in auto_log if 'msg="vendor on the way"' in l)
destroyed = sum(1 for l in town_log if 'msg="destroyed to make room"' in l)
loot_left = sum(1 for l in combat if 'msg="loot left: bags full"' in l)
# Time split (scripts/bench_sample.py: one activity every 5 s).
samples = collections.Counter(l.split()[1] for l in read(f"logs/bench_{name}.samples") if len(l.split()) == 2)
n_samples = sum(samples.values())
split = ", ".join(f"{k} {100 * v / n_samples:.0f} % ({v * 5 / 60:.1f} min)" for k, v in samples.most_common()) if n_samples else "pas d'échantillons"
final_level = me.get("level", levels[-1][0] if levels else 1)
reached = final_level >= int(target)

date = datetime.datetime.fromtimestamp(start).strftime("%Y-%m-%d %H:%M")
lines = [f"# Bench {name} — {race} {cls} {first_level} → {target} ({date})", "",
         *( [f"- Départ : snapshot {snap} (niveau {first_level})"] if snap else [] ),
         f"- Commit : {commit}",
         f"- Résultat : {'OBJECTIF ATTEINT' if reached else 'non atteint'} (niveau {final_level}, code level.sh {code}) en {minutes:.1f} min",
         f"- Temps par niveau (minutes depuis le début) : " + ", ".join(f"{lv}: {m:.1f}" for lv, m in levels),
         f"- Répartition du temps : {split}",
         f"- Réglages : {json.dumps(tuning, separators=(',', ':')) if isinstance(tuning, dict) else tuning}",
         f"- Décisions de fuite limites (comment/resté) : {dict(decisions)} ; combats restés : {len(stays)}, morts : {stay_deaths}",
         f"- Morts : {deaths} ; combats en marchant : {walk_fights} ; cibles abandonnées : {gave_up}",
         f"- Coffres : {chests_seen} vus, {len(chests_opened)} ouverts, {chests_skipped} abandonnés" + (f" ({'; '.join(chests_opened)})" if chests_opened else ""),
         f"- Sacs : {vendor_stops} arrêts chez un vendeur en passant, {destroyed} piles jetées pour faire de la place, {loot_left} butins laissés (sacs pleins)",
         f"- Quêtes faites ({len(done)}) : {done}",
         f"- Quêtes sautées ({len(skips)}) :"] + [f"  - {s}" for s in skips] + [
         f"- Grinds : {grinds} ; passages marchand/achat : {vendor} ; repas : {meals} ; potions bues : {potions} ; objets de buff : {buff_items}",
         f"- Argent final : {me.get('money', '?')} cuivres ; durabilité : {state.get('durabilityPct', '?')} %",
         f"- Sorts refusés (sort/code) : {dict(fails.most_common(8))}",
         f"- Combats : {len(fights)} ; perdus : {len(lost)}"] + [f"  - {l}" for l in lost] + [
         f"- Fuites : {len(fled)} ; semées : {escaped} ; par vie au départ (fuites/morts) : " +
         " ; ".join(f"{b} % {flight_hp[b][0]}/{flight_hp[b][1]}" for b in ("<30", "30-44", "45-74", "75+") if b in flight_hp)] + [f"  - {l}" for l in fled] + [
         f"- Combats à plusieurs : {group_fights} ; changements de cible : {switches} ; combats perdus tenus pour finir un monstre : {holds} ; sorts lancés à plusieurs : {dict(group_casts.most_common(8))}",
         f"- Taille de pull : {len(sizes)} changements, finale {sizes[-1] if sizes else '—'} ; pulls de groupe (monstres: nombre) : {dict(sorted(group_pulls.items()))}",
         f"- Zones fuies évitées : points d'apparition {zone_spawns} ; choix de pull {zone_pulls}",
         f"- Coups reçus par côté : {' ; '.join(blow_line(k) for k in blows)}",
         f"- Rencontres (entrée → sortie de combat) : {enc_line()}",
         f"- Défense : {defense[-1] if defense else '—'}",
         f"- Repos : {rest_line('false')} ; {rest_line('true')} ; par niveau (repos, durée) : {rest_by_level()} ; avant un pull (rest_adaptive) : {rest_before_pull}",
         f"- Fuites jugées perdues d'avance (resté) : {hopeless}",
         f"- Fin : {stops[-1] if stops else '?'}"]
os.makedirs(os.path.join(root, "journal/bench"), exist_ok=True)
fname = f"journal/bench/{datetime.datetime.fromtimestamp(start).strftime('%Y%m%d_%H%M')}_{name}.md"
open(os.path.join(root, fname), "w").write("\n".join(lines) + "\n")
csv = os.path.join(root, "journal/bench/results.csv")
if not os.path.exists(csv):
    open(csv, "w").write("date,name,commit,race,class,target,reached,final_level,minutes,deaths,quests_done,quests_skipped,walk_fights,gave_up,meals,money,from,start_level\n")
open(csv, "a").write(f"{date},{name},{commit},{race},{cls},{target},{int(reached)},{final_level},{minutes:.1f},{deaths},{len(done)},{len(skips)},{walk_fights},{gave_up},{meals},{me.get('money', '')},{snap},{first_level}\n")
print("\n".join(lines))
print(f"\n(report: {fname})")
