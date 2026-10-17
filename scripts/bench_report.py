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
deaths = sum(1 for l in read("logs/state.log")[state_from - 1:] if "msg=teleported" in l)
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
# Fled zones (combat/zones.go): spawn points the grind skipped, pull choices that left monsters there.
# Multi-target fights (combat/focus.go, near_attackers): target switches and spells cast with 2+ attackers.
switches = sum(1 for l in combat if "msg=\"switching target\"" in l)
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
         f"- Quêtes faites ({len(done)}) : {done}",
         f"- Quêtes sautées ({len(skips)}) :"] + [f"  - {s}" for s in skips] + [
         f"- Grinds : {grinds} ; passages marchand/achat : {vendor} ; repas : {meals} ; potions bues : {potions} ; objets de buff : {buff_items}",
         f"- Argent final : {me.get('money', '?')} cuivres ; durabilité : {state.get('durabilityPct', '?')} %",
         f"- Sorts refusés (sort/code) : {dict(fails.most_common(8))}",
         f"- Combats : {len(fights)} ; perdus : {len(lost)}"] + [f"  - {l}" for l in lost] + [
         f"- Fuites : {len(fled)} ; semées : {escaped}"] + [f"  - {l}" for l in fled] + [
         f"- Combats à plusieurs : {group_fights} ; changements de cible : {switches} ; sorts lancés à plusieurs : {dict(group_casts.most_common(8))}",
         f"- Taille de pull : {len(sizes)} changements, finale {sizes[-1] if sizes else '—'} ; pulls de groupe (monstres: nombre) : {dict(sorted(group_pulls.items()))}",
         f"- Zones fuies évitées : points d'apparition {zone_spawns} ; choix de pull {zone_pulls}",
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
