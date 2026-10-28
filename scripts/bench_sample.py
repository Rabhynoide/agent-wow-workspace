"""Activity sampler of a bench (scripts/bench.sh): every 5 s, what the bot is doing, from auto.status,
combat.status and move.status, one line per sample in logs/bench_<name>.samples ("<unix time> <activity>"), and each change of our money in
logs/bench_<name>.money ("<unix time> <copper> <activity>", where the money went: purchases, repairs, training).
bench_report.py turns them into the time split (fight, flight, rest, walk...). Read-only RPC calls.
usage: bench_sample.py <name> (stops when the file logs/bench_<name>.stop exists or after 4 h)"""
import json, os, sys, time, urllib.request

name = sys.argv[1]
root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
port = os.environ.get("AGENT_WOW_WORLDRPC_PORT", "8086")
out = os.path.join(root, "logs", f"bench_{name}.samples")
stop = os.path.join(root, "logs", f"bench_{name}.stop")
money_out = os.path.join(root, "logs", f"bench_{name}.money")


def rpc(method):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": {}}).encode()
    req = urllib.request.Request(f"http://127.0.0.1:{port}/rpc", body, {"Content-Type": "application/json"})
    try:
        return json.load(urllib.request.urlopen(req, timeout=4)).get("result") or {}
    except Exception:
        return None


def activity(auto, combat, move):
    if auto is None:
        return "hors_ligne"
    doing = (combat or {}).get("doing", "") if (combat or {}).get("state") == "running" else ""
    moving = (move or {}).get("state") == "moving"
    step = (auto or {}).get("step", "").lower()
    if doing == "fleeing":
        return "fuite"
    if doing in ("resting", "eating / drinking"):
        return "repos"
    if doing.startswith(("fighting", "defending", "body pulling")) or "fighting back" in step or "clearing the way" in step:
        return "approche" if moving and doing.startswith("fighting") else "combat"
    if doing == "looting":
        return "butin"
    if any(w in step for w in ("vendor", "buying", "sold", "repair", "restock")):
        return "marchand"
    if "train" in step:
        return "maitre"
    if moving:
        return "marche"
    if (auto or {}).get("state") != "running":
        return "attente"
    return "autre"


deadline = time.time() + 4 * 3600
last_money = None
with open(out, "a") as f, open(money_out, "a") as fm:
    while time.time() < deadline and not os.path.exists(stop):
        a = rpc("auto.status")
        c = rpc("combat.status") if a is not None else None
        m = rpc("move.status") if a is not None else None
        act = activity(a, c, m)
        f.write(f"{int(time.time())} {act}\n")
        f.flush()
        b = rpc("state.brief") if a is not None else None
        money = ((b or {}).get("self") or {}).get("money")
        if money is not None and money != last_money:
            fm.write(f"{int(time.time())} {money} {act} {(a or {}).get('step', '')[:80]!r}\n")
            fm.flush()
            last_money = money
        time.sleep(5)
