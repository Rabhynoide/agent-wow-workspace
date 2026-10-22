#!/usr/bin/env python3
"""Bot dashboard: a local page that reads what the bot writes (logs, bench results, learned fights)
and its live state over the world RPC, on every refresh. Nothing to regenerate by hand.

usage: scripts/dashboard.py [port=8790] [host=127.0.0.1]
Started by scripts/start.sh on the LAN address of the server (http://192.168.100.7:8790). Read only:
GET / and /api/data, no action on the bot.
"""
import csv
import glob
import json
import os
import re
import struct
import sys
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RPC = "http://127.0.0.1:%s/rpc" % os.environ.get("AGENT_WOW_WORLDRPC_PORT", "8086")
FIELD = re.compile(r'(\w+)=("[^"]*"|\S+)')
SCORE = re.compile(r"score (\d+)\s+(.+?)\s+L\d+-\d+ spawns")
LEVEL = re.compile(r"\[(?:(\d+)m)?(\d+)s\] (?:L|LEVEL )(\d+)\b")


def path(*p):
    return os.path.join(ROOT, *p)


def tail(file, n, keep=None):
    """Last n lines of a file (only the ones keep accepts), read from the end."""
    try:
        with open(file, "rb") as f:
            f.seek(0, 2)
            size, block, data = f.tell(), 65536, b""
            while size > 0:
                step = min(block, size)
                size -= step
                f.seek(size)
                data = f.read(step) + data
                lines = data.decode("utf-8", "replace").splitlines()
                if keep:
                    lines = [l for l in lines if keep(l)]
                if len(lines) > n or size == 0:
                    return lines[-n:]
                block *= 2
    except OSError:
        return []
    return []


def rpc(method, params=None):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params or {}}).encode()
    req = urllib.request.Request(RPC, body, {"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=3) as r:
            return json.load(r).get("result")
    except Exception:
        return None


def current_run():
    logs = sorted(glob.glob(path("logs", "level_*.log")), key=os.path.getmtime)
    if not logs:
        return None
    log = logs[-1]
    name = os.path.basename(log)[len("level_"):-len(".log")]
    levels, names = {}, {}
    try:
        with open(log, encoding="utf-8", errors="replace") as f:
            for line in f:
                m = LEVEL.search(line)
                if m and m.group(3) not in levels:
                    levels[m.group(3)] = round(int(m.group(1) or 0) + int(m.group(2)) / 60, 1)
                m = SCORE.search(line)
                if m:
                    names[m.group(1)] = m.group(2)
    except OSError:
        pass
    # A new run restarts at L1: keep the level times of the last one only.
    return {"name": name, "updated": os.path.getmtime(log), "levels": levels, "names": names,
            "lines": tail(log, 30)}


def fights():
    out = []
    for line in tail(path("logs", "combat.log"), 40, lambda l: "msg=fight " in l):
        f = {k: v.strip('"') for k, v in FIELD.findall(line)}
        try:
            gap = int(f.get("target_level", 0)) - int(f.get("level", 0))
        except ValueError:
            gap = 0
        out.append({"time": f.get("time", "")[11:19], "result": f.get("result"), "target": f.get("target"),
                    "level": f.get("level"), "gap": gap, "hp_start": f.get("hp_start"), "hp_min": f.get("hp_min"),
                    "attackers": f.get("attackers_max"), "s": f.get("s")})
    return out[::-1]


def pull_size():
    """Adaptive pull size (combat/pullsize.go): the last changes, newest first."""
    out = []
    for line in tail(path("logs", "combat.log"), 8, lambda l: 'msg="pull size"' in l):
        f = {k: v.strip('"') for k, v in FIELD.findall(line)}
        out.append({"time": f.get("time", "")[11:19], "size": f.get("size"), "why": f.get("why"),
                    "attackers": f.get("attackers", ""), "hp_min": f.get("hp_min", ""), "bad": f.get("bad", "")})
    return out[::-1]


def bags_events():
    """Chests and bag room (auto/chest.go, auto/town.go, town/room.go): the last events, newest first."""
    keep = ('msg="chest opened"', 'msg="chest skipped"', 'msg="chest not opened"', 'msg="vendor on the way"')
    out = []
    for name, ok in (("auto.log", lambda l: any(k in l for k in keep)), ("town.log", lambda l: 'msg="destroyed to make room"' in l)):
        for line in tail(path("logs", name), 6, ok):
            f = {k: v.strip('"') for k, v in FIELD.findall(line)}
            out.append({"time": f.get("time", "")[11:19], "msg": f.get("msg", ""), "name": f.get("name") or f.get("entry", ""),
                        "items": f.get("items", ""), "err": f.get("err", "") or f.get("reason", ""), "copper": f.get("copper", "")})
    return sorted(out, key=lambda e: e["time"], reverse=True)[:8]


def learned():
    try:
        with open(path("journal", "learn", "fights.json")) as f:
            t = json.load(f)
    except (OSError, ValueError):
        return {"gaps": {}, "mobs": []}
    gaps = {g: {"n": round(c["n"]), "loss": round(c["loss_sum"] / c["n"]), "s": round(c["sec_sum"] / c["n"])}
            for g, c in t.get("classes", {}).get("1", {}).items() if c.get("n")}
    mobs = []
    for cls, entries in t.get("mobs", {}).items():
        for entry, by_gap in entries.items():
            tot = {"class": cls, "entry": entry, "n": 0, "deaths": 0, "chained": 0, "chained_deaths": 0, "fled": 0}
            for c in by_gap.values():
                for k in ("n", "deaths", "chained", "chained_deaths", "fled"):
                    tot[k] += c.get(k, 0)
            for k in ("n", "chained"):  # counts are scaled down past 40 fights (xpscore forget)
                tot[k] = round(tot[k])
            for k in ("deaths", "chained_deaths", "fled"):
                tot[k] = round(tot[k], 1)
            mobs.append(tot)
    mobs.sort(key=lambda m: (-m["deaths"], -m["n"]))
    return {"gaps": gaps, "mobs": mobs}


def benches():
    try:
        with open(path("journal", "bench", "results.csv"), newline="") as f:
            return list(csv.DictReader(f))[-20:]
    except OSError:
        return []


ICONS = None  # ItemDisplayInfo.dbc id -> icon name (assets/dbc, copied from the server)
ITEMS = {}  # item entry -> (world.item_info, time read)


def icons():
    global ICONS
    if ICONS is None:
        ICONS = {}
        try:
            with open(path("assets", "dbc", "ItemDisplayInfo.dbc"), "rb") as f:
                b = f.read()
            _, n, fields, size, _ = struct.unpack("<4s4I", b[:20])
            strings = 20 + n * size
            for i in range(n):
                rec = struct.unpack_from("<%dI" % fields, b, 20 + i * size)
                end = b.index(b"\0", strings + rec[5])  # field 5: InventoryIcon[0]
                if end > strings + rec[5]:
                    ICONS[rec[0]] = b[strings + rec[5]:end].decode("ascii", "replace").lower()
        except (OSError, ValueError, struct.error):
            pass
    return ICONS


def item(entry):
    """world.item_info, kept; read again after 10 min when the icon is missing (old world module)."""
    it, at = ITEMS.get(entry, (None, 0))
    if it is None or (not it.get("displayId") and time.time() - at > 600):
        r = rpc("world.item_info", {"entry": entry})
        if r and r.get("items"):
            it = r["items"][0]
            it["icon"] = icons().get(it.get("displayId", 0), "")
            ITEMS[entry] = (it, time.time())
    return it


def gear(snap):
    """Equipment and bags (state.snapshot: slot 0-18 worn, 19-22 bags, 23-38 backpack; bag 19-22 =
    inside an equipped bag), with each item's sheet for the tooltips."""
    if not snap:
        return None
    out = []
    for i in snap.get("items", []):
        bag, slot = i.get("bag", 0), i.get("bagSlot", 0)
        if bag == 0 and slot == 0 and not i.get("slot"):
            continue  # bank
        info = item(i["entry"]) or {"entry": i["entry"], "name": "objet %d" % i["entry"]}
        out.append({"bag": bag, "slot": slot, "count": i.get("count", 1), "dur": i.get("durability", 0),
                    "maxDur": i.get("maxDurability", 0), "info": info})
    return {"items": out}


QUESTS = {}
QLINE = re.compile(r"\[([\dhms]+)\] (.*)")


def quest(qid):
    """world.quest_info, kept (titles and objectives do not change)."""
    if qid not in QUESTS:
        r = rpc("world.quest_info", {"id": qid})
        if not r or not r.get("quests"):
            return {"id": qid, "title": "quête %d" % qid}
        QUESTS[qid] = r["quests"][0]
    return QUESTS[qid]


def route(log):
    """The quests of the run in the order met in the level log, each with its last event: planned
    ("L5 quests: 62 Title | ..."), done, postponed (to a level, or 4 min), skipped, death."""
    out, order = {}, []

    def ev(qid, status, detail, at, title=None):
        if qid not in out:
            out[qid] = {"id": qid, "title": title or "", "status": status, "detail": detail, "at": at}
            order.append(qid)
        else:
            out[qid].update({"status": status, "detail": detail, "at": at})
            if title:
                out[qid]["title"] = title
    try:
        with open(log, encoding="utf-8", errors="replace") as f:
            lines = f.readlines()
    except OSError:
        return []
    for line in lines:
        m = QLINE.search(line)
        if not m:
            continue
        at, msg = m.group(1), m.group(2).strip()
        if re.match(r"L\d+ quests: ", msg):
            for part in msg.split(": ", 1)[1].split(" | "):
                q = re.match(r"(\d+) (.*)", part.strip())
                if q and (int(q.group(1)) not in out or out[int(q.group(1))]["status"] == "planned"):
                    ev(int(q.group(1)), "planned", "", at, q.group(2))
        elif msg.startswith("done ["):
            for qid in re.findall(r"\d+", msg):
                ev(int(qid), "done", "", at)
        elif (q := re.match(r"DEATH on (\d+) .*?postponed to level (\d+): (.*)", msg)):
            ev(int(q.group(1)), "death", "mort, reportée au niveau %s : %s" % (q.group(2), q.group(3)), at)
        elif (q := re.match(r"(\d+) postponed to level (\d+): (.*)", msg)):
            ev(int(q.group(1)), "postponed", "niveau %s : %s" % (q.group(2), q.group(3)), at)
        elif (q := re.match(r"(\d+) .* postponed 4 min \((.*)\)", msg)):
            ev(int(q.group(1)), "waiting", "4 min (%s)" % q.group(2), at)
        elif (q := re.match(r"SKIP (\d+) (.*?): (.*)", msg)):
            ev(int(q.group(1)), "skipped", q.group(3), at, q.group(2))
    return [out[q] for q in order]


def quests(snap, run):
    """Quest log (state.snapshot) with each objective's progress (kill/object counters of the log,
    items counted in the bags), the quest auto works on, and the route history of the run."""
    auto = rpc("auto.status") or {}
    have = {}
    for i in (snap or {}).get("items", []):
        have[i["entry"]] = have.get(i["entry"], 0) + i.get("count", 1)
    log = []
    for q in (snap or {}).get("quests", []):
        info = quest(q["id"])
        counts = q.get("counts", [])
        objs = []
        for o in info.get("objectives", []):
            kind, need = o.get("kind", ""), o.get("count", 1) or 1
            if kind == "item":
                done = have.get(o.get("entry"), 0)
            elif kind in ("kill", "gameobject"):
                c = o.get("counter", 0)
                done = counts[c] if c < len(counts) else 0
            else:
                done = need if q.get("state") == 1 else 0
            objs.append({"kind": kind, "name": o.get("name") or kind, "done": min(done, need), "count": need})
        log.append({"id": q["id"], "title": info.get("title", ""), "level": info.get("level", 0),
                    "complete": q.get("state") == 1, "failed": q.get("state") == 2, "objectives": objs,
                    "text": info.get("objectivesText", "")})
    hist = route(path("logs", "level_%s.log" % run["name"])) if run else []
    return {"current": {"quest": auto.get("quest", 0), "step": auto.get("step", ""), "state": auto.get("state", "")},
            "log": log, "route": hist}


def data():
    brief = rpc("state.brief")
    combat = rpc("combat.status") or {}  # places fled from (combat/zones.go), avoided for 5 min
    snap = rpc("state.snapshot")
    run = current_run()
    return {"live": brief, "zones": combat.get("avoid", []), "gear": gear(snap), "doing": combat.get("doing", ""), "run": run,
            "quests": quests(snap, run), "fights": fights(), "pull_size": pull_size(), "bags": bags_events(), "learned": learned(), "benches": benches()}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path.startswith("/api/data"):
            body, kind = json.dumps(data()).encode(), "application/json"
        elif self.path in ("/", "/index.html"):
            with open(path("scripts", "dashboard.html"), "rb") as f:
                body, kind = f.read(), "text/html; charset=utf-8"
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", kind)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *a):  # logs/dashboard.log: who asked what (to check LAN access)
        if not self.path.startswith("/api/data"):
            sys.stderr.write("%s %s %s\n" % (self.log_date_time_string(), self.client_address[0], fmt % a))


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8790
    host = sys.argv[2] if len(sys.argv) > 2 else "127.0.0.1"
    ThreadingHTTPServer((host, port), Handler).serve_forever()
