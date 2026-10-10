# Claudio — guerrier humain (GUID 3) — test de leveling, 2026-10-10

Objectif de l'utilisateur : leveling jusqu'au niveau 10 avec un personnage créé de zéro (modèle Sonnet).

## Chronologie
- Niveau 1 → 3 : 6 quêtes de Northshire (783, 7, 5261, 33, 15, 3100 = lettre de classe) en 9 min, sans mort. Battle Shout appris automatiquement.
- Niveau 3 → 5 : 18, 3903, 6, 21, 3904, 3905 en 17 min ; vente automatique (12 piles / 100 pc), Charge et Rend appris.
- 54 + 2158 acceptées ensemble puis rendues à Goldshire (72 s).
- Chaîne Maclure : 85 faite ; 86 (viande de sanglier) → 2 morts sur des Rockhide Boars niveau 7-8 (le filtre de danger laissait niveau + 2) ; filtre ramené à niveau + 1 pour le guerrier ; 106, 111, 107 faites.
- Chasse : niveau 6 en ~4 min (Sanglier 113, Araignée 30 ; loups 525 et coupe-bourses 94 hors de portée).

## Défauts trouvés (corrigés)
1. Rockhide Boars niveau 7-8 chassés par un guerrier de niveau 5 : règle de danger par classe (casters +2, autres +1), repos à 75 % de PV pour le guerrier.
2. Rotation : Battle Shout sans rage (code 85), Charge trop près (128), Heroic Strike relancé (40).
3. `grind.sh` bouclait à vide avec « bags full » : remplacé par `auto.grind`.

## Suite à observer
- Overpower tenté à l'aveugle ; pas de soin, ni nourriture, ni Premiers soins.
- Talents automatiques au niveau 10 (premier test réel de `journal/classes/warrior.yaml`).
