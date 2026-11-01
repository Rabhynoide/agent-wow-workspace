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


# Archivé depuis BACKLOG.md le 2026-11-01 (éléments faits et sections historiques)

## Étape 1 : Elwynn rapide (niveaux 1-10), mesurable dès maintenant
- [x] (2026-10-24, adopté, série QR 36,6 min 0 mort : `safe.AggroRangeOf`, `world.creature` detection_range, adds 3D + hauteur 3 + entraide 10 dans combat/pull.go et auto/clear.go) **PRIORITÉ** (2026-10-24) Rayon d'aggro réel dans le choix des pulls et des adds (Creature::GetAggroRange / CanStartAttack lus) : rayon = detection_range (colonne de creature_template : 18 pour tous les monstres d'Elwynn, 20 par défaut) − (niveau joueur − niveau monstre), borné 5-45, × Rate.Creature.Aggro (1), distance 3D ; pas d'aggro si écart de hauteur > 3 yards (CREATURE_Z_ATTACK_RANGE) ni sans ligne de vue ; entraide : CreatureFamilyAssistanceRadius 10 yards après 2 s (CreatureFamilyAssistanceDelay 2000), fuite à basse vie : 30 yards. Aujourd'hui pull.go / safe.AggroRange prennent 20 et ignorent la hauteur et l'entraide. Les morts des séries SR, M, RF sont presque toutes des meutes dans la mine.
- [x] (2026-10-24, adopté avec la série QR, table des stays vidée puis réapprise ; rencontres, combat/encounter.go) Juger une décision de fuite « restée » à la fin du combat (tous les attaquants morts ou semés), pas à la mort de sa cible : explorations 4:10 et 4:20 notées « won » puis mort 6-7 s plus tard (série RF runs 1 et 4). Sur SR+M+RF : rester avec 3 frais meurt 3/46, fuir 4/12 (la fuite par le sentier traverse la mine).
- [x] (2026-10-24, adopté, série RF 31,6 / échec / 36,7 / 32,8 : 1 repos interrompu sur 35 contre 15/32 ; reste fait le 2026-10-24 : monstres proches vérifiés AVANT restSpot)  (2026-10-24, série M) S'asseoir pour manger à côté d'un monstre : 15 repos sur 32 interrompus par une attaque (assis avec un repas, perte médiane 8 points, coups critiques sur un personnage assis), 1 mort assis (run 1 : kobold à 2 yards, mort 1,6 s après le repas), et la mort 2 du run 2 après deux repos interrompus de suite. aggressor ne voit pas l'attaquant (cible serveur en retard). À faire : pas de repas ni de s'asseoir avec un hostile à < ~8 yards ou sous le drapeau de combat ; attendre debout (52 → 100 % en 4 s mesuré).
- [x] (2026-10-24, `eat_from_level` 10 adopté avec la correction du repos, repos debout 3-7 s) Repos sans manger aux bas niveaux (mesuré le 2026-10-24 sur le log de combat des 23-24/10, ~2 000 intervalles entre combats, niveaux 6-7) : la vie ne remonte pas les 5 premières secondes après un combat (drapeau de combat), puis d'environ 5 %/s (+39 points médians pour un intervalle de 5-10 s), donc 50 → 90 % en ~10 s debout. Repas : 453, dont 58 % suivis d'un combat dans les 15 s (vie 61 → 58 % pour les intervalles < 10 s avec repas) ; avec 20-40 s de pause, 46 → 100 %, à peine mieux que sans. Repos total 0,7-3,4 min par run (72 pauses, médiane 10 s). Le joueur humain (session du 14/10) ne s'assoit jamais et mange 5-18 s. Pistes : ne plus manger sous un niveau à fixer (attendre debout, sans attaque ni sort), repartir à ~70 % et finir la régénération en marchant ; journaliser chaque repos (durée, vie avant/après, repas) pour mesurer la régénération par niveau.
- [x] (2026-10-24, `vertical_give_up` adopté, série SR 38,9 / 37,4 / 40,2 / 35,7 = 38,1 min, 2 morts contre 39,0 et 4 : 6 cas abandonnés en 5-9 s sans fuite ni mort ; reste à terme : refuser les liens de navmesh entre étages) Cible à la verticale dans la mine de Fargodeep (2026-10-23) : le navmesh relie directement deux étages à 8 yards l'un de l'autre ((-9894,255) z 26,9 ↔ 35,1 en 1 yard ; (-9871,237) 27,8 ↔ 19,6), donc la marche « walking to its floor » se dit arrivée sans changer d'étage (z_after = my_z) et le combat attend encore l'abandon (25 s sans dégâts, 48 s au total). Série RB75 (2026-10-23, run 3 Benchkbgkpn) : une mort de ce cas, l'abandon à 94 % de vie suivi d'une fuite par le sentier à travers la mine (5 kobolds). À faire : abandonner dès que la marche n'a pas changé l'étage, fuir vers la sortie plutôt que par le sentier ; à terme, refuser les liens de navmesh dont la montée dépasse ce que le serveur accepte.
- [x] (2026-10-23, `auto/chest.go`, `quest.chests`, mesuré avec la vente en passant, série 4f243f0 : 40,4 / 36,8 / 37,6 / 37,4 = 38,1 min, 0 mort, contre 39,5 et 2 morts, adopté ; 0 à 3 coffres ouverts par run, une variante de Food Crate passe le filtre) Coffres : un coffre en vue (40 yards) passe avant la quête, le combat en cours fini, chemin nettoyé. Avant : ouvrir les coffres du décor croisés en route (butin vert : épée à deux mains). Vérifié le 2026-10-23 : rien de fait, seuls les objets de quête sont ouverts (`quest.use`, sort 22810 pour les coffres verrouillés). À faire : chercher les coffres (gameobject type 3 hors quête) dans un petit rayon pendant la marche et entre deux combats, détour court seulement sans monstre à côté, ouvrir et ramasser. Gain indirect (arme verte à deux mains au 6-10) ; plus utile après Elwynn.
- [x] Contournement des monstres (`move/avoid.go`) mesuré contre son absence (variante E, 2026-10-17) : sans, 46,6 min contre 58,6 ; désactivé (`avoid` false), la marche passe de 38 % à 22-26 %.
- [x] (2026-10-22, fd87e40) Stays appris défaits une seconde plus tard : la décision était refaite chaque seconde, un 4e kobold la faisait tomber dans une case peu remplie et la règle fuyait (9 sur 16 stays appris à 3 attaquants). Une décision apprise « rester » tient maintenant tant que la marge attendue reste ≥ 0 : 40,6 min sur 4 runs contre 45,3 (gain probable, de l'ordre de la variance), autant de fuites mais 28 combats restés contre 21.
- [x] (2026-10-22, 59a81a3) Fuir un gros paquet coûte aussi des morts : la règle ne compte que flee_cost_s fixe. Devant 4+ kobolds la fuite meurt ~7 % (4/54), à <25 % de vie 12 % (7/56) ; mort de Bencharxjwk : fuite à 90 % devant 4 kobolds frais. Piste : apprendre aussi le taux de mort des fuites par case (lignes « flee outcome » stayed=false) et comparer stay vs flee avec les deux risques. Fait : flee.json apprend aussi les fuites par case (238 reprises de combat.log), une fuite coûte flee_cost_s + P(mort en fuite) × death_cost_s, et « rester » est exploré sans marge là où fuir tue ≥ 20 % (`explore_flee_death`). Série 4 runs : 42,7 / arrêté (33,0 min niveau 7, 4 morts) / 41,9 / 35,9, 7 morts contre 2 ; neutre : aucune décision apprise ne change et aucune exploration dans les nouvelles cases ne s'est produite, les morts viennent toutes de stays appris déjà « rester » avant. Gardé (à jauger sur plus de runs).
- [x] (2026-10-22, 59a81a3) Mort en mangeant (Benchbevgak 11:58) : attaqué pendant le repas, aggressor() ne voit pas les attaquants (cible ou drapeau de combat pas encore à jour ?), le bot remange et ne se défend qu'à 5 %. Rare (2 fois depuis le 13/10). Piste : après « attacked while resting » sans aggressor, prendre toute unité en combat au contact. Fait : 5 s après un repas interrompu, aggressor prend toute unité en combat qui nous vise (abandonnée ou non), sinon la plus proche en combat au contact (log « attacker after a broken rest ») ; pas encore vu en jeu.
- [x] (2026-10-22, variante AC08 `add_cost=0.8` mesurée et rejetée : 45,8 / 42,9 / 41,3 / 44,7 = 43,7 min contre 40,6, 7 morts contre 1 ; le bot reste presque toujours, enchaîne les paquets sans repos et meurt en fuyant à 10-30 % de vie) Fuites restantes de la mine : 4-5 kobolds entre 72 et 87 % de vie, marge attendue sous 0 parce que `add_cost` 1,5 fait ×7 pour 5 attaquants. Mesure (combat.log, tous benchs, combats gagnés) : perte réelle 12,5 % seul contre un kobold, 20 % à 2, 27 % à 3, 35 % à 4, 58 % à 5 (n=3), soit ≈ +0,6-0,8 par add ; le modèle prévoit 31/50/69/87. Fuir coûte aussi : la fuite devant 4 kobolds perd 32 % de vie en route (2 morts sur 40), devant 5 : 46 % (2 morts sur 14). La plupart des morts à 4-5 viennent de la fuite, pas du combat.
- [x] (2026-10-22, `combat/service.go`, commits 5708e42 puis df9cb0c « s'écarter une fois », série df9cb0c 41,6 / 36,8 / 41,5 / 38,1 = 39,5 min contre 40,6, adopté ; reste : un kobold bloqué sous le sol qui ne frappe pas attend encore noProgress, l'abandonner sans fuir) « unhittable attacker » 475 (32 fois) : kobold juste dessous/dessus (dist 0-1, dz 5-9, étages de la mine). dist3 > meleeRange 4,5 : le bot marchait sur place sans frapper jusqu'à noProgress (25 s) puis fuyait, alors que la portée de mêlée du serveur (5 + les deux combat reach) le laisse frapper. Correction : frapper quand dist < 2, le serveur juge (tooFar). Insuffisant (1 cas par run encore, runs 1-2 de AC08) : le serveur n'envoie SMSG_ATTACKSWING_NOTINRANGE qu'une fois tant qu'aucun coup ne porte (m_swingErrorMsg), le bot ne voit pas le refus. À faire : après ~5 s sans dégât avec dist < 2 et |dz| > 4, reculer de ~6 m pour que le kobold nous rejoigne au même étage, puis reprendre le combat.
- [x] (2026-10-23, `auto/town.go`, `town.make_room`, série 4f243f0 adoptée avec les coffres : 7-10 arrêts en passant par run, 0-5 piles jetées, plus aucun aller-retour pour vendre ; reste : la nourriture n'est presque jamais achetée, budget 1/3 de l'argent trop court au niveau 6) Ville seulement pour quêtes, maître, réparation ; vente quand un vendeur est à ≤ 40 yards ; sacs pleins : jeter le moins cher pour un objet qui vaut plus. Avant : Butin (2 %) et sacs : ne ramasser que ce qui vaut quelque chose quand les sacs sont presque pleins. État vérifié le 2026-10-23 : `ensureSpace` (avant chaque combat de quête et chaque rendu) va au marchand le plus proche sous 3 places libres, vend selon `sellrule`, répare et rachète ; en combat, « bags full » à 0 place, vente puis reprise (2 fois au plus). 99 visites dans l'historique, dont 23 sacs déjà pleins (le dernier butin les remplit) ; 1 seule sur la série 59a81a3 (Benchttvzuy 29m40s, ~2 min avec les combats en route). Manques : jamais de gris jeté sur place, aucun tri du butin sacs presque pleins, le trajet coupe la quête (depuis le fond de la mine). Pistes : sous 3 places, ne ramasser que quête/vert+/objets chers ; détruire le gris le moins cher au lieu d'un aller-retour quand le marchand est loin.

## Étape 2 : quitter Elwynn (niveaux 10-20)
- [x] Changer de zone : module `travel` (2026-10-25) : vols (découverte en passant, vol express), portails de zone (tram à pied), bateaux (horaire du serveur porté), pierre de foyer ; marches refusées à travers les zones de plus de 5 niveaux au-dessus. Testé : Elwynn → Ironforge, vol Ironforge → Stormwind, bateau Stormwind → Auberdine. Suite : section « Voyages longue distance ».

## Audit du 2026-10-10 (après la première revue) : corrigé
- [ ] Reste : danger des GROUPES de monstres (la règle de niveau laisse passer les quêtes de la mine : 60, 47, 87) → pathfinding/aggro (Opus).

## Revente et inventaire (fait le 2026-10-10)
- [ ] Le prix attendu (156 pc) dépasse le gain mesuré (134 pc) : 22 pc d'écart non expliqués (prix de vente ajusté par le serveur ?). Le 2e test (44 pc attendus, 44 reçus) était exact. Comparer par objet si ça se reproduit.
- [ ] Ne ramasser que ce qui vaut quelque chose quand les sacs sont presque pleins (ignorer les gris de très faible valeur, loot sélectif dans `loot.Take`).
- [x] Équiper automatiquement les sacs trouvés/achetés (aujourd'hui gardés, jamais équipés) : plus d'emplacements, donc moins de ventes.
- [ ] Manger/boire : `combat.rest` ne mange ni ne boit ; la nourriture gardée ne sert à rien tant que ce n'est pas fait (gain de temps de repos).
- [ ] Argent pour l'entraînement : `progress.train` achète du moins cher au plus cher ; si l'argent manque, noter le déficit dans le statut.

## Voyages longue distance (module travel, idées du 2026-10-25)
- [x] (2026-10-25, `cmd/travelgraph`, MODULES « travel » ; danger = monstres près du chemin au lieu du niveau d'exploration ; reste : test en jeu, aubergistes utilisés pour le lien) **Graphe de trajets précalculé hors jeu** (comme mod-playerbots, `TravelNodeMap` et tables `playerbots_travelnode*`) : nœuds fixes (maîtres de vol, entrées et sorties de portails, pontons des bateaux, auberges) et marches entre nœuds proches calculées une fois (longueur, niveau d'exploration maximal des zones traversées), dans un fichier de `journal/` lu par `travel`. En jeu, ne vérifier que la marche position → premier nœud et dernier nœud → destination. Supprime le problème des 30 s au lieu de le contourner par des reprises (`busy`). Choix du plan selon le niveau du personnage au moment du voyage.
- [x] (2026-10-25 : `cmd/roads`, `nav.Filter.Road`, MODULES « move » ; 2-5 % → 31-63 % du chemin sur une route ; à mesurer en bench : moins de combats en marchant ?) **Suivre les routes** (idée de l'utilisateur, 2026-10-25) : les routes sont des textures du terrain du client (ADT dans les MPQ 3.3.5 : MTEX `…Road…`, `…Cobblestone…`, `…Path…`, couches MCLY, opacité MCAL 64 × 64 par carré de 33 yards). Ni `maps/`, ni `mmaps/`, ni la base ne les ont (textures jetées à l'extraction). Il faut le dossier `Data` du client 3.3.5 (aucun MPQ sur la machine, demandé à l'utilisateur) → lecteur ADT hors jeu → grille « route » par carte → l'A* de `move` et le graphe paient moins un pas sur la route (× 0,6 ?). Légal (données du client). En attendant, le danger des marches (monstres près du chemin) favorise déjà les routes.

## Déplacement (idées des bots du serveur, 2026-10-10)
- [x] Évitement des monstres hostiles dans l'A* (coût autour des monstres, `move/avoid.go`, 2026-10-13).

## Test du guerrier Claudio (2026-10-10) : ouvert
- [~] Overpower après une esquive ou Taste for Blood, Victory Rush selon l'aura state (fait, 2026-10-11) ; recharge de Charge suivie entre combats. Reste : Charge refusée pour ligne de vue (47) malgré `move.los` (doodads M2 ou relief).
- [~] Soins du guerrier : nourriture achetée et mangée (fait) ; reste Premiers soins (compétence + bandages).
- [ ] Portée visible : le serveur n'envoie que les monstres proches (~100 yards), donc les chasses « rayon 150 » sur des créatures éloignées échouent vite ; `auto.grind` saute, mais `world.spawns` pourrait guider `hunt` vers les points d'apparition éloignés (déjà fait dans `hunt` ; à vérifier pour `grind`).

## Sécurité du personnage (morts répétées constatées le 2026-10-10)
- [x] Pathfinding qui évite les monstres hostiles (idée utilisateur, fait le 2026-10-13 : `nav.Filter.Avoid`) : voir « Déplacement ». Cause des 3 morts dans la mine de Fargodeep.
- [~] `auto.collect` / `auto.hunt` : monstres élites ou de plus de 2 niveaux au-dessus refusés (fait) ; reste le danger des groupes (quêtes 60, 47, 87 de la mine).
- [~] (point sûr près du corps : fait, à valider sur une vraie mort) Dès le niveau 11 : le corps reste souvent dans le camp qui nous a tués. A faire : avant de réclamer, attendre/éviter les monstres proches du corps (vue du fantôme), ou ne pas partir au corps tant que les monstres n'ont pas quitté la zone ; envisager de fuir plus tôt (voir pathfinding qui évite les monstres). Niveau requis : Opus.
- [~] Après une résurrection, se reposer avant toute autre action (sinon mort immédiate à 50 % de PV) : l'esprit guérisseur redonne ~75 % de PV loin du danger ; ajouter un repos automatique à la fin de `auto.recover`.

## Leveling automatique `level.sh` (2026-10-11)
- [x] (2026-10-13, `logs/level_<Nom>.plan.json`) Les reports (« too dangerous », mort) et les SKIP ne sont gardés que pendant une exécution : relancer retente tout de suite (ex. 47 dans la mine juste après une mort). Les garder dans `logs/level_<Nom>.state` si ça gêne.
- [x] (2026-10-13, `guidecheck/classquests.go` ; reste « Beat Bartleby » 1640, duel scripté, manuel) Quêtes de classe (1638 au niveau 10, 1718 au niveau 30) : pas dans la route Zygor convertie ; à ajouter au plan selon `journal/classes/<classe>.yaml`.

## Revue de mod-playerbots (2026-10-11) : à développer
- [x] Potions/pierre de soins à PV critiques (2026-10-13, `combat/potion.go`, achat de 3 potions chez le marchand) ; racials par déclencheur.

## Benchmark 1 → 6 (2026-10-11, voir journal/bench/)
- [~] Quête 62 (mine de Fargodeep) mortelle au niveau 5 : nettoyage progressif du chemin (idée utilisateur, fait le 2026-10-12, à mesurer au bench) ; reste la fuite quand les PV baissent.
- [~] Argent très juste au niveau 5 : réparation avant nourriture et pas de trajets inutiles (fait) ; reste : vendre plus / meilleur butin.

## Bench e9aa9fd (2026-10-12)
- [x] combat.defend bloqué 60 min sur un kobold inaccessible : cibles abandonnées ignorées, 3 min max.
- [x] Limite de temps de level.sh appliquée aussi pendant un job bloqué.
- [x] Revalidé au bench 9eca5b5 : niveau 6 en 55,5 min, résurrection au corps, plus de riposte manquée.

## Bench 9b66e71 (2026-10-12) : L5 en 59 min, 11 morts
- [x] Régression : les cibles « trop entourées » étaient marquées abandonnées, donc ignorées par la défense ; attaqué par elles, le personnage mangeait sous les coups. Liste séparée (`crowded`).
- [x] level.sh comptait « died 4 times » comme 1 mort (3 comptées pour 11) : compte réel.
- [x] Résurrection à l'entrée de la mine : point sûr cherché jusqu'à 37 yards du corps (au lieu de 33).
- [x] Grind lancé au milieu des kobolds de la mine : apparitions triées par isolement, créature en camps seulement sautée par le grind.
- [x] Quête 62 au niveau 5 : 3 morts pendant le nettoyage (kobolds 5-6) au bench c8edda2 ; nettoyage refusé si un gardien est au-dessus de notre niveau (quête reportée).
- [x] level.sh ne compte pas les morts qu'auto.run gère lui-même (3 morts dans la quête 62 comptées 1) : champ `deaths` du statut.
- [x] Journal de combat insuffisant pour expliquer une mort : ligne `fight` par combat, combats perdus dans le rapport.
- [ ] Deux runs finissent « dead » au passage niveau 6 : à comprendre.
- [ ] Morts en marchant pendant le grind (riposte en route) : éviter les monstres sur le chemin.

## Bench table de priorités (2026-10-12) : niveau 5 en 52 min, 5 morts
- [x] Moteur de priorités en table (`combat/priority.go`) : combats simples identiques à l'ancien code (voir journal/bench/20261012_2045_Benchgnnqye.md).
- [x] Morts en enchaînement contre les Mangy Wolf (525) au niveau 5 : P_mort mesurée par monstre et par écart (`xpscore`), repos avant toute marche sous 40 % de PV. Bench a12ac5c+ : niveau 6 en 53,6 min, 2 morts (au lieu de 5).
- [x] Score à la place de « niveau + 1 » et de la capacité de pull (bench 2026-10-12 23:01) : niveau 6 en 51,4 min, 3 morts, pas plus de combats à plusieurs. Morts restantes = loups errants qui rejoignent un combat en cours.
- [x] Fuite quand la perte restante prévue dépasse les PV − 15 % (`combat/flee.go`, 2026-10-13) : bench 47,8 min, 0 mort (record), 5 fuites réussies. Zone fuie évitée 5 min (pull et points d'apparition du grind, 2026-10-13 : bench 50,8 min / 0 mort, 1 seule fuite, zone jamais sollicitée : non vérifiée en jeu). Reste : chemin réel (navmesh) qui traverse une zone, relevé de PV à 100 % juste après une fuite à vérifier. Fuite plus tôt dès 3 attaquants frais et retour sur nos pas (`combat/trail.go`, 2026-10-13, après les 2 morts de la mine de Fargodeep au bench 6 → 10 : à mesurer). Bench 12:33 : 13/16 fuites réussies, morts en y retournant => un job kill s'arrête à la première fuite, la marche aussi (`CombatStatus.fled`), « fled » confirmé deux fois, repos sur 3 lectures. « self health jump » (module state) : sauts de +20 % des PV max par SMSG_COMPRESSED_UPDATE_OBJECT = Victory Rush (soin de 20 %), donc lectures réelles ; le 100 % juste après une fuite est réel (régénération hors combat des bas niveaux : +18 %/2 s au niveau 8). Bench 13:09 : 0 quête en 100 min (fuite contre 3 kobolds gris => quête reportée à chaque niveau) => règle des 3 attaquants seulement si perte prévue > 30 % ; marge +10 % par attaquant ≥ +2 niveaux (2 morts face au Riverpaw Outrunner). Un report de quête bloquait toute la route (linéaire) : `nextBatch` passe maintenant les quêtes reportées (comme celles en attente de réapparition) et ne les rend bloquantes que s'il ne reste rien d'autre à faire (2026-10-13). Reste : monstres plus rapides que nous (speed_run > 1) et ralentissements non gérés.
- [x] Marche vers le maître de classe (jusqu'à 400 yards, `progress.train` via ApproachNpc) sans riposte : mort au départ d'un snapshot à la mine (2026-10-13). `auto` marche lui-même vers le maître (`TrainCheckResult.x/y/z`) avec riposte avant `progress.train`.
- [x] Cible abandonnée (« not on the navmesh ») qui nous frappe au corps à corps ignorée par `aggressor` : mort en mangeant (10 bananes interrompues, 2026-10-13). Elle est de nouveau combattue à portée de mêlée (même niveau, `atMelee`).
- [x] Apprentissage sans oubli (question utilisateur 2026-10-13) : les morts d'avant les fuites pesaient autant que les combats récents. `xpscore` : au-delà de 40 combats par case, compteurs ramenés à 40 (`keepFights`, `forget`).
- [x] Équipement ramassé jamais porté pendant les longs grinds (remarque utilisateur 2026-10-13 : cotte, bottes et brassards de mailles dans les sacs) : `progress.gear` n'était appelé qu'au début d'un job, aux niveaux et après une quête ; `auto` l'appelle maintenant après chaque tournée de chasse (`huntAt`).
- [x] Sacs non équipés et parchemins jamais lus (remarque utilisateur 2026-10-13) : `progress.gear` équipe le plus grand sac ordinaire dans un emplacement libre, `combat` lit les parchemins avant un pull.
- [x] Élixir de force jamais bu (remarque utilisateur 2026-10-13) : parchemins généralisés en objets de buff (élixirs, flacons, parchemins) filtrés par l'utilité de leurs auras pour la classe, utilisés une fois la cible choisie (plus avant la recherche de cible, pour ne pas les gaspiller dans une marche). À vérifier en jeu (« using buff item » dans les logs).
- [~] Morts hors combat (bench 2026-10-13 10:39, 2 morts) : Defias Cutpurse (94) abandonné pour « no damage dealt » (25 s sans dégâts) mais qui nous frappait toujours ; les cibles abandonnées ne sont pas combattues en riposte et aucune fuite n'était possible hors combat. Corrigé : `giveUp` (flee.go) fuit quand la cible abandonnée nous attaque, et journalise `unhittable attacker` (distance, dz, refus de mêlée, flags). Reste : comprendre POURQUOI nos coups ne le touchaient pas (lire ces lignes au prochain cas).
- [ ] Les 2 morts restantes : loup + Defias Cutpurse niveau 6 ensemble ; le Cutpurse achève après le loup (49 % puis 14 % de PV). Pas de fuite ; envisager la fuite ou un défensif (Intimidating Shout n'est appris qu'au niveau 22).

## Fait les 2026-10-13 et 14 (benchs Benchtrhoxd, Benchnwhuzf, Benchzcoilj, Benchekrosq ; analyses dans journal/bench/)
- [x] Contournement des monstres dans l'A* (`nav.Filter.Avoid`), recalculé en marchant quand un monstre apparaît près des 80 prochains yards, zones fuies comprises (Cutpurse de la route de Goldshire : 6 fuites puis aucune).
- [x] Potions de soin : bues sous 35 %, comptées dans la décision de fuite (30 % de PV), achetées (3 visées) chez un vendeur de potions à moins de 500 yards ou autour du dernier marchand atteint ; PNJ d'événement exclus de `world.find_npc`.
- [x] Élixirs, flacons, parchemins utiles à la classe utilisés juste avant un pull.
- [x] Points d'apparition derrière une fuite sautés (`auto.behindFled`, 80 yards) : la mine coûtait 4 fuites par quête.
- [x] guidecheck : grind élargi à 600 yards sous 2 cibles ; repos + attente 90 s quand il ne trouve rien à chasser (zones fuies) au lieu de 4 tours dans la même seconde ; reports et sauts gardés entre deux exécutions ; quêtes de classe insérées dans la route.
- [x] Bench : répartition du temps (`bench_sample.py`), snapshots en route (`SAVE_AT`), modules construits un par un avant le lancement.

## Prochaines étapes suggérées (2026-10-11, remplacé par « Objectif speedrun 1 → 80 »)
- FAIT : suivi des auras et aura state (« Rotation et combat » étape 1). FAIT aussi : « peut lancer ». SUITE : moteur de priorités en table, rotations Guerrier/Prêtre en tables ; tester le prêtre (Claudia) en jeu.
- Pull où l'on est susceptible de gagner (voir « Points à réfléchir ») et capacité qui apprend des combats.
- Continuer Claudio avec `level.sh` (11 → 20) : quête 87, quête de classe 1638 (posture défensive), Westfall à partir de 13-14 (voyage à pied).
- Gros test visuel du déplacement (escaliers, ponts, intérieurs).
- Claudia (prêtre 6 → 10) avec `level.sh` une fois la rotation prêtre refaite.

## Fait les 2026-10-13 et 14 (benchs Benchtrhoxd, Benchnwhuzf, Benchzcoilj, Benchekrosq ; analyses dans journal/bench/)
- [ ] Mesurer ces changements selon le protocole (2 benchs 6 → 8) : ils ont surtout réduit les morts (3-4 → 0-1 par run) sans gain de vitesse visible (6 → 9 : 84 min avant, 92-110 après).
