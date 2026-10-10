# MODULES

Sources Go partagées : `module/` (un seul go.mod, un binaire par module dans `module/cmd/<nom>`).
Manifestes + descripteurs : `modules/<nom>/` (module.yaml, compose.yaml, module.pb).
Régénérer protos : `cd module && make generate` (met à jour `modules/*/module.pb`). Les images se reconstruisent au `char play`.
Scripts : `start.sh` (démarre la session), `job.sh` (attend un job async), `grind.sh <niveau> <entrée>...` (enchaîne les chasses, repos, résurrection, arrêt après 3 morts / 25 min).
Appel : `scripts/rpc.sh <module.methode> '<json>' [chemin]` (sortie JSON compacte, flottants arrondis). Jobs async : `scripts/job.sh <module.methode> '<json>'` (attend la fin). Logs : `logs/<module>.log`.
Test navmesh hors-ligne : `cd module && go test ./internal/nav -v`.

## state  (paquets -> état du monde)
- `state.brief {}` : self (dont class_id, race, talent_points) + quêtes + free_slots, sans les objets (à préférer).
- `state.snapshot {}` : self (guid,map,x,y,z,o,level,hp,power,xp,money,target,dead,run_speed), items (slot -1 = sac secondaire ou banque), quêtes (id,state,counts[4] uint16), free_slots (sac à dos + sacs équipés), nb objets.
- `state.nearby {kind:"unit|gameobject|player|any", entry, radius, max(5), alive_only, npc_flags}` : trié par distance (guid,entry,pos,dist,level,hp,faction,flags,npc_flags,dyn_flags,target).
- `state.spells {}` : sorts connus. self.ghost/dead (fantôme détecté). `state.get_obj {guid}`, `state.set_pos {x,y,z,o,map}` (utilisé par move).
- Limites : factions/hostilité non résolues ; argent = 0 au snapshot initial à confirmer ; pas de chat/combat log. Positions des mobs = destination de la spline (approximatif ; spline cyclique = position de départ).
- Debug : un paquet d'update non parsable est dumpé dans `logs/state_bad_N.hex` (5 max).

## world  (MySQL acore_world, lecture seule, session `transaction_read_only`)
- `world.find_npc {name|entry, map_filter+map, x,y,radius, npc_flags, max}` : spawns + distance. npc_flags : vendeur 128, réparateur 4096, aubergiste 65536, questgiver 2, gossip 1.
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
- `combat.kill {entry, count, item_entry, item_count, radius(80), max_s(600), no_loot, wait_s}` : chasse l'entrée. Priorités : soin si PV < 40 % (Soins, sinon Soins inférieurs), Bouclier si PV < 80 % (15 s d'Âme affaiblie), Rénovation si PV < 65 %, Mot de l'ombre : Douleur (toutes les 18 s), Attaque mentale (8 s), Châtiment ; sans mana : baguette (Tir) si équipée, sinon mêlée. Robustesse/Feu intérieur hors combat toutes les 25 min. Rangs : `module/internal/priest` (vérifiés contre Spell.dbc). Se défend contre les agresseurs (unités en combat qui nous ciblent), un mob perdu de vue ne compte pas comme tué, repos auto (PV<55 % ou mana<25 %), butin des cadavres. Échoue avec "bags full" si sac plein.
- `combat.rest {}`, `combat.status {wait_s}`, `combat.stop {}`. Statut : state running|done|failed|stopped, kills, items, hp_pct, mana_pct, doing.
- Limites : sorts de prêtre seulement, pas de fuite, pas de ciblage par hostilité, pas de suivi des auras (minuteries).

## town  (requires: state, world, move)
- `town.sell_junk {}` : vend les objets gris du sac à dos au vendeur à <= 8 yards (il faut déjà y être ; `auto` s'en charge).

## auto  (requires: state, world, move, quest, combat, town, progress) — orchestrateur
- `auto.run {quests:[ids], rewards:{id:index}, no_progress, wait_s}` : au départ et après chaque quête (sauf `no_progress`) : `progress.gear`, `progress.talents`, et entraînement si l'entraîneur a des sorts à apprendre à moins de 1500 yards ; récompense choisie par `progress.pick_reward` si absente de `rewards` ; événements dans `progress` du statut. Puis pour chaque quête : va au donneur, accepte, objectifs (kill / objets via la source la plus proche sur la carte / objets de décor), rend. Suit les PNJ qui se déplacent. Reprend après une mort. Gère sac plein.
- `auto.status {wait_s}`, `auto.stop {}`, `auto.recover {wait_s}` (libérer l'âme, puis, aux niveaux 1-10 seulement, ressusciter chez l'esprit guérisseur du cimetière s'il est à moins de 150 yards (malus de résurrection dès le niveau 11 : jamais utilisé ensuite) ; sinon courir au corps et le réclamer en insistant 4 minutes).
- Limites : pas de quête à groupe/élite, une carte à la fois.

## progress  (requires: state, world, move) — rendre le personnage plus fort
- `progress.train_check {}` : sorts que l'entraîneur de classe le plus proche apprendrait maintenant (count, cost, spells, trainer, dist, unaffordable). Source : tables `trainer`/`trainer_spell` + Spell.dbc (sorts appris, race/classe).
- `progress.train {wait_s}` (job async, `progress.status`/`progress.stop`) : va à l'entraîneur, ouvre la liste (CMSG_TRAINER_LIST), achète tout ce qui est disponible et abordable, du moins cher au plus cher. Mémorise les sorts refusés pour le niveau courant (pas de voyage inutile).
- `progress.talents {dry_run}` : dépense les points selon `priest.Build` (Ombre, ordre des guides de leveling Icy Veins / Highground Gaming : Spirit Tap 10-12, Darkness, Mind Flay 20, Shadowform 40, Vampiric Touch 50, fin en Discipline ; 71 points, rejoué par test contre Talent.dbc). Rang courant lu dans SMSG_TALENTS_INFO.
- `progress.gear {dry_run}` : équipe les améliorations du sac (score prêtre, ordre des guides : puissance des sorts 1,2 > Esprit 1,0 > hâte 0,7 > critique 0,6 > Int 0,55 > mp5 0,5 > Endu 0,4 > toucher 0,3 ; armure 0,1 ; DPS baguette 2,0, autre arme 0,15 car la main droite ne donne que des stats). Utilisable = niveau, masques classe/race et sort de compétence connu (ex. dagues 1180 : à apprendre chez un maître d'armes). Bagues/bijoux sur l'emplacement le plus faible (CMSG_SWAP_ITEM), sinon CMSG_AUTOEQUIP_ITEM.
- `progress.pick_reward {items}` : index de la récompense de quête qui améliore le plus l'équipement, sinon la plus chère à la revente.
- Données client : `assets/dbc` (Spell, Talent, TalentTab, SkillLineAbility) monté en `/dbc`.
- Limites : build de talents et poids de stats pour prêtre seulement ; pas de maître d'armes ; pas de comparaison de sacs.

## move  (requires: state)
- `move.goto {x,y,z, radius(1.5), wait_s(20, max 25), walk}` : planifie sur navmesh (ground only) ; si une marche est en cours (même allure), remplace l'itinéraire sans stop/start (poursuite fluide) ; sinon lance un job de marche (start/heartbeat/facing/stop, paquets avec GUID packé), bloque jusqu'à wait_s. Retourne state moving|arrived|failed|stopped, reason, pos, dist_left, eta_s.
- `move.status {wait_s}` : attend la fin du job (max 25 s par appel — timeout RPC 30 s). `move.stop {}`. `move.path {x,y,z}` : dry-run (points, longueur).
- Pathfinding : `module/internal/nav` (tuiles Detour lues depuis `assets/mmaps`, A* + funnel, ~50 ms pour 2 km ; cache LRU de 128 tuiles).
- `move.goto` échoue avec « character died » si le personnage meurt en marchant.
- Jobs async (move, combat, auto) : `module/internal/jobs` (un job à la fois, start/stop sérialisés, stop interruptible).
- Hauteurs : couloir A* + sol réel (assets/maps), lissage z, pas 1,5 yard, heartbeat 250 ms, vitesse horizontale constante. Arrivée = distance horizontale <= radius ET écart de hauteur <= 3.
- Limites : une seule carte à la fois (self.map), pas de nage/vol/transports/ascenseurs, pas d'évitement dynamique ni d'interruption sur combat, pas d'ack des changements de vitesse/téléports serveur.
