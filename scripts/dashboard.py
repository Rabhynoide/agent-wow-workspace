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


def learned():
    try:
        with open(path("journal", "learn", "fights.json")) as f:
            t = json.load(f)
    except (OSError, ValueError):
        return {"gaps": {}, "mobs": []}
    gaps = {g: {"n": c["n"], "loss": round(c["loss_sum"] / c["n"]), "s": round(c["sec_sum"] / c["n"])}
            for g, c in t.get("classes", {}).get("1", {}).items() if c.get("n")}
    mobs = []
    for cls, entries in t.get("mobs", {}).items():
        for entry, by_gap in entries.items():
            tot = {"class": cls, "entry": entry, "n": 0, "deaths": 0, "chained": 0, "chained_deaths": 0, "fled": 0}
            for c in by_gap.values():
                for k in ("n", "deaths", "chained", "chained_deaths", "fled"):
                    tot[k] += c.get(k, 0)
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


def gear():
    """Equipment and bags (state.snapshot: slot 0-18 worn, 19-22 bags, 23-38 backpack; bag 19-22 =
    inside an equipped bag), with each item's sheet for the tooltips."""
    snap = rpc("state.snapshot")
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


def data():
    brief = rpc("state.brief")
    combat = rpc("combat.status") or {}  # places fled from (combat/zones.go), avoided for 5 min
    return {"live": brief, "zones": combat.get("avoid", []), "gear": gear(), "doing": combat.get("doing", ""), "run": current_run(),
            "fights": fights(), "learned": learned(), "benches": benches()}


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
