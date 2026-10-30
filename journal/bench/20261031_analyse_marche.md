# Analyse de la marche (2026-10-31), 12 runs 6 → 8 Zygor (cd155d2, 0c6554c, 6db3d1d)

Méthode : chaque relevé de 5 s « marche » (`logs/bench_*.samples`) est attribué à l'objectif en cours du
log de niveau (dernière ligne qui n'est pas un incident : combat, vendeur, riposte...). Scripts dans le
scratchpad de la session (walk.py, batches.py), à refaire en `scripts/` si on le répète.

Temps moyen par run : marche 17,1 min (11,9 à 22,8), combat 13,8, marchand 1,9, autre 1,9, approche 1,7.

| Marche par objectif | min/run | runs |
|---|---|---|
| retour de Stormwind (« travel to », 198 s à pied) | 2,7 | 10/12 |
| rendre « Shipment to Stormwind » à Morgan Pestle (Stormwind) | 1,8 | 11/12 |
| début de lot L6 / L7 (vers le premier objectif) | 3,1 | 12/12 |
| rendre « Young Lovers » | 0,9 | 11/12 |
| explorer les entrées des mines (87, 88) | 1,5 | 9-12 |
| chasse Kobold Miner | 0,9 | 9/12 |
| morts : corps, gardien des esprits | 0,8 | 6/12 |

- Shipment to Stormwind (61) : 790 XP pour ~4,5 min d'aller-retour (175 XP/min ; la moyenne 6 → 8 est
  ~225 XP/min). Zygor la rend au niveau 10, en passant à Stormwind. Le bench 6 → 8 pénalise tout report :
  les 790 XP manquent avant le niveau 8 (~3,5 min de jeu), alors que dans la vraie montée elles viennent
  plus tard presque sans marche. C'est sans doute ce qui a fait rejeter hold_turnins (2026-10-28).
- Tentatives ratées (lot commencé puis « too dangerous », fuite, report) : 6,8 min/run sur les 12 runs
  (Fargodeep 60/62 surtout), 3,0 sur la dernière série (décision de fuite par coût).
