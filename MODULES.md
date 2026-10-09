# MODULES

Sources Go partagées : `module/` (un seul go.mod, un binaire par module dans `module/cmd/<nom>`).
Manifestes + descripteurs : `modules/<nom>/` (module.yaml, compose.yaml, module.pb).
Régénérer protos : `cd module && make generate` (met à jour `modules/*/module.pb`). Les images se reconstruisent au `char play`.
Appel : `scripts/rpc.sh <module.methode> '<json>'` (sortie JSON compacte). Logs : `logs/<module>.log`.
Test navmesh hors-ligne : `cd module && go test ./internal/nav -v`.

## state  (paquets -> état du monde)
- `state.snapshot {}` : self (guid,map,x,y,z,o,level,hp,power,xp,money,target,dead,run_speed), items (slot -1 = sac secondaire), quêtes (id,state,counts), nb objets.
- `state.nearby {kind:"unit|gameobject|player|any", entry, radius, max(15), alive_only, npc_flags}` : trié par distance (guid,entry,pos,dist,level,hp,faction,flags,npc_flags,dyn_flags,target).
- `state.get_obj {guid}`, `state.set_pos {x,y,z,o,map}` (utilisé par move).
- Limites : factions/hostilité non résolues ; counts de quêtes (octets du champ 2) non vérifiés ; argent = 0 au snapshot initial à confirmer ; pas de chat/combat log. Positions des mobs = dernier point de spline (approximatif).
- Debug : un paquet d'update non parsable est dumpé dans `logs/state_bad_N.hex` (5 max).

## world  (MySQL acore_world, lecture seule, session `transaction_read_only`)
- `world.find_npc {name|entry, map_filter+map, x,y,radius, npc_flags, max}` : spawns + distance. npc_flags : vendeur 128, réparateur 4096, aubergiste 65536, questgiver 2, gossip 1.
- `world.quest_info {id|title}` : niveau, objectifs (kill/item/gameobject + noms), starters/enders avec coords, quête suivante/précédente.
- `world.quests_for_npc {entry}`, `world.spawns {entry,map,x,y,radius,kind:creature|gameobject}`, `world.item_info {entry|name}`, `world.creature {entry}`.
- Hôte DB : 192.168.100.7:3306 (env dans modules/world/compose.yaml).

## move  (requires: state)
- `move.goto {x,y,z, radius(1.5), wait_s(20, max 25), walk}` : planifie sur navmesh (ground only), lance un job de marche (start/heartbeat/facing/stop, paquets avec GUID packé), bloque jusqu'à wait_s. Retourne state moving|arrived|failed|stopped, reason, pos, dist_left, eta_s.
- `move.status {wait_s}` : attend la fin du job (max 25 s par appel — timeout RPC 30 s). `move.stop {}`. `move.path {x,y,z}` : dry-run (points, longueur).
- Pathfinding : `module/internal/nav` (tuiles Detour lues depuis `assets/mmaps`, A* + funnel, ~50 ms pour 2 km).
- Limites : une seule carte à la fois (self.map), pas de nage/vol/transports/ascenseurs, pas d'évitement dynamique ni d'interruption sur combat, pas d'ack des changements de vitesse/téléports serveur.
