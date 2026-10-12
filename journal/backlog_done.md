# BACKLOG : éléments faits (archivés depuis BACKLOG.md le 2026-10-12)

Historique : ce qui a été fait, par section d'origine. Le BACKLOG ne garde que ce qui reste à faire ou est partiel.

## Remarques de l'utilisateur
- [x] Les objets de quête ne tombent pas à 100 % : vérifié en base (90 % pour les peaux de loup) ; le job `combat.kill` boucle jusqu'au compte voulu. La vraie cause de l'échec était le sac plein.
- [x] Équiper automatiquement les objets utilisables qui améliorent le personnage : `progress.gear` (+ choix de récompense de quête), appelé par `auto` entre les quêtes.
- [x] Build de leveling : `progress.train` (entraîneur de classe, testé en jeu), `progress.talents` (build Ombre, testé hors-ligne : premier point au niveau 10, à vérifier en jeu), `combat` utilise Bouclier/Rénovation/Douleur/Attaque mentale/baguette.
- [x] Le personnage était mort/fantôme (1 PV) : détection du fantôme via PLAYER_FLAGS, `auto.recover` (libérer l'âme, courir au cadavre, ressusciter), reprise automatique dans `auto.run`.
- [x] Consigner ces remarques : ce fichier.

## Problèmes connus (moi)
- [x] Coffre « Milly's Harvest » : verrou « Open Kneeling » => sort 22810 lancé par `quest.use`.
- [x] Vente de camelote : règles `sellrule` + distance 5 yards + PNJ « [DND] » ignorés (voir « Revente »).
- [x] Hostilité (factions) : `state.nearby` donne la réaction (FactionTemplate.dbc) et filtre `hostile_only`.
- [x] `go vet` signalait des copies de messages protobuf (move/combat/auto) : package `jobs` + `proto.Clone`.
- [x] Compteurs d'objectifs de quête : 4 x uint16 sur 2 champs (était lu comme 4 octets d'un seul champ) ; indexés par emplacement RequiredNpcOrGo.

## Audit du 2026-10-10 (après la première revue) : corrigé
- [x] `guidecheck -plan` : niveau minimum (était +1), prérequis (rendus ou planifiés avant), quêtes de classe sans `only`, `only <race>` seul, filtre de danger `-maxabove 2`, types manuels (élite/donjon/raid/escorte/PvP). Tests unitaires (`plan_test.go`) ; `zygor2route` testé et corrigé (`only` en tête d'étape).
- [x] `grind.sh` : champs à zéro omis (XP 0 après un niveau) et session fermée.
- [x] `combat` : un échec de sort n'est attribué qu'à notre lancer (compteur + sort) ; buffs notés seulement s'ils ont réussi, oubliés à la mort.
- [x] `auto` : entraîneur à 400 yards max, monstres élites ou trop hauts refusés, kills conservés d'un point d'apparition à l'autre, talents par lots, visite au marchand qui répare (durabilité < 60 %).
- [x] Réparation (`town.repair`) et durabilité (`state`) ; `world.find_npc` ignore les PNJ « [DND] » (ils masquaient les vrais marchands : cause probable de l'échec de vente inexpliqué).
- [x] Revente : les réactifs sont gardés.

## Revente et inventaire (fait le 2026-10-10)
- [x] `town.sell_junk` vend aussi les objets blancs inutiles selon des règles (voir MODULES.md : équipement inutilisable ou sans gain, marchandises, nourriture en trop, consommables obsolètes à 10 niveaux d'écart). Testé en jeu : 12 piles, 134 pc, 12 emplacements libres.
- [x] `dry_run` + `show_kept` : détail (prix, raison) avant de vendre. Distance de vente 5 yards (le serveur refuse au-delà de 5,5 ; l'ancien seuil de 8 yards faisait échouer des ventes en silence). `auto.ensureSpace` suit le marchand jusqu'à 5 yards.
- [x] Les objets vendus (liste de rachat) ne sont plus comptés dans `state.snapshot`.
- [x] Réparation : `town.repair` + passage automatique quand la durabilité < 60 %. Rachat : non nécessaire.

## Déplacement (idées des bots du serveur, 2026-10-10)
- [x] Lissage du chemin collé à la surface (port de `findSmoothPath`/`moveAlongSurface`) : fait, testé hors ligne et en jeu (auberge de Goldshire, poursuite en combat). A valider visuellement lors du « gros test » (escaliers, ponts, virages serrés).
- [x] Hauteurs vmaps (port de StaticMapTree/BIH/WorldModel du serveur, `internal/vmap`) : sols des bâtiments à 0,1-0,2 yard des créatures de référence (auberge sur 2 étages, abbaye, cathédrale), choix terrain/vmap comme `Map::GetHeight`. Testé en jeu (étage de l'auberge par l'escalier). Ligne de vue : `move.los`, testée (plafond bloque).
- [x] Ligne de vue en combat : le prêtre se rapproche (mi-distance) quand la cible n'est pas visible, le guerrier ne charge que s'il voit la cible ; l'ancien repli « mêlée après 3 échecs » reste en dernier recours. Testé en terrain dégagé (pas de régression) ; à observer près des bâtiments pendant le gros test (log « no line of sight, moving closer »).

## Nage, eau et quêtes d'eau (question utilisateur, 2026-10-11)
- [x] `hunt` essaie le point d'apparition suivant quand l'un est hors navmesh ; chemin partiel désactivé par défaut (seul `GroundPartial` l'active).

## Test du guerrier Claudio (2026-10-10) : ouvert
- [x] « Fight timeout » et mort : analysée (personnage laissé sans job parmi des loups après l'arrêt de `auto.grind` ; coups refusés par le serveur) ; corrigé : garde d'`auto` (`combat.defend`), `auto.grind` tolérant, refus de mêlée écoutés (portée/orientation), abandon d'une cible sans dégâts après 25 s. Validé 15 min sans mort.
- [x] Parry : en fait utile (permet de parer), pas un défaut. (Si un sort doit être exclu un jour : `training.skip` du fichier de classe.)
- [x] Danger par classe, rotation (Battle Shout/Charge/Heroic Strike), boucle de `grind.sh` (voir `journal/characters/Claudio.md`).

## Sécurité du personnage (morts répétées constatées le 2026-10-10)
- [x] Téléportations jamais confirmées (MSG_MOVE_TELEPORT_ACK / WORLDPORT_ACK) : après chaque mort le serveur ignorait nos mouvements. Corrigé dans `state` le 2026-10-11 (aussi vitesses forcées et enracinement). Cause probable des « attacked while idle: defending » répétés toutes les 4 min pendant des heures à la lac : à revoir si ça réapparaît.
- [x] Mort pendant une marche : `move` s'arrête désormais (« character died »).
- [x] `auto.recover` : résurrection chez l'esprit guérisseur du cimetière s'il est à moins de 150 yards (le corps est souvent dans le camp), niveaux 1-10 seulement (malus de résurrection dès le niveau 11, `Death.SicknessLevel`) ; sinon course au corps avec attente jusqu'à 4 minutes (délai de récupération croissant). Testé en jeu au niveau 5.
- [x] `scripts/grind.sh` : chasse en boucle jusqu'au niveau voulu avec résurrection et arrêt après 3 morts (sans mort pendant le test, 580 s, +510 XP).

## Quêtes spéciales à gérer dans `auto`
- [x] Quêtes « explorer une zone » : objectif `explore` (déclencheurs `areatrigger_involvedrelation`), testé en jeu sur 62 et 76 (2026-10-11).

## Sélection des quêtes de la route (décisions utilisateur, 2026-10-11)
- [x] Quêtes déjà faites : sautées.
- [x] Quêtes interdites à la race/classe : sautées.
  - [x] Crédit de kill par d'autres créatures (KillCredit1/2) : fait (`credit_from`, `auto` chasse la plus proche). Non testé en jeu (quêtes 58+). Attention : certaines de ces créatures sont alliées (prisonniers à libérer, villageois à sauver : 11255, 12237) : ce sont alors des événements, pas des kills ; à traiter quand on y sera.
  - [x] Zones à explorer (24) : fait et testé.
  - [x] Objet de quête à utiliser sur une cible (45 des 48) : fait (`use_item`, `quest.use_item`). CMSG_USE_ITEM validé avec la pierre de foyer ; sur une cible, pas encore testé (première quête : niveau 14+). Restent 3 sans objet ni cible connus (ex. 2118 Plagued Lands : capturer un ours par script). Limite : les quêtes où il faut d'abord affaiblir/tuer la cible puis utiliser l'objet ne sont pas gérées.
  - [x] Groupe, élite, donjon, JcJ : abandon (décision utilisateur).

## Leveling automatique `level.sh` (2026-10-11)
- [x] « Continue le leveling » : `scripts/level.sh` (route Zygor dans l'ordre + grind d'un niveau quand la quête suivante est trop haute). Testé : Claudio 9 → 10.

## Revue de mod-playerbots (2026-10-11) : à développer
- [x] Repos réel : manger/boire au repos, achat de nourriture/boisson de son niveau (fait et testé le 2026-10-11).
- [x] Marche qui riposte quand un monstre attaque en route (fait et testé : niveau 10 → 11 sans mort, quêtes des murlocs 112/114 faites).
- [x] Réaction des unités (hostile/neutre/amical) dans `state` (FactionTemplate.dbc).

## Rotation et combat : sources étudiées le 2026-10-11 (à faire, Opus)
1. [x] (fait et testé le 2026-10-11 ; utilisé par les rotations guerrier et prêtre) Aura state (UNIT_FIELD_AURASTATE) et auras (SMSG_AURA_UPDATE / _ALL : buffs, procs, DoT avec durée restante, « les miens »), pour le personnage et la cible. Conditions des sorts lues dans Spell.dbc : CasterAuraState (champ 20 ; Victory Rush = 10), TargetAuraState (champ 21 ; Execute = 2 « moins de 20 % »), Revenge = 1. Règle l'erreur 67 de Victory Rush (aura « Victorious » 32216).
2. [x] (fait le 2026-10-11 : recharges, coût, aura states, portée ; testé) « Peut lancer » générique : sort connu, recharge, ressource, aura state, procs (comme `usable=1` d'Ovale et CAN_CAST de playerbots).

## Leveling guidé par des guides extraits (décidé le 2026-10-10)
2. [x] `cmd/guidecheck` : validation contre la base + liste pour `auto.run` (voir MODULES.md).

## Benchmark 1 → 6 (2026-10-11, voir journal/bench/)
- [x] Capacité : la cible seule compte au plus 1 (un Thug niveau 4 seul était écarté au niveau 2).
- [x] Repos après toute résurrection (esprit guérisseur compris).
- [x] Étapes « reach this level N » de la route respectées.
- [x] Réparation impossible (pas d'argent) non bloquante ; pas de nouvel essai avant d'avoir gagné 1 pa.
- [x] Sorts refusés (Rend hors de portée, Heroic Strike, Thunder Clap pas prêt, Charge trop près) : « peut lancer » (5 loups : 1 refus au lieu de 8).
- [x] Quête dont les monstres sont morts/absents : reportée de 4 min (3 essais) au lieu d'être sautée (run de référence : quête 18).
