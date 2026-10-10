# NOTES (lire en premier à chaque session)

## Etat du personnage (fin de session 2026-10-10)
- Claudia, humain prêtre (guid 1), niveau 5, XP 1634/2800, 291 cuivres, 122/122 PV. Position : Northshire Abbey, étage de la tour (~-8900,-181,114), map 0.
- Quêtes de Northshire terminées : 783, 7, 15, 21, 3103 (lettre prêtre), 5261, 33, 18, 6, 3903, 3904, 3905, 5623. Aucune quête en cours.
- Non faites : 54 "Report to Goldshire" (quitte la zone), 2158 "Rest and Relaxation" (aubergiste).
- Sorts non appris depuis le niveau 1 : voir BACKLOG (entraîneur prêtre, talents, équipement non gérés).

## Choix du modèle (à suggérer à l'utilisateur, en une ligne, avant de commencer la tâche)
Je ne peux pas changer de modèle moi-même : je propose `/model <nom>` quand le modèle courant ne colle pas à la tâche, puis je continue avec celui que l'utilisateur garde. Changer via `/model` dans la même session (le contexte est conservé).
- Sonnet : jouer (start.sh, job.sh auto.run, choix de quêtes, suivi), dev simple (RPC calqués sur l'existant, SQL world, scripts, doc, tests), mise à jour NOTES/MODULES/BACKLOG.
- Opus : code protocole (décodage/envoi de paquets, update fields, navmesh, terrain, déplacement), debug d'un comportement incompris après 2 tentatives infructueuses, revue avant push.
- Revue Opus : sur le diff seulement (`git diff origin/claudia-modules...HEAD` ou `/code-review`), par lot de fonctionnalités, en ciblant les points « à vérifier » de BACKLOG.
- En Sonnet : noter dans BACKLOG les doutes de protocole (« à vérifier : … ») et écrire des tests (paquets réels de `logs/state_bad_N.hex`) pour alléger la revue.

## Démarrer / arrêter
- Démarrer : `scripts/start.sh` (vérifie auth + port, lance `char play` en arrière-plan, attend "world session ready", 1 à 3 min ; une seule ligne en sortie). Lancer la commande Bash avec un timeout de 10 min.
- Appeler : `scripts/rpc.sh <module.methode> '<json>' [chemin]` (chemin optionnel, ex. `self` ou `self.hp` ; flottants arrondis). Arrêter : `scripts/rpc.sh session.logout` (refusé en combat : attendre).
- Si le port 8086 est pris : variable AGENT_WOW_WORLDRPC_PORT (aussi pour scripts/rpc.sh).
- Vérifier côté serveur (lecture seule) : `docker exec ac-database mysql -uroot -ppassword -N acore_characters -e "select ... from characters where name='Claudia'"`.

## Flux de jeu typique (économie de tokens : peu d'appels, sorties courtes)
1. `scripts/job.sh auto.run '{"quests":[ids]}'` : lance le job et attend la fin (9 min max, une ligne par changement d'étape). Code 2 = encore en cours : relancer `scripts/job.sh auto.status`. Idéalement en Bash `run_in_background` (notification à la fin). Grouper toutes les quêtes dans un seul `auto.run`. Même usage pour `combat.kill`, `combat.rest`, `auto.recover`, `move.goto`.
2. État courant : `state.brief` (sans la liste d'objets) plutôt que `state.snapshot`.
2. Trouver les quêtes d'un PNJ : `world.quests_for_npc {entry}` ; détail : `world.quest_info {id}`.
3. `auto.recover` si le personnage est mort/fantôme. `combat.rest` pour récupérer PV/mana.

## Environnement
- Serveur AzerothCore en Docker local (ac-worldserver/authserver/database). Realm 127.0.0.1:8085. DB `acore_world` (lecture seule) sur 192.168.100.7:3306. Pas de client mysql sur l'hôte : `docker exec ac-database mysql ...`.
- Données copiées depuis le volume ac-worldserver, dans `assets/` (gitignoré) : `mmaps/` (navmesh, 2.1G), `maps/` (hauteurs terrain, 295M), `dbc/` (Lock.dbc ; Talent.dbc etc. à copier au besoin depuis `/azerothcore/env/dist/data/dbc`). Les recréer : `docker cp ac-worldserver:/azerothcore/env/dist/data/<mmaps|maps> assets/`.
- DB `acore_world` : le module world lit `modules/world/.env` (ignoré par git) pour un compte en lecture seule (WORLD_DB_USER/WORLD_DB_PASSWORD) ; sans ce fichier, repli sur root/password.
- Git : remote `origin` = fork Rabhynoide/agent-wow-workspace, `upstream` = agent-wow/agent-wow-workspace. Branche de travail `claudia-modules`. Token GitHub dans `.env` (GITHUB_TOKEN, ignoré par git) : pousser avec un GIT_ASKPASS temporaire qui lit .env, sans jamais l'écrire dans la config git.

## Pièges rencontrés
- Appels module : timeout 30 s => actions longues en jobs asynchrones (wait_s <= 25).
- Démarrage de 8 modules > 2 min : toujours `--module-timeout 8m`.
- MSG_MOVE_* client->serveur : GUID packé en tête obligatoire, sinon ignoré silencieusement.
- Noms de fichiers mmtile = indices ADT, pas Detour : indexation par en-tête (nav.LoadMap).
- Hauteurs : navmesh seul faux de ~1 yard ; sol réel via `.map` si écart < 0,8 ; hauteurs ponts/étages via polygones du couloir A* uniquement.
- Fantôme = 1 PV (PLAYER_FLAGS & 0x10), pas 0 PV. `state.snapshot.self.ghost/dead`.
- Sac plein (16 emplacements sac à dos) : le butin est refusé en silence. `town.sell_junk` + `auto` gèrent.
- Coffres verrouillés (Lock.dbc type 13 "Open Kneeling") : lancer le sort 22810 sur l'objet (fait dans quest.use).
- Un rpc protobuf ne peut pas avoir le même nom que son message de réponse.
- Distances d'interaction : 3D ! (PNJ à l'étage = hors de portée même au même x/y).
- Reste à vérifier visuellement : saccades sur escaliers, passage sous ponts/barrières (voir BACKLOG).
