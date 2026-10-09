# NOTES

## Etat (2026-10-09)
- Personnage : Claudia (humain prêtre, guid 1), niveau 1, Northshire Abbey (~-8903,-160, map 0). Vérifié: hp 72, mana 123.
- Aucune quête prise. Inventaire de départ seulement.
- Modules actifs : state, world, move (voir MODULES.md). Prochains : quest, combat, town.

## Environnement
- Serveur AzerothCore en Docker local (ac-worldserver/authserver/database). Realm 127.0.0.1:8085, DB `acore_world` accessible (lecture seule) sur 192.168.100.7:3306. Pas de client mysql sur l'hôte : `docker exec ac-database mysql -uroot -ppassword -N <db> -e "..."`.
- Mmaps copiés dans `./assets/mmaps` (gitignoré, 2.1G) depuis le volume ac-worldserver.
- Démarrer : `nohup agent-wow char play Claudia > logs/play.log 2>&1 &` puis attendre ~60-90 s (build docker) jusqu'à "world session ready". Quitter : `scripts/rpc.sh session.logout`.
- Vérifier la position côté serveur après logout : `select position_x,position_y,position_z from acore_characters.characters where name='Claudia'`.

## Pièges
- Les appels module ont un timeout de 30 s : les actions longues sont des jobs asynchrones (wait_s <= 25).
- Paquets MSG_MOVE_* client->serveur : GUID packé obligatoire en tête, sinon ignorés silencieusement.
- Noms de fichiers mmtile = indices ADT, pas indices Detour : indexation par en-tête (nav.LoadMap).
- Proto: un rpc ne peut pas porter le même nom que son message de retour (Status, CreatureInfo).
- Le serveur ne renvoie pas la position du joueur : move pousse la position vers state.set_pos (heartbeat 500 ms).
