# Vérifications en jeu des modules (retirées de MODULES.md le 2026-10-12)

- `move.safe_point {x, y, z, max_dist(30), min_dist, path_clear}` : po… → Testé.
- Auras (2026-10-11) : `state` décode SMSG_AURA_UPDATE / SMSG_AURA_UPD… → Testé en jeu (Rend 6546 sur la cible avec 13 s, Battle Shout 6673, Posture de combat).
- `quest.explore {trigger}` : envoie CMSG_AREATRIGGER (il faut être da… → Testé : quêtes 62 et 76.
- `quest.use_item {item_entry, spell, target_entry, target_kind}` : ut… → Testé : pierre de foyer (6948) ; sur une cible : pas encore (quêtes de niveau 14+).
- « Peut lancer » (2026-10-11, `internal/combat/castcheck.go`) : avant… → Test : 5 loups → 1 sort refusé (8 avant).
- Décisions tirées des auras (plus de minuteries devinées) : guerrier … → Testé en jeu pour le guerrier (plus d'échec de Victory Rush hors fenêtre) ; prêtre non testé en jeu.
- Repos : mange le meilleur aliment des sacs (catégorie de sort 11, ni… → Testé en jeu.
- `town.restock {food, drink, max_copper}` : chez le marchand à <= 5 y… → Testé.
- Nourriture : à chaque passage chez un marchand, `auto` complète jusq… → Testé.
- Marches : `auto` vérifie toutes les 2 s si un monstre en combat nous… → Testé (niveau 10 → 11 sans mort).
- `auto.goto {x, y, z, radius(3), wait_s}` : marche qui riposte quand … → Testé.
- Chemin lissé (`nav.SmoothPath`, port du `findSmoothPath` du serveur … → Testé : 6 trajets réels d'Elwynn, 100 % des points sur la surface, < 20 ms par km.
