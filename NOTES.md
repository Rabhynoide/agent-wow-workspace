# NOTES (lire en premier à chaque session)

## Nouveau personnage (ex. test d'un guerrier)
Les sections « Etat du personnage » et « Pièges » parlent de Claudia (prêtre). Pour un autre personnage :
1. Créer : `agent-wow char create --name <Nom> --race human --class warrior` (races : human, dwarf, gnome, night-elf, draenei ; Alliance seulement pour les routes générées). Noter son état dans `journal/characters/<Nom>.md` (pas dans la section de Claudia).
2. Démarrer : `scripts/start.sh <Nom>` (timeout Bash 10 min). Arrêter : `scripts/rpc.sh session.logout`.
3. Route de la race (guides Zygor convertis dans `assets/routes/`, voir MODULES « zygor2route ») : humain `alliance_01_human_1_13.json`, elfe de la nuit `_02_night_elf_1_13`, gnome `_03_gnome_1_13`, nain `_04_dwarf_1_13`, draeneï `_05_draenei_1_13`, puis pour tous `alliance_07_main_guide_13_20.json alliance_08_levels_20_25.json alliance_09_levels_25_30.json ...`. Si `assets/routes/` est vide : relancer `zygor2route` (MODULES).
4. Leveling : `scripts/level.sh <Nom> <niveau visé>` fait tout (route Zygor dans l'ordre + grind quand il faut un niveau de plus, voir MODULES « level.sh »). A la main : `cd module && go run ./cmd/guidecheck -plan -char <Nom> -race Human -class Warrior -level <niveau> -n 10 ../assets/routes/alliance_01_human_1_13.json ../assets/routes/alliance_07_main_guide_13_20.json` puis `scripts/job.sh auto.run '<json>'`. Refaire le plan à chaque niveau ou quand la liste est finie.
5. Automatique, quelle que soit la classe : talents dès le niveau 10 d'après `journal/classes/<classe>.yaml` (points retirés au respec sautés : pas de respec), entraînement (sans la liste `training.skip`), équipement et revente avec le profil de stats de la classe (guerrier : force, armes à deux mains), réparation, résurrection.
6. Guerrier : rotation de base (Charge, Battle Shout + Bloodrage, Victory Rush, Execute, Overpower, Rend, Thunder Clap avant 20, Heroic Strike avec la rage en trop, Mortal Strike / Slam plus tard). Pas de postures (quêtes de classe 10 et 30 à faire, mais la posture n'est pas utilisée), pas de Premiers soins. Nourriture : achetée et mangée automatiquement (toutes classes ; boisson pour les classes à mana).
8. Survie (toutes classes) : les marches d'`auto` ripostent quand un monstre attaque ; cibles entourées de trop de monstres laissées de côté (capacité `max_pull`) ; résurrection au corps au point le plus sûr puis repos.
7. Classes sans fichier dans `journal/classes/` : pas de talents automatiques ; combat de prêtre (sorts absents) = attaque automatique seulement.

## Etat du personnage : Claudio (guerrier humain, GUID 3)
- Niveau 11 (2026-10-11), ~4 pa, vivant, PV pleins, DURABILITÉ 30 % (réparer au prochain marchand : `auto` le fait), près du camp de Defias au nord-est du lac (-9312,-223), session ouverte. Talents : Improved Rend 2/2. 3 morts pendant les tests du pull (envoyé avec `move.goto` qui ne se défend pas).
- Fait : vallée de Northshire, 54, 2158, 106, 111, 107, 86, 84, 62, 76, 60, 47, 61, 88, 40, 35, 112, 114, 37 (+ précédentes). Reste à refaire : 87 Goldtooth (mine de Fargodeep, sautée pour trop de combats en route ; limite relevée depuis).
- Continuer : `scripts/start.sh Claudio` (si besoin) puis `scripts/level.sh Claudio <niveau>` en arrière-plan. Dès le niveau 11 : malus de résurrection, `auto.recover` ne passe plus par l'esprit guérisseur. Quête de classe du niveau 10 (1638, posture défensive) toujours à faire.

## Etat du personnage : Claudia (fin de session 2026-10-10)
- Humain prêtre (guid 1), niveau 6, XP 1818/3600, 29 cuivres, 171 PV max, équipement réparé à 100 %. Sorts : Châtiment 2, Soins inférieurs 2, Bouclier, Mot de l'ombre : Douleur, Robustesse. Position : Elwynn, sud-ouest de Goldshire (~-9540,67), map 0. Session fermée.
- Quêtes faites : Northshire, 54 « Report to Goldshire », 2158 « Rest and Relaxation », chaîne Maclure (85, 86, 106, 84, 111, 107). Dans le journal : 60 (Kobold Candles) — NE PAS la faire avant 8+ (kobolds en groupes dans la mine).
- Suite : `guidecheck -plan -char Claudia -race Human -class Priest -level 6` (MODULES « zygor2route »). Le plan propose encore 60, 47, 87 (mine de Fargodeep) : les écarter tant que l'évitement des groupes de monstres n'existe pas.
- Prochains sorts au niveau 8 (Priestess Josetta 377 à Goldshire, -9461,33 ; ou le « Priest Trainer » 26328 tout près). Talents automatiques dès le niveau 10 (`journal/classes/priest.yaml`).
- Chasse utile à son niveau (~35 XP/kill) : Loup galeux 525, Sanglier 113, Araignée de forêt 30, Coupe-bourse défias 94, autour de Goldshire : `scripts/grind.sh 7 113 94 30 525`.
- Marchands de Goldshire : Brog Hamfist 151 (-9465,10,57), Farley 295 ; réparateurs : Andrew Krighton 2046 (-9462,88,58), Corina Steele 54.
- NE PAS aller dans la mine de Fargodeep (-9741,114) : kobolds 5-7 en groupes + élite niveau 10 (3 morts le 2026-10-10).

## Choix du modèle (à suggérer à l'utilisateur, en une ligne, avant de commencer la tâche)
Je ne peux pas changer de modèle moi-même : je propose `/model <nom>` quand le modèle courant ne colle pas à la tâche, puis je continue avec celui que l'utilisateur garde. Changer via `/model` dans la même session (le contexte est conservé).
- Sonnet : jouer (start.sh, job.sh auto.run, choix de quêtes, suivi), dev simple (RPC calqués sur l'existant, SQL world, scripts, doc, tests), mise à jour NOTES/MODULES/BACKLOG.
- Opus : code protocole (décodage/envoi de paquets, update fields, navmesh, terrain, déplacement), debug d'un comportement incompris après 2 tentatives infructueuses, revue avant push.
- Revue Opus : sur le diff seulement (`git diff origin/claudia-modules...HEAD` ou `/code-review`), par lot de fonctionnalités, en ciblant les points « à vérifier » de BACKLOG.
- En Sonnet : noter dans BACKLOG les doutes de protocole (« à vérifier : … ») et écrire des tests (paquets réels de `logs/state_bad_N.hex`) pour alléger la revue.

## Builds et guides (voir BACKLOG « A demander à l'utilisateur »)
Icy Veins bloque ma lecture (403). Aux paliers de niveau listés dans BACKLOG (9, 19, 39, 49, 59, changement de zone), prévenir l'utilisateur en une ligne pour qu'il colle le guide complet, avant de modifier `journal/classes/<classe>.yaml`, les profils de `gear` ou la rotation de `combat`.

## Guides de leveling
- Données de classe (talents, rotation, sorts) : `journal/classes/<classe>.yaml` (prêtre, guerrier), à valider avec `cmd/classcheck`.
- Routes de quêtes : guides Zygor convertis dans `assets/routes/` (MODULES « zygor2route »). `guidecheck -plan` donne les prochaines quêtes du personnage, `scripts/job.sh auto.run '<json>'` les fait. Les guides ont 2-3 ans (WotLK Classic ≠ 3.3.5) : la base du serveur fait foi.
- Extraction Claude Desktop (en pause) : `journal/guides/` (format : README du dossier).

## Démarrer / arrêter
- Démarrer : `scripts/start.sh` (vérifie auth + port, lance `char play` en arrière-plan, attend "world session ready", 1 à 3 min ; une seule ligne en sortie). Lancer la commande Bash avec un timeout de 10 min.
- Appeler : `scripts/rpc.sh <module.methode> '<json>' [chemin]` (chemin optionnel, ex. `self` ou `self.hp` ; flottants arrondis). Arrêter : `scripts/rpc.sh session.logout` (refusé en combat : attendre).
- Si le port 8086 est pris : variable AGENT_WOW_WORLDRPC_PORT (aussi pour scripts/rpc.sh).
- Vérifier côté serveur (lecture seule) : `docker exec ac-database mysql -uroot -ppassword -N acore_characters -e "select ... from characters where name='Claudia'"`.

## Flux de jeu typique (économie de tokens : peu d'appels, sorties courtes)
1. Démarrer (`scripts/start.sh <Nom>`), puis le plan de quêtes (`guidecheck -plan ...`, voir « Guides de leveling »).
2. Le plus simple : `scripts/level.sh <Nom> <niveau>` en arrière-plan (tout le leveling ; voir MODULES « level.sh »). Sinon `scripts/job.sh auto.run '{"quests":[ids]}'` : lance le job et attend la fin (9 min max, une ligne par changement d'étape). Code 2 = encore en cours : relancer `scripts/job.sh auto.status`. Idéalement en Bash `run_in_background`. Grouper les quêtes dans un seul `auto.run`. Même usage pour `combat.kill`, `combat.rest`, `auto.recover`, `auto.goto`. Pour se déplacer hors combat : `auto.goto` (riposte si attaqué), PAS `move.goto` (ne se défend pas : 3 morts le 2026-10-11). `auto` gère seul l'équipement, les talents, l'entraîneur proche, la revente, la réparation et la résurrection.
3. Entre deux plans, ou si les quêtes manquent : `scripts/grind.sh <niveau> <entrées>`.
4. État courant : `state.brief` (sans la liste d'objets) plutôt que `state.snapshot`. Quêtes d'un PNJ : `world.quests_for_npc {entry}` ; détail : `world.quest_info {id}`.
5. Mort : `auto.recover`. Repos : `combat.rest`. Arrêter : `scripts/rpc.sh session.logout`.

## Environnement
- Serveur AzerothCore en Docker local (ac-worldserver/authserver/database). Realm 127.0.0.1:8085. DB `acore_world` (lecture seule) sur 192.168.100.7:3306. Pas de client mysql sur l'hôte : `docker exec ac-database mysql ...`.
- Données copiées depuis le volume ac-worldserver, dans `assets/` (gitignoré) : `mmaps/` (navmesh, 2.1G), `maps/` (hauteurs terrain, 295M), `dbc/` (Lock, Spell, Talent, TalentTab, SkillLineAbility, FactionTemplate ; requis par `progress` et `state` (réactions), et `guidecheck -play` ; autres DBC dans `/azerothcore/env/dist/data/dbc`), `vmaps/` (657M, géométrie de collision : hauteurs dans les bâtiments et ligne de vue, requis par `move`). Les recréer : `docker cp ac-worldserver:/azerothcore/env/dist/data/<mmaps|maps|vmaps> assets/`.
- DB `acore_world` : le module world lit `modules/world/.env` (ignoré par git) pour un compte en lecture seule (WORLD_DB_USER/WORLD_DB_PASSWORD) ; sans ce fichier, repli sur root/password.
- Sources de rotation (Ovale, WeakAuras décodés, IA de classe de mod-playerbots) : `assets/sources/` (local) ; résumés dans `journal/rotations/` (`scripts/rotation_sources.py`). Revue de mod-playerbots : `journal/playerbots_review.md`.
- Git : remote `origin` = fork Rabhynoide/agent-wow-workspace, `upstream` = agent-wow/agent-wow-workspace. Branche de travail `claudia-modules`. Token GitHub dans `.env` (GITHUB_TOKEN, ignoré par git) : pousser avec un GIT_ASKPASS temporaire qui lit .env, sans jamais l'écrire dans la config git.

## Pièges rencontrés
- (corrigé le 2026-10-11) Après une mort, le serveur téléporte le fantôme au cimetière et attend MSG_MOVE_TELEPORT_ACK ; sans réponse, il ignorait tous nos mouvements : le corps se réclamait (le serveur nous croyait encore là) mais ensuite le personnage « marchait » seul de notre côté, ne voyait plus aucun PNJ (« not visible within 40 yards »), et à la reconnexion revenait au cimetière. `state` répond maintenant. Symptôme si ça revient : `state.nearby` vide dans une zone peuplée, ou vitesse 10,5 (fantôme) après résurrection : grep « teleported » dans logs/state.log.
- Après une résurrection le personnage a 50 % de PV : ne JAMAIS rester immobile près d'un camp ; `auto.recover` ressuscite chez l'esprit guérisseur du cimetière (CMSG_SPIRIT_HEALER_ACTIVATE, flag PNJ 0x4000) s'il est à moins de 150 yards, UNIQUEMENT aux niveaux 1-10 ; à partir du niveau 11 (`Death.SicknessLevel = 11` du serveur) l'esprit guérisseur applique le malus de résurrection (stats très réduites pendant niveau-10 minutes, 10 min dès le niveau 20) : il court alors au corps (la récupération du corps n'applique jamais le malus). Le guérisseur coûte aussi 25 % de durabilité. Le corps est souvent dans le groupe qui nous a tués.
- Un `move.goto` s'arrête maintenant si le personnage meurt (avant, le corps « marchait » dans le modèle de position).
- Le délai de récupération du corps grandit avec les morts rapprochées (jusqu'à ~2 min) : `recover` insiste 4 minutes.
- `auto` refuse les monstres élites ou au-dessus de la marge de danger (« too dangerous for now »). `combat.kill` évite maintenant les cibles entourées de trop de monstres (capacité `max_pull`), mais les groupes LIÉS (camps de Defias, formations) et l'aggro en marchant restent un risque.
- `move.goto` ne se défend pas : un monstre qui attaque en route tue le personnage. Utiliser `auto.goto` (ou `auto.run`/`auto.grind`, qui ripostent).
- Un script shell qui boucle sur `combat.kill` peut tourner à vide quand le sac est plein (« bags full » instantané) : lancer la chasse par `auto.grind` (via `grind.sh`), jamais par `combat.kill` en boucle. Pour tuer un script : `pkill -f "grind[.]sh"` (les crochets évitent de se tuer soi-même).
- `kill -f` / `pkill -f scripts/...` dans une commande Bash qui contient ce motif tue aussi la commande elle-même (code 144) : utiliser `rpc.sh combat.stop` / `auto.stop` / `move.stop`.
- Appels module : timeout 30 s => actions longues en jobs asynchrones (wait_s <= 25).
- Démarrage de 9 modules (images Docker) > 2 min : toujours `--module-timeout 8m` (`scripts/start.sh` le fait).
- MSG_MOVE_* client->serveur : GUID packé en tête obligatoire, sinon ignoré silencieusement.
- Noms de fichiers mmtile = indices ADT, pas Detour : indexation par en-tête (nav.LoadMap).
- Hauteurs : navmesh seul faux de ~1 yard. Désormais chemin lissé sur la surface du navmesh puis sol choisi comme le serveur (terrain `.map` ou vmaps). Ne pas revenir à l'interpolation en ligne droite.
- Fantôme = 1 PV (PLAYER_FLAGS & 0x10), pas 0 PV. `state.snapshot.self.ghost/dead`.
- Sac plein (16 emplacements sac à dos) : le butin est refusé en silence. `auto` vend (règles `sellrule`) et répare chez le marchand le plus proche.
- Les PNJ « [DND] ... » sont techniques (61 autour de Goldshire avec les flags marchand/réparateur) : `world.find_npc` les ignore.
- Résultat de sort 40 = interrompu, 47 = ligne de vue ; les sorts ignorent les doodads M2 pour la ligne de vue (comme `move.los`).
- Coffres verrouillés (Lock.dbc type 13 "Open Kneeling") : lancer le sort 22810 sur l'objet (fait dans quest.use).
- Un rpc protobuf ne peut pas avoir le même nom que son message de réponse.
- Distances d'interaction : 3D ! (PNJ à l'étage = hors de portée même au même x/y).
- Reste à vérifier visuellement : saccades sur escaliers, passage sous ponts/barrières (voir BACKLOG).
