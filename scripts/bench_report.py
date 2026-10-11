"""Report of a bench.sh run: reads logs/level_<name>.log, logs/bench_<name>.out, the final state and
logs/combat.log (from a line), writes journal/bench/<date>_<name>.md and appends results.csv."""
import collections, datetime, json, os, re, sys

name, commit, race, cls, target, start, end, code, combat_from, state_from = sys.argv[1:11]
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
stops = [l.split("] ", 1)[-1] for l in out if "STOP" in l or "time limit" in l or "route finished" in l]
fails = collections.Counter(re.sub(r".*spell=(\d+) result=(\d+).*", r"\1/\2", l) for l in combat if "cast failed" in l)
meals = sum(1 for l in combat if "consuming" in l)
gave_up = sum(1 for l in combat if "giving up target" in l)
final_level = me.get("level", levels[-1][0] if levels else 1)
reached = final_level >= int(target)

date = datetime.datetime.fromtimestamp(start).strftime("%Y-%m-%d %H:%M")
lines = [f"# Bench {name} — {race} {cls} 1 → {target} ({date})", "",
         f"- Commit : {commit}",
         f"- Résultat : {'OBJECTIF ATTEINT' if reached else 'non atteint'} (niveau {final_level}, code level.sh {code}) en {minutes:.1f} min",
         f"- Temps par niveau (minutes depuis le début) : " + ", ".join(f"{lv}: {m:.1f}" for lv, m in levels),
         f"- Morts : {deaths} ; combats en marchant : {walk_fights} ; cibles abandonnées : {gave_up}",
         f"- Quêtes faites ({len(done)}) : {done}",
         f"- Quêtes sautées ({len(skips)}) :"] + [f"  - {s}" for s in skips] + [
         f"- Grinds : {grinds} ; passages marchand/achat : {vendor} ; repas : {meals}",
         f"- Argent final : {me.get('money', '?')} cuivres ; durabilité : {state.get('durabilityPct', '?')} %",
         f"- Sorts refusés (sort/code) : {dict(fails.most_common(8))}",
         f"- Fin : {stops[-1] if stops else '?'}"]
os.makedirs(os.path.join(root, "journal/bench"), exist_ok=True)
fname = f"journal/bench/{datetime.datetime.fromtimestamp(start).strftime('%Y%m%d_%H%M')}_{name}.md"
open(os.path.join(root, fname), "w").write("\n".join(lines) + "\n")
csv = os.path.join(root, "journal/bench/results.csv")
if not os.path.exists(csv):
    open(csv, "w").write("date,name,commit,race,class,target,reached,final_level,minutes,deaths,quests_done,quests_skipped,walk_fights,gave_up,meals,money\n")
open(csv, "a").write(f"{date},{name},{commit},{race},{cls},{target},{int(reached)},{final_level},{minutes:.1f},{deaths},{len(done)},{len(skips)},{walk_fights},{gave_up},{meals},{me.get('money', '')}\n")
print("\n".join(lines))
print(f"\n(report: {fname})")
