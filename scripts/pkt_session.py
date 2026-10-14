"""Audit of a human play session captured by the server (PacketLogFile + .packetlog <name> on).
usage: python3 -I scripts/pkt_session.py logs/<file>.pkt > logs/<file>.txt
Reads the AzerothCore .pkt (PacketLog.cpp: 66-byte file header, then per packet 44-byte header and
payload) and prints a timeline (logins, levels, quests, deaths, rests) and a summary comparable to
the bench reports: time per level, quest order, kills by monster, multi-monster fights, rest, travel.
Quest and creature names come from acore_world (docker exec ac-database, read only)."""
import collections, struct, subprocess, sys, math

path = sys.argv[1]
data = open(path, "rb").read()
if data[:3] != b"PKT":
    sys.exit("not a .pkt file")
start_unix, start_ticks = struct.unpack_from("<II", data, 3 + 2 + 1 + 4 + 4 + 40)
off = 66

C, S = 0x47534d43, 0x47534d53
MOVE = set(range(0xB5, 0xEF))  # MSG_MOVE_* from the client
ev = []  # (t seconds, kind, value)
me = None
pos = []  # (t, x, y)
attackers = []  # (t, attacker guid)
swings = []  # t of our attack starts
sit = []  # (t, state)
casts = collections.Counter()
uses = []


def packed_guid(b, o):
    mask = b[o]; o += 1; g = 0
    for i in range(8):
        if mask & (1 << i):
            g |= b[o] << (8 * i); o += 1
    return g, o


while off + 44 <= len(data):
    direction, _conn, ticks, optsize, length = struct.unpack_from("<IIIII", data, off)
    opcode = struct.unpack_from("<I", data, off + 20 + optsize)[0]
    p = data[off + 24 + optsize: off + 20 + optsize + length]
    off += 20 + optsize + length
    t = ((ticks - start_ticks) & 0xFFFFFFFF) / 1000.0
    try:
        if direction == C:
            if opcode == 0x03D:
                me = struct.unpack_from("<Q", p)[0]; ev.append((t, "login", me & 0xFFFFFFFF))
            elif opcode == 0x189:
                ev.append((t, "accept", struct.unpack_from("<QI", p)[1]))
            elif opcode == 0x18E:
                ev.append((t, "turnin", struct.unpack_from("<QI", p)[1]))
            elif opcode == 0x15A:
                ev.append((t, "death", 0))
            elif opcode == 0x1D2:
                ev.append((t, "corpse", 0))
            elif opcode == 0x141:
                swings.append(t)
            elif opcode == 0x12E:
                casts[struct.unpack_from("<BI", p)[1]] += 1
            elif opcode == 0x0AB:
                uses.append(t)
            elif opcode == 0x101:
                sit.append((t, struct.unpack_from("<B", p)[0]))
            elif opcode in (0x1A0, 0x1A2):
                ev.append((t, "vendor", 0))
            elif opcode in MOVE and len(p) >= 20:
                _, o = packed_guid(p, 0)
                x, y = struct.unpack_from("<ff", p, o + 4 + 2 + 4)
                if abs(x) < 20000 and abs(y) < 20000:
                    pos.append((t, x, y))
        else:
            if opcode == 0x1D4:
                ev.append((t, "level", struct.unpack_from("<I", p)[0]))
            elif opcode == 0x1D0:
                victim, xp, kind = struct.unpack_from("<QIB", p)
                if kind == 0:
                    ev.append((t, "kill", (victim >> 24) & 0xFFFFFF))
            elif opcode == 0x143:
                a, v = struct.unpack_from("<QQ", p)
                if me and v == me:
                    attackers.append((t, a))
    except struct.error:
        pass


def sql(q):
    r = subprocess.run(["docker", "exec", "ac-database", "mysql", "-uroot", "-ppassword", "-N", "acore_world", "-e", q],
                       capture_output=True, text=True)
    return [l.split("\t") for l in r.stdout.splitlines() if l]


qids = sorted({v for _, k, v in ev if k in ("accept", "turnin")})
kids = sorted({v for _, k, v in ev if k == "kill"})
qname = {int(a): b for a, b in sql("select ID, LogTitle from quest_template where ID in (%s)" % ",".join(map(str, qids)))} if qids else {}
cname = {int(a): b for a, b in sql("select entry, name from creature_template where entry in (%s)" % ",".join(map(str, kids)))} if kids else {}
mm = lambda t: "%5.1f" % (t / 60)

print("# Session humaine", path, "(début", __import__("datetime").datetime.fromtimestamp(start_unix), ")")
print("\n## Chronologie (minutes depuis le début de la capture)")
for t, k, v in sorted(ev):
    if k == "kill":
        continue
    label = {"accept": "accepte", "turnin": "rend", "level": "NIVEAU", "death": "MORT (libère l'esprit)", "corpse": "récupère le corps",
             "login": "connexion perso guid", "vendor": "marchand (achat/vente)"}[k]
    extra = " %d %s" % (v, qname.get(v, "")) if k in ("accept", "turnin") else (" %d" % v if k in ("level", "login") else "")
    print(mm(t), label + extra)

print("\n## Résumé")
end = max([e[0] for e in ev] + [p[0] for p in pos] + [0])
print("- Durée capturée : %.1f min" % (end / 60))
lv = [(v, t) for t, k, v in ev if k == "level"]
print("- Niveaux (min) : " + ", ".join("%d: %.1f" % (v, t / 60) for v, t in lv))
print("- Quêtes rendues : %d ; acceptées : %d ; morts : %d" % (sum(1 for e in ev if e[1] == "turnin"), sum(1 for e in ev if e[1] == "accept"), sum(1 for e in ev if e[1] == "death")))
kills = collections.Counter(v for _, k, v in ev if k == "kill")
print("- Monstres tués (%d) : %s" % (sum(kills.values()), ", ".join("%s %d" % (cname.get(e, e), n) for e, n in kills.most_common())))
# Fights: distinct monsters that started attacking us within 15 s windows.
groups = collections.Counter()
i = 0
att = sorted(attackers)
while i < len(att):
    j, seen = i, set()
    while j < len(att) and att[j][0] - att[i][0] <= 15:
        seen.add(att[j][1]); j += 1
    groups[len(seen)] += 1
    i = j
print("- Monstres qui nous attaquent par fenêtre de 15 s (nombre de monstres: fois) : %s" % dict(sorted(groups.items())))
# Rest: sitting (stand state 1) until standing again.
rest, down = 0.0, None
for t, s in sit:
    if s == 1 and down is None:
        down = t
    elif s != 1 and down is not None:
        rest += t - down; down = None
print("- Repos assis : %.1f min ; objets utilisés (nourriture, potions...) : %d" % (rest / 60, len(uses)))
print("- Sorts lancés : %s" % dict(casts.most_common(10)))
# Travel: distance walked and time moving (samples less than 3 s apart).
dist, moving = 0.0, 0.0
for (t0, x0, y0), (t1, x1, y1) in zip(pos, pos[1:]):
    d = math.hypot(x1 - x0, y1 - y0)
    if t1 - t0 < 3 and d < 40:
        dist += d; moving += t1 - t0
print("- Distance parcourue : %.0f yards ; temps en mouvement : %.1f min (%.0f %% de la session)" % (dist, moving / 60, 100 * moving / max(end, 1)))
