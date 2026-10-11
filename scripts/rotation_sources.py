#!/usr/bin/env python3
"""Summarize the rotation sources kept in assets/sources (never committed) into journal/rotations/auto/:
one markdown file per class with
  - the Afenar WotLK WeakAuras: what each icon tracks (spell, buff/debuff on player or target) and
    its glow/desaturate thresholds;
  - the mod-playerbots strategies: trigger -> actions with their priority.
usage: scripts/rotation_sources.py   (from the workspace root)
"""
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SRC = os.path.join(ROOT, "assets", "sources")
OUT = os.path.join(ROOT, "journal", "rotations", "auto")

CLASSES = {  # our name -> (afenar file key, playerbots dir)
    "warrior": ("warrior", "Warrior"), "priest": ("priest", "Priest"), "mage": ("mage", "Mage"),
    "warlock": ("warlock", "Warlock"), "rogue": ("rogue", "Rogue"), "hunter": ("hunter", "Hunter"),
    "paladin": ("paladin", "Paladin"), "druid": ("druid", "Druid"), "shaman": ("shaman", "Shaman"),
    "deathknight": ("deathknight", "Dk"),
}


def afenar(path):
    d = json.load(open(path))
    wa = json.loads(d["json"])
    auras = [wa["d"]] + wa.get("c", [])
    by = {a["id"]: a for a in auras}
    lines = []

    def cond(c):
        ch = c.get("check", {})
        txt = json.dumps(ch, ensure_ascii=False)
        m = re.findall(r'"op": "([<>=]+)".*?"value": "?([\d.]+)"?.*?"variable": "(\w+)"', txt)
        changes = sorted({x.get("property", "") for x in c.get("changes", []) if isinstance(x, dict)})
        effect = "glow" if any("glow" in p for p in changes) else ("grey" if "desaturate" in changes else "")
        return [f"{v} {op} {val} -> {effect}" for op, val, v in m if effect]

    def walk(i, path):
        a = by.get(i)
        if not a:
            return
        rt = a.get("regionType")
        if rt in ("group", "dynamicgroup"):
            for ch in a.get("controlledChildren", []):
                walk(ch, path + [i])
            return
        tracked = []
        for k, v in a.get("triggers", {}).items():
            if not k.isdigit():
                continue
            t = v.get("trigger", {})
            if t.get("type") == "aura2":
                names = t.get("auranames") or t.get("auraspellids") or []
                if names:
                    tracked.append(f"aura {','.join(map(str, names))} on {t.get('unit', '?')}"
                                   + (" (missing)" if t.get("matchesShowOn") == "showOnMissing" else ""))
            elif t.get("type") == "spell" and t.get("spellName"):
                tracked.append(f"spell {t.get('realSpellName') or t.get('spellName')} ({t.get('event', '')})")
        thresholds = []
        for c in a.get("conditions", []):
            thresholds += cond(c)
        if tracked:
            lines.append(f"- **{i}** ({path[-1] if path else ''}): {'; '.join(tracked[:4])}"
                         + (f" — {'; '.join(sorted(set(thresholds))[:4])}" if thresholds else ""))

    walk(wa["d"]["id"], [])
    return lines


TRIG = re.compile(r'TriggerNode\(\s*"([^"]+)"\s*,\s*\{(.*?)\}\s*\)', re.S)
ACT = re.compile(r'NextAction\(\s*"([^"]+)"\s*,\s*([A-Z_]+(?:\s*[+-]\s*[\d.]+f?)?)')
DEF = re.compile(r'NextAction\(\s*"([^"]+)"\s*,\s*([A-Z_]+(?:\s*[+-]\s*[\d.]+f?)?)\s*\)')


def playerbots(d):
    lines = []
    sdir = os.path.join(d, "Strategy")
    for f in sorted(os.listdir(sdir)) if os.path.isdir(sdir) else []:
        if not f.endswith(".cpp"):
            continue
        src = open(os.path.join(sdir, f), encoding="utf-8", errors="replace").read()
        trig = TRIG.findall(src)
        defaults = []
        m = re.search(r"getDefaultActions\(\)\s*\{(.*?)\n\}", src, re.S)
        if m:
            defaults = [a for a, _ in DEF.findall(m.group(1))]
        if not trig and not defaults:
            continue
        lines.append(f"\n### {f[:-4]}")
        if defaults:
            lines.append(f"- par défaut : {', '.join(defaults)}")
        for name, body in trig:
            acts = [f"{a} ({p.replace(' ', '')})" for a, p in ACT.findall(body)]
            if acts:
                lines.append(f"- `{name}` → {', '.join(acts)}")
    return lines


def main():
    os.makedirs(OUT, exist_ok=True)
    wago = os.path.join(SRC, "wago")
    for cls, (af, pb) in CLASSES.items():
        out = [f"# {cls} — résumé automatique des sources (scripts/rotation_sources.py)", ""]
        files = [f for f in os.listdir(wago) if f.startswith(f"afenar_{af}_")] if os.path.isdir(wago) else []
        out.append("## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille")
        for f in files:
            out += afenar(os.path.join(wago, f))
        if not files:
            out.append("(source absente)")
        out += ["", "## mod-playerbots : déclencheur → actions (priorité)"]
        d = os.path.join(SRC, "playerbots_class", pb)
        out += playerbots(d) if os.path.isdir(d) else ["(source absente)"]
        with open(os.path.join(OUT, f"{cls}.md"), "w") as fh:
            fh.write("\n".join(out) + "\n")
        print(cls, len(out), "lines")


if __name__ == "__main__":
    sys.exit(main())
