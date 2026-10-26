#!/usr/bin/env python3
"""Builds the route files of the Warcraft Tavern Alliance leveling guide (WotLK Classic, by Luxrah,
https://www.warcrafttavern.com/wotlk/guides/alliance-leveling/) in the shape guidecheck reads.

guidecheck -play does each quest whole (accept, objectives, turn-in), so a quest is listed where the
guide turns it in when it is accepted on the way (deliveries), except when its giver is far behind.
"gate N" is a "reach this level" step. Skipped steps are commented with the reason.
Run: python3 routes/warcrafttavern/build.py (writes the json files next to it).
"""
import json, os

HERE = os.path.dirname(os.path.abspath(__file__))
URL = "https://www.warcrafttavern.com/wotlk/guides/"

ROUTES = [
    ("alliance_01_human_1_10.json", "Human", "Elwynn Forest & Dun Morogh (1-10)", "1-10", 1, "alliance-leveling-human-1-10", [
        # Northshire
        (783, "A Threat Within"), (7, "Kobold Camp Cleanup"), (5261, "Eagan Peltskinner"),
        (33, "Wolves Across the Border"), (15, "Investigate Echo Ridge"), (18, "Brotherhood of Thieves"),
        (3100, "Simple Letter", "Warrior")  # other classes: their own letter, not listed yet
       , (21, "Skirmish at Echo Ridge"), (3903, "Milly Osworth"),
        (6, "Bounty on Garrick Padfoot"), (3904, "Milly's Harvest"), (3905, "Grape Manifest"),
        # Given in Northshire, turned in at Goldshire on the way to Stormwind (the guide keeps them for
        # level 10; the giver is behind us by then).
        (54, "Report to Goldshire"), (2158, "Rest and Relaxation"),
        # Dun Morogh: Kharanos (Beer Basted Boar Ribs skipped: Cooking and a bought Rhapsody Malt)
        (400, "Tools for Steelgrill"), (5541, "Ammo for Rumbleshot"), (317, "Stocking Jetsteam"),
        (313, "The Grizzled Den"), (318, "Evershine"),
        # Brewnall Village (Bitter Rivals, Distracting Jarven, Guarded Thunderbrew Barrel, Return to
        # Marleth skipped: a bought Thunder Ale and a scripted barrel)
        (319, "A Favor for Evershine"), (315, "The Perfect Stout"), (320, "Return to Bellowfiz"),
        (412, "Operation Recombobulation"), (413, "Shimmer Stout"),
        # Gol'Bolar Quarry, North Gate
        (314, "Protecting the Herd"), (433, "The Public Servant"), (432, "Those Blasted Troggs!"),
        (419, "The Lost Pilot"), (417, "A Pilot's Revenge"),
        # Loch Modan, then Ironforge and Stormwind by the tram
        (414, "Stout to Kadrell"), (6661, "Deeprun Rat Roundup"), (6662, "Me Brother, Nipsy"),
        (399, "Humble Beginnings"),
    ]),
    ("alliance_07_elwynn_10_12.json", "", "Elwynn Forest (10-12)", "10-12", 10, "alliance-leveling-10-12", [
        "gate 10",
        # Goldshire
        (47, "Gold Dust Exchange"), (60, "Kobold Candles"), (62, "The Fargodeep Mine"),
        (40, "A Fishy Peril"), (35, "Further Concerns"),
        (85, "Lost Necklace"), (88, "Princess Must Die!"), (86, "Pie for Billy"), (106, "Young Lovers"),
        (84, "Back to Billy"), (111, "Speak with Gramma"), (107, "Note to William"), (87, "Goldtooth"),
        # Eastvale Logging Camp
        (76, "The Jasperlode Mine"), (112, "Collecting Kelp"), (37, "Find the Lost Guards"),
        (52, "Protect the Frontier"), (45, "Discover Rolf's Fate"), (71, "Report to Thomas"),
        (5545, "A Bundle of Trouble"), (83, "Red Linen Goods"), (39, "Deliver Thomas' Report"),
        # Forest's Edge (Wanted: "Hogger" skipped: elite)
        (114, "The Escape"), (239, "Westbrook Garrison Needs Help!"), (11, "Riverpaw Gnoll Bounty"),
        # Deliveries to Stormwind (the guide turns them in from Westfall by flight)
        (61, "Shipment to Stormwind"), (1097, "Elmore's Task"),
    ]),
    ("alliance_08_westfall_12_15.json", "", "Westfall (12-15)", "12-15", 12, "alliance-leveling-12-15", [
        "gate 12",
        (109, "Report to Gryan Stoutmantle"),
        (36, "Westfall Stew"), (64, "The Forgotten Heirloom"), (151, "Poor Old Blanchy"),
        (38, "Westfall Stew"), (22, "Goretusk Liver Pie"), (9, "The Killing Fields"),
        (12, "The People's Militia"), (102, "Patrolling Westfall"), (6181, "A Swift Message"),
        (153, "Red Leather Bandanas"), (13, "The People's Militia"), (14, "The People's Militia"),
        (6281, "Continue to Stormwind"), (6261, "Dungar Longdrink"),
        (353, "Stormpike's Delivery"), (2039, "Find Bingles"),
    ]),
]

for fname, race, zone, levels, start, page, items in ROUTES:
    steps = []
    for it in items:
        if isinstance(it, str):
            steps.append({"n": len(steps) + 1, "action": "note", "level": it.split()[1], "note": "reach this level"})
        else:
            st = {"n": len(steps) + 1, "action": "accept", "quest": it[1], "quest_id": it[0]}
            if len(it) > 2:
                st["class"] = it[2]
            steps.append(st)
    out = {"source": "warcrafttavern:" + URL + page + "/", "faction": "Alliance", "race": race, "zone": zone,
           "levels": levels, "startlevel": start, "steps": steps}
    with open(os.path.join(HERE, fname), "w") as f:
        json.dump(out, f, indent=1)
        f.write("\n")
    print(fname, len(steps))
