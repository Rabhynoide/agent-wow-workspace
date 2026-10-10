# MODULES

Sources Go partagées : `module/` (un seul go.mod, un binaire par module dans `module/cmd/<nom>`).
Manifestes + descripteurs : `modules/<nom>/` (module.yaml, compose.yaml, module.pb).
Régénérer protos : `cd module && make generate` (met à jour `modules/*/module.pb`). Les images se reconstruisent au `char play`.
Scripts : `start.sh` (démarre la session), `job.sh` (attend un job async), `grind.sh <niveau> <entrée>...` (enchaîne les chasses, repos, résurrection, arrêt après 3 morts / 25 min).
Appel : `scripts/rpc.sh <module.methode> '<json>' [chemin]` (sortie JSON compacte, flottants arrondis). Jobs async : `scripts/job.sh <module.methode> '<json>'` (attend la fin). Logs : `logs/<module>.log`.
Test navmesh hors-ligne : `cd module && go test ./internal/nav -v`.

## zygor2route + guidecheck -plan  (leveling guidé par les guides Zygor)
- Source : dépôt GitHub ErebusAres/ZygorGuidesRemaster-3.3.5a_WOTLK cloné dans `assets/zygor/repo/` (contenu tiers sous droits : JAMAIS dans git ; `assets/` est ignoré). On garde seulement notre code.
- `cd module && go run ./cmd/zygor2route -src ../assets/zygor/repo/ZygorGuidesViewerRM/Guides/Leveling/ZygorGuidesAlliance.lua -out ../assets/routes` (idem `ZygorGuidesHorde.lua`, avec `-prefix horde`) : 25 routes JSON (Alliance : 5 races de départ 1-13, puis Main Guide 13-20, Levels 20-25 ... Northrend 78-80) avec ids de quêtes/PNJ/monstres, variantes `only` (race+classe).
- `go run ./cmd/guidecheck -max 6 <fichiers de la chaîne dans l'ordre>` valide contre la base (Human 1-13 + 13-20 + 20-25 + 25-30 : 1090 étapes, 296 quêtes, 8 écarts bénins).
- `go run ./cmd/guidecheck -plan -char Claudia -race Human -class Priest -level 6 -n 12 [-maxabove 2] <fichiers>` : prochaines quêtes de la route non faites (lit `character_queststatus_rewarded`), sans les quêtes manuelles (exploration, sort à lancer, groupe, élite/donjon/raid/escorte/PvP via QuestInfoID), autorisées à la race et à la classe, niveau minimum atteint, niveau de quête <= niveau + maxabove, prérequis rendus ou placés avant dans la liste ; + JSON pour `auto.run` (qui s'arrête au premier refus). Chaîne humaine : `assets/routes/alliance_01_human_1_13.json alliance_07_main_guide_13_20.json alliance_08_levels_20_25.json ...`.

## classcheck  (outil hors jeu, `module/cmd/classcheck`)
- Fichiers de classe fournis par l'utilisateur dans `journal/classes/<classe>.yaml` (format : `journal/classes/README.md`, extraits d'Icy Veins : spec, ordre des talents avec respec, sorts clés, rotation, quêtes de classe, entraînement).
- `cd module && go run ./cmd/classcheck ../journal/classes/warrior.yaml` : rejoue l'ordre des talents avec les règles du serveur, vérifie arbres et sorts (Talent/TalentTab/Spell/SkillLineAbility.dbc) et quêtes de classe (base), complète les niveaux d'apprentissage (trainer_spell). Code 1 s'il y a des problèmes.

## guidecheck  (outil hors jeu, `module/cmd/guidecheck`)
- `cd module && go run ./cmd/guidecheck -dir ../journal/guides` : valide les guides extraits (format dans `journal/guides/README.md`) contre la base : titre/id de quête, race, classe, niveau minimum, PNJ donneur/rendeur, prérequis, doublons de titre (départagés par PNJ puis classe). Liste les quêtes à faire à la main (exploration, sort à lancer sur une cible, groupe). Code de sortie 1 s'il y a des problèmes.
- `... -ids -from 5 -to 10` : imprime `{"quests":[...]}` (ordre du guide, quêtes réalisables par `auto`) à donner à `auto.run`. Nécessite la base accessible depuis l'hôte (WORLD_DB_HOST, défaut 192.168.100.7:3306).

## state  (paquets -> état du monde)
- `state.brief {}` : self (dont class_id, race, talent_points) + quêtes + free_slots + durability_pct (pire objet équipé, 100 si rien ne s'use), sans les objets (à préférer).
- `state.snapshot {}` : self (guid,map,x,y,z,o,level,hp,power,xp,money,target,dead,run_speed), items (slot -1 = sac secondaire ou banque), quêtes (id,state,counts[4] uint16), free_slots (sac à dos + sacs équipés), nb objets.
- `state.nearby {kind:"unit|gameobject|player|any", entry, radius, max(5), alive_only, npc_flags}` : trié par distance (guid,entry,pos,dist,level,hp,faction,flags,npc_flags,dyn_flags,target).
- `state.spells {}` : sorts connus. self.ghost/dead (fantôme détecté). `state.get_obj {guid}`, `state.set_pos {x,y,z,o,map}` (utilisé par move).
- Limites : factions/hostilité non résolues ; argent = 0 au snapshot initial à confirmer ; pas de chat/combat log. Positions des mobs = destination de la spline (approximatif ; spline cyclique = position de départ).
- Debug : un paquet d'update non parsable est dumpé dans `logs/state_bad_N.hex` (5 max).

## world  (MySQL acore_world, lecture seule, session `transaction_read_only`)
- `world.find_npc {name|entry, map_filter+map, x,y,radius, npc_flags, max}` : spawns + distance (sans les PNJ techniques « [DND] ... » : 61 autour de Goldshire portent les flags marchand/réparateur). `world.spawns` renvoie aussi `rank` (1 élite, 2 rare élite, 3 boss, 4 rare). npc_flags : vendeur 128, réparateur 4096, aubergiste 65536, questgiver 2, gossip 1.
- `world.quest_info {id|title}` : niveau, objectifs (kill/item/gameobject + noms, `counter` = index du compteur du journal), starters/enders avec coords, quête suivante/précédente.
- `world.quests_for_npc {entry}`, `world.spawns {entry,map,x,y,radius,kind:creature|gameobject}`, `world.item_info {entry|name}`, `world.creature {entry}`.
- `world.class_trainers {class_id, map, x, y, max(3)}` : entraîneurs de classe les plus proches avec leurs sorts (coût, niveau, prérequis).
- `world.item_info` renvoie aussi armor, dps, stats (type ItemMod, valeur), allowable_class/race ; `world.quest_info` renvoie reward_choices.
- `world.item_sources {entry}` : créatures/objets qui lâchent un objet (avec chance).
- Hôte DB : 192.168.100.7:3306 (env dans modules/world/compose.yaml).

## quest  (requires: state)
- `quest.accept {npc_entry, quest_id}`, `quest.turnin {npc_entry, quest_id, reward}` : PNJ visible à <= 5 yards (3D), confirme via le journal de quêtes.
- `quest.use {entry, loot}` : utilise un objet (coffre), repli sort 22810 si verrouillé, ramasse tout le butin.

## combat  (requires: state, move) — jobs asynchrones
- `combat.kill {entry, count, item_entry, item_count, radius(80), max_s(600), no_loot, wait_s}` : chasse l'entrée. GUERRIER (`internal/warrior`, rangs vérifiés contre Spell.dbc) : Charge (8-24 yards, hors combat), attaque auto, Victory Rush après un kill, Execute < 20 %, Overpower (essayé toutes les 2 s), Rend si cible > 50 %, Mortal Strike, Slam (41+), Thunder Clap avant 20, Heroic Strike au-delà de 30 rage (60 dès 41) ; Battle Shout + Bloodrage entre les combats. PRÊTRE : priorités : soin si PV < 40 % (Soins, sinon Soins inférieurs), Bouclier si PV < 80 % (15 s d'Âme affaiblie), Rénovation si PV < 65 %, Mot de l'ombre : Douleur (toutes les 18 s), Attaque mentale (8 s), Châtiment ; sans mana : baguette (Tir) si équipée, sinon mêlée. Robustesse/Feu intérieur hors combat toutes les 25 min. Rangs : `module/internal/priest` (vérifiés contre Spell.dbc). Se défend contre les agresseurs (unités en combat qui nous ciblent), un mob perdu de vue ne compte pas comme tué, repos auto (PV<55 % ou mana<25 %), butin des cadavres. Échoue avec "bags full" si sac plein.
- `combat.rest {}`, `combat.status {wait_s}`, `combat.stop {}`. Statut : state running|done|failed|stopped, kills, items, hp_pct, mana_pct, doing.
- Limites : sorts de prêtre seulement, pas de fuite, pas de ciblage par hostilité, pas de suivi des auras (minuteries).

## town  (requires: state, world, move)
- `town.sell_junk {dry_run, min_free, show_kept}` (garde aussi les réactifs, classe 5) : vend ce que les règles `module/internal/sellrule` désignent, au marchand à <= 5 yards (limite du serveur : 5,5 ; il faut déjà y être, `auto` s'en charge). `dry_run` + `show_kept` donnent le détail (objet, quantité, prix, raison) sans rien vendre. `min_free` : s'arrêter quand ce nombre d'emplacements est libre. Vérifie ensuite ce que le serveur a vraiment pris.
- Règles : on VEND les objets gris ; l'armure/arme que la classe ne peut pas porter (compétence non apprise) ou qui n'améliore pas l'équipement (`gear.Gain`) ; les marchandises/recettes/gemmes/munitions ; les objets divers « junk » ; les nourritures et boissons en trop (on garde la meilleure entrée de chaque sorte, catégories 11 et 59) ; les consommables dont le niveau est inférieur d'au moins 10 au nôtre. On GARDE : objets de quête (journal + classe 12), objets impossibles à vendre (pierre de foyer), bleus et mieux, sacs (pas équipés automatiquement), clés, améliorations non équipées (ou à moins de 5 niveaux), autres consommables.
- `town.repair {}` : répare tout (CMSG_REPAIR_ITEM, objet 0) chez un réparateur (flag 4096) à <= 5 yards ; répare ce que l'argent permet. `auto` passe chez un marchand quand les sacs sont presque pleins OU la durabilité < 60 %, vend, puis répare si le PNJ sait réparer.
- Les objets vendus restent dans la liste de rachat du marchand : `state` ne les liste plus (sinon les comptes d'objets de quête étaient faussés).

## auto  (requires: state, world, move, quest, combat, town, progress) — orchestrateur
- `auto.run {quests:[ids], rewards:{id:index}, no_progress, wait_s}` : au départ et après chaque quête (sauf `no_progress`) : `progress.gear`, `progress.talents`, et entraînement si l'entraîneur a des sorts à apprendre à moins de 1500 yards ; récompense choisie par `progress.pick_reward` si absente de `rewards` ; événements dans `progress` du statut. Puis pour chaque quête : va au donneur, accepte, objectifs (kill / objets via la source la plus proche sur la carte / objets de décor), rend. Suit les PNJ qui se déplacent. Reprend après une mort. Gère sac plein.
- `auto.status {wait_s}`, `auto.stop {}`, `auto.recover {wait_s}` (libérer l'âme, puis, aux niveaux 1-10 seulement, ressusciter chez l'esprit guérisseur du cimetière s'il est à moins de 150 yards (malus de résurrection dès le niveau 11 : jamais utilisé ensuite) ; sinon courir au corps et le réclamer en insistant 4 minutes).
- Danger : `hunt`/`collect` refusent les monstres élites ou de niveau > niveau + 2 (« too dangerous for now »), mais ne voient pas les groupes (ex. kobolds de la mine) : voir BACKLOG. Entraîneur seulement à moins de 400 yards. Talents par lots de 10.
- Limites : pas de quête à groupe/élite, une carte à la fois.

## progress  (requires: state, world, move) — rendre le personnage plus fort
- `progress.train_check {}` : sorts que l'entraîneur de classe le plus proche apprendrait maintenant (count, cost, spells, trainer, dist, unaffordable). Source : tables `trainer`/`trainer_spell` + Spell.dbc (sorts appris, race/classe).
- `progress.train {wait_s}` (job async, `progress.status`/`progress.stop`) : va à l'entraîneur, ouvre la liste (CMSG_TRAINER_LIST), achète tout ce qui est disponible et abordable, du moins cher au plus cher. Mémorise les sorts refusés pour le niveau courant (pas de voyage inutile).
- `progress.talents {dry_run}` : dépense les points selon `journal/classes/<classe>.yaml` (monté en `/classes`, lu par `internal/classdata`) : ordre `talent_order`, points marqués `removed_at` sautés (jamais de respec), 10 points par appel. Fichiers validés par `cmd/classcheck` et par le test de `classdata`.
- `progress.gear {dry_run}` : équipe les améliorations du sac, score selon le profil de la classe (`gear.ProfileFor` : lanceur pour prêtre/mage/démoniste = puissance des sorts > Esprit > hâte > critique > Int > Endu, baguette ; force pour guerrier/paladin/DK = DPS d'arme (deux mains surtout) > Force > critique > toucher > Agilité > Endu ; agilité pour voleur/chasseur/druide/chaman). Utilisable = niveau, masques classe/race, sort de compétence connu. Bagues/bijoux sur l'emplacement le plus faible (CMSG_SWAP_ITEM), sinon CMSG_AUTOEQUIP_ITEM. La revente utilise le même profil.
- `progress.pick_reward {items}` : index de la récompense de quête qui améliore le plus l'équipement, sinon la plus chère à la revente.
- Données client : `assets/dbc` (Spell, Talent, TalentTab, SkillLineAbility) monté en `/dbc`.
- Limites : build de talents et poids de stats pour prêtre seulement ; pas de maître d'armes ; pas de comparaison de sacs.

## move  (requires: state)
- `move.goto {x,y,z, radius(1.5), wait_s(20, max 25), walk}` : planifie sur navmesh (ground only) ; si une marche est en cours (même allure), remplace l'itinéraire sans stop/start (poursuite fluide) ; sinon lance un job de marche (start/heartbeat/facing/stop, paquets avec GUID packé), bloque jusqu'à wait_s. Retourne state moving|arrived|failed|stopped, reason, pos, dist_left, eta_s.
- `move.los {x,y,z}` : ligne de vue (vmaps, sans les doodads M2) des yeux du personnage vers ceux de la cible (+2 yards chacun) ; `visible` absent = bloqué.
- `move.status {wait_s}` : attend la fin du job (max 25 s par appel — timeout RPC 30 s). `move.stop {}`. `move.path {x,y,z}` : dry-run (points, longueur).
- Pathfinding : `module/internal/nav` (tuiles Detour lues depuis `assets/mmaps`, A* + funnel, ~50 ms pour 2 km ; cache LRU de 128 tuiles).
- `move.goto` échoue avec « character died » si le personnage meurt en marchant.
- Jobs async (move, combat, auto) : `module/internal/jobs` (un job à la fois, start/stop sérialisés, stop interruptible).
- Chemin lissé (`nav.SmoothPath`, port du `findSmoothPath` du serveur / Detour) : un point tous les 2 yards, chaque pas glisse le long des polygones marchables (`moveAlongSurface`, pas de coupe de coin au-dessus du vide ou à travers un mur), corridor recalé à chaque pas, hauteur du polygone atteint (escaliers et étages compris). Puis hauteur du sol comme le serveur (`Map::GetHeight`) : recherche 2 yards au-dessus du point, terrain (`assets/maps`) si on n'est pas dessous, sol des vmaps (`internal/vmap`, `assets/vmaps` : bâtiments, ponts, rochers, arbres) s'il est au-dessus du terrain ou plus proche ; ignoré s'il s'écarte de plus de 3 yards du navmesh (autre étage) ; lissage z sauf premier et dernier point. Ancienne méthode (entonnoir + interpolation) en secours. Vitesse horizontale constante, heartbeat 250 ms. Arrivée = distance horizontale <= radius ET écart de hauteur <= 3. Testé : 6 trajets réels d'Elwynn, 100 % des points sur la surface, < 20 ms par km.
- Limites : une seule carte à la fois (self.map), pas de nage/vol/transports/ascenseurs, pas d'évitement dynamique ni d'interruption sur combat, pas d'ack des changements de vitesse/téléports serveur.
