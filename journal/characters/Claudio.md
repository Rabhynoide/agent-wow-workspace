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

## Analyse (Opus, après l'arrêt)
- La mort finale : après un « fight timeout » (120 s, 0 kill, 100 % PV, 0 rage = jamais au contact), `auto.grind` s'est ARRÊTÉ et le personnage est resté sans job au milieu de 5 loups galeux (corps en -9567,301 ; base : 5 loups à moins de 10 yards). Il est mort sans se défendre.
- Le « fight timeout » : le serveur refusait nos coups ; les codes relevés pendant le test le confirment (Rend 134 UNIT_NOT_INFRONT, 97 OUT_OF_RANGE ; Charge 1 AFFECTING_COMBAT = état « en combat » en retard). Nous n'écoutions pas SMSG_ATTACKSWING_NOTINRANGE / BADFACING et la position de la cible est approximative.
- Equipement en combat refusé (EQUIP_ERR_NOT_IN_COMBAT, code 60).
- Parry acheté : en fait utile (le guerrier peut parer) — pas un défaut.

## Corrections (commit suivant l'analyse)
4. Garde d'`auto` : quand un job d'auto se termine seul, `auto` surveille le personnage et lance `combat.defend` s'il est attaqué (désarmée par `auto.stop` et la déconnexion).
5. `auto.grind` ne s'arrête plus sur un combat ou une marche raté : tour suivant ; arrêt après 5 échecs de suite.
6. Mêlée : écoute de SMSG_ATTACKSWING_NOTINRANGE (rapprochement à 1,5 yard), BADFACING (orientation renvoyée ; et toutes les 2 s), distance 3D ; une cible sans dégâts depuis 25 s est abandonnée (2 min sans la reprendre) au lieu de 120 s.
7. Overpower seulement après une esquive réelle (SMSG_ATTACKERSTATEUPDATE, état 2) ; temps de recharge de Charge conservé entre les combats ; pas de Charge sur une cible qui nous attaque déjà.
8. Pas de changement d'équipement en combat.

## Validation (même soir)
- Chasse `auto.grind` relancée : 15 min sans mort ni arrêt, 2 ventes (124 + 200 pc), réparations, Thunder Clap et Victory Rush appris ; niveau 6 → 3163/3600 XP, 640 pc.
- Reste : Victory Rush tenté hors fenêtre (67, bénin) ; Charge refusée pour ligne de vue (47) alors que notre `move.los` disait « visible » (cible derrière un relief ou un doodad).

## Reprise du 2026-10-11 (niveau 6 → 8)
- Chasse `auto.grind` : 25 min sans mort, niveau 7 (ventes 324 pc, réparations). Puis auto.run [86, 84] : fait (Rockhide Boars niveau 7-8 acceptables à partir du niveau 7), niveau 8, Heroic Strike 2 et Hamstring appris.
- Quête 112 (Collecting Kelp, murlocs du lac) : points d'apparition dans l'eau → `hunt` abandonnait ; essai d'un chemin partiel vers la rive → 2 morts contre les Murloc Streamrunners (5 à moins de 10 yards de la rive). Chemin partiel rendu opt-in. Niveau 8, XP 4157/5400.
- Etat : mort (fantôme) près du lac (-9447,-261), session ouverte, rien ne tourne.
