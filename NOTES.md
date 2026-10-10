# NOTES (lire en premier à chaque session)

## Etat du personnage (fin de session 2026-10-10, soir)
- Claudia, humain prêtre (guid 1), niveau 5, XP 2580/2800 (il manque 220 pour le niveau 6), 237 cuivres, 152 PV max. Position : Goldshire, devant le marchand Brog Hamfist (~-9465,12,58), map 0. Session fermée.
- Sacs : 12 emplacements libres (revente automatique faite le 2026-10-10 ; `auto` vend tout seul à moins de 3 emplacements libres). Marchands de Goldshire : Brog Hamfist (151) à (-9465,10,57), aussi Farley 295, Dobbins 465, Toddrick 3935.
- Quêtes faites : celles de Northshire + 54 « Report to Goldshire » + 2158 « Rest and Relaxation ». Aucune quête en cours. Les autres quêtes d'Elwynn proposées à son niveau sont soit dangereuses (60, 47 : kobolds 5-8 dans la mine de Fargodeep, 3 morts), soit non gérées (5624 soin de Guard Roberts ; 62 exploration).
- Entraîneur de classe de Goldshire : Priestess Josetta (377) à (-9461,33). Au niveau 6 : Bouclier (100 pc) et Châtiment 2 (100 pc) : il faut ~200 pc.
- Talents : première dépense au niveau 10 (build Ombre dans `priest.Build`).
- Chasse utile à son niveau (35 XP/kill) : Loup galeux 525, Sanglier défenseur 113, Araignée de forêt 30, Coupe-bourse défias 94 (tous niveau 5-6, autour de Goldshire). `scripts/grind.sh 6 113 94 30 525` enchaîne les chasses jusqu'au niveau voulu.
- NE PAS aller dans la mine de Fargodeep (-9741,114) ni près du cimetière de la mine : kobolds 5-7 en groupes + élite niveau 10.

## Choix du modèle (à suggérer à l'utilisateur, en une ligne, avant de commencer la tâche)
Je ne peux pas changer de modèle moi-même : je propose `/model <nom>` quand le modèle courant ne colle pas à la tâche, puis je continue avec celui que l'utilisateur garde. Changer via `/model` dans la même session (le contexte est conservé).
- Sonnet : jouer (start.sh, job.sh auto.run, choix de quêtes, suivi), dev simple (RPC calqués sur l'existant, SQL world, scripts, doc, tests), mise à jour NOTES/MODULES/BACKLOG.
- Opus : code protocole (décodage/envoi de paquets, update fields, navmesh, terrain, déplacement), debug d'un comportement incompris après 2 tentatives infructueuses, revue avant push.
- Revue Opus : sur le diff seulement (`git diff origin/claudia-modules...HEAD` ou `/code-review`), par lot de fonctionnalités, en ciblant les points « à vérifier » de BACKLOG.
- En Sonnet : noter dans BACKLOG les doutes de protocole (« à vérifier : … ») et écrire des tests (paquets réels de `logs/state_bad_N.hex`) pour alléger la revue.

## Builds et guides (voir BACKLOG « A demander à l'utilisateur »)
Icy Veins bloque ma lecture (403). Aux paliers de niveau listés dans BACKLOG (9, 19, 39, 49, 59, changement de zone), prévenir l'utilisateur en une ligne pour qu'il colle le guide complet, avant de modifier `priest.Build`, les poids de `gear` ou la rotation de `combat`.

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
- Données copiées depuis le volume ac-worldserver, dans `assets/` (gitignoré) : `mmaps/` (navmesh, 2.1G), `maps/` (hauteurs terrain, 295M), `dbc/` (Lock, Spell, Talent, TalentTab, SkillLineAbility ; requis par `progress` ; autres DBC dans `/azerothcore/env/dist/data/dbc`). Les recréer : `docker cp ac-worldserver:/azerothcore/env/dist/data/<mmaps|maps> assets/`.
- DB `acore_world` : le module world lit `modules/world/.env` (ignoré par git) pour un compte en lecture seule (WORLD_DB_USER/WORLD_DB_PASSWORD) ; sans ce fichier, repli sur root/password.
- Git : remote `origin` = fork Rabhynoide/agent-wow-workspace, `upstream` = agent-wow/agent-wow-workspace. Branche de travail `claudia-modules`. Token GitHub dans `.env` (GITHUB_TOKEN, ignoré par git) : pousser avec un GIT_ASKPASS temporaire qui lit .env, sans jamais l'écrire dans la config git.

## Pièges rencontrés
- Après une résurrection le personnage a 50 % de PV : ne JAMAIS rester immobile près d'un camp ; `auto.recover` ressuscite chez l'esprit guérisseur du cimetière (CMSG_SPIRIT_HEALER_ACTIVATE, flag PNJ 0x4000) s'il est à moins de 150 yards, UNIQUEMENT aux niveaux 1-10 ; à partir du niveau 11 (`Death.SicknessLevel = 11` du serveur) l'esprit guérisseur applique le malus de résurrection (stats très réduites pendant niveau-10 minutes, 10 min dès le niveau 20) : il court alors au corps (la récupération du corps n'applique jamais le malus). Le guérisseur coûte aussi 25 % de durabilité. Le corps est souvent dans le groupe qui nous a tués.
- Un `move.goto` s'arrête maintenant si le personnage meurt (avant, le corps « marchait » dans le modèle de position).
- Le délai de récupération du corps grandit avec les morts rapprochées (jusqu'à ~2 min) : `recover` insiste 4 minutes.
- `auto.run` choisit la source d'objet la plus proche sans regarder le niveau : peut envoyer dans un camp trop fort (voir BACKLOG).
- `kill -f` / `pkill -f scripts/...` dans une commande Bash qui contient ce motif tue aussi la commande elle-même (code 144) : utiliser `rpc.sh combat.stop` / `auto.stop` / `move.stop`.
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
