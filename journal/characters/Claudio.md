# Claudio — guerrier humain (GUID 3) — test de leveling, 2026-10-10

Objectif de l'utilisateur : leveling jusqu'au niveau 10 avec un personnage créé de zéro (modèle Sonnet).
Arrêté par l'utilisateur au niveau 6 (le test continue à la prochaine session).

## Etat à l'arrêt
- Niveau 6, XP 852/3600, 197 cuivres, 155 PV max, équipement réparé à 100 %.
- MORT (fantôme au cimetière d'Elwynn, ~-9559,218) : `auto.recover` le ressuscite chez l'esprit guérisseur (niveau < 11).
- Session fermée. Démarrer : `scripts/start.sh Claudio`.

## Chronologie
- Niveau 1 → 3 : 6 quêtes de Northshire (783, 7, 5261, 33, 15, 3100 = lettre de classe) en 9 min, sans mort. Battle Shout appris automatiquement.
- Niveau 3 → 5 : 18, 3903, 6, 21, 3904, 3905 en 17 min ; vente automatique (12 piles / 100 pc), Charge et Rend appris.
- 54 + 2158 acceptées ensemble puis rendues à Goldshire (72 s). Vallée de Northshire complète : 12 quêtes sur 12 disponibles (7961, 7962 et 14032 n'ont aucun donneur : événements/doublons).
- Chaîne Maclure : 85 faite ; 86 (viande de sanglier) → 2 morts sur des Rockhide Boars niveau 7-8 ; filtre ramené à niveau + 1 ; 106, 111, 107 faites.
- Chasse : niveau 6 en ~4 min (Sanglier 113, Araignée 30, Loup 525). `grind.sh` bouclait à vide avec les sacs pleins → remplacé par `auto.grind` : vente 12 piles / 204 pc, réparation 47 pc, gants équipés, mais « fight timeout » avec 0 kill au bout de 430 s, puis personnage trouvé mort à l'arrêt.

## Défauts trouvés
Corrigés (commit 4b36bc8) :
1. Rockhide Boars niveau 7-8 chassés par un guerrier de niveau 5 : danger par classe (casters +2, autres +1), repos à 75 % de PV.
2. Rotation : Battle Shout sans rage (code 85), Charge trop près (128), Heroic Strike relancé (40).
3. `grind.sh` boucle à vide avec « bags full » : remplacé par `auto.grind`.
Ouverts (voir BACKLOG « Test du guerrier ») :
4. « fight timeout » : un combat de 120 s sans kill (cible non atteinte ou non frappée ?), puis mort non expliquée.
5. L'entraîneur achète des sorts inutiles (Parry passive, 95 pc).
6. Charge « pas prête » (code 67) répété ; Overpower tenté à l'aveugle.
7. Loups (525) et coupe-bourses (94) hors de la portée visible autour de Goldshire : les rounds correspondants échouent vite (sauté par `auto.grind`).
