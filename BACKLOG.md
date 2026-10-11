# BACKLOG (remarques utilisateur + problèmes connus)

Légende : [ ] à faire, [~] partiel, [x] fait.

## Remarques de l'utilisateur
- [x] Les objets de quête ne tombent pas à 100 % : vérifié en base (90 % pour les peaux de loup) ; le job `combat.kill` boucle jusqu'au compte voulu. La vraie cause de l'échec était le sac plein.
- [x] Équiper automatiquement les objets utilisables qui améliorent le personnage : `progress.gear` (+ choix de récompense de quête), appelé par `auto` entre les quêtes.
- [x] Build de leveling : `progress.train` (entraîneur de classe, testé en jeu), `progress.talents` (build Ombre, testé hors-ligne : premier point au niveau 10, à vérifier en jeu), `combat` utilise Bouclier/Rénovation/Douleur/Attaque mentale/baguette.
- [ ] À vérifier en jeu : talents au niveau 10 (SMSG_TALENTS_INFO, CMSG_LEARN_TALENT), Bouclier/Rénovation au niveau 6-8, baguette (Tir) une fois une baguette équipée.
- [ ] Rotation d'ombre (guides de leveling) : le build prend Mind Flay (20), Shadowform (40), Vampiric Touch (50) mais `combat` ne les lance pas. A ajouter avec les rangs vérifiés par test : Dévorer (Devouring Plague, niveau 20), Attaque mentale puis Fouet mental, Étreinte vampirique, Forme d'ombre (bloque les sorts du Sacré : Bouclier, Soins, Châtiment ; donc à gérer ensemble), Toucher vampirique. Niveau requis : Opus pour la logique de combat.
- [ ] Baguette : les guides la jugent très importante dès le niveau 5 (Tir, sans mana). `progress.gear` la privilégie (poids 2,0), `combat` la utilise une fois équipée ; il faut en obtenir une (butin, récompense de quête ou achat).
- [ ] Maître d'armes (dagues pour le prêtre) : non géré ; `progress.gear` ignore les armes sans compétence.
- [~] Saccades/téléportations sur escaliers et terrain accidenté (remarque utilisateur) : chemin lissé sur la surface du navmesh (2 yards), sol comme le serveur (terrain ou vmaps), heartbeat 250 ms. A revérifier visuellement (gros test).
- [~] Hauteur : le personnage flottait ou s'enfonçait. Corrigé : hauteur du sol calculée comme le serveur (vmaps : 0,1-0,2 yard des références dans l'auberge, l'abbaye, la cathédrale). A revérifier visuellement.
- [~] Le personnage passait sous les ponts / à travers des barrières. Corrigé : chemin qui glisse le long des polygones marchables (plus de coupe de coin) et ponts/bâtiments lus dans les vmaps. Les barrières absentes du navmesh restent possibles : à revérifier visuellement.
- [x] Le personnage était mort/fantôme (1 PV) : détection du fantôme via PLAYER_FLAGS, `auto.recover` (libérer l'âme, courir au cadavre, ressusciter), reprise automatique dans `auto.run`.
- [x] Consigner ces remarques : ce fichier.

## Problèmes connus (moi)
- [~] Éviter de mourir : garde d'`auto` contre les attaques entre deux jobs (fait) ; reste : fuir, utiliser les consommables.
- [ ] `session.logout` est refusé en combat (reason 1) : attendre la fin du combat.
- [x] Coffre « Milly's Harvest » : verrou « Open Kneeling » => sort 22810 lancé par `quest.use`.
- [x] Vente de camelote : règles `sellrule` + distance 5 yards + PNJ « [DND] » ignorés (voir « Revente »).
- [~] Hostilité (factions) non résolue : `combat` ne chasse que des entrées explicites ; les agresseurs doivent être en combat (UNIT_FLAG_IN_COMBAT).
- [~] Durée de démarrage : `char play --module-timeout 8m` nécessaire (9 images Docker) ; `scripts/start.sh` attend à notre place.
- [x] `go vet` signalait des copies de messages protobuf (move/combat/auto) : package `jobs` + `proto.Clone`.
- [x] Compteurs d'objectifs de quête : 4 x uint16 sur 2 champs (était lu comme 4 octets d'un seul champ) ; indexés par emplacement RequiredNpcOrGo.
- [ ] Compte MySQL en lecture seule pour `world` : créer `agent_ro` (SELECT sur acore_world) et remplir `modules/world/.env`.

## Audit du 2026-10-10 (après la première revue) : corrigé
- [x] `guidecheck -plan` : niveau minimum (était +1), prérequis (rendus ou planifiés avant), quêtes de classe sans `only`, `only <race>` seul, filtre de danger `-maxabove 2`, types manuels (élite/donjon/raid/escorte/PvP). Tests unitaires (`plan_test.go`) ; `zygor2route` testé et corrigé (`only` en tête d'étape).
- [x] `grind.sh` : champs à zéro omis (XP 0 après un niveau) et session fermée.
- [x] `combat` : un échec de sort n'est attribué qu'à notre lancer (compteur + sort) ; buffs notés seulement s'ils ont réussi, oubliés à la mort.
- [x] `auto` : entraîneur à 400 yards max, monstres élites ou trop hauts refusés, kills conservés d'un point d'apparition à l'autre, talents par lots, visite au marchand qui répare (durabilité < 60 %).
- [x] Réparation (`town.repair`) et durabilité (`state`) ; `world.find_npc` ignore les PNJ « [DND] » (ils masquaient les vrais marchands : cause probable de l'échec de vente inexpliqué).
- [x] Revente : les réactifs sont gardés.
- [ ] Reste : danger des GROUPES de monstres (la règle de niveau laisse passer les quêtes de la mine : 60, 47, 87) → pathfinding/aggro (Opus).

## Revente et inventaire (fait le 2026-10-10)
- [x] `town.sell_junk` vend aussi les objets blancs inutiles selon des règles (voir MODULES.md : équipement inutilisable ou sans gain, marchandises, nourriture en trop, consommables obsolètes à 10 niveaux d'écart). Testé en jeu : 12 piles, 134 pc, 12 emplacements libres.
- [x] `dry_run` + `show_kept` : détail (prix, raison) avant de vendre. Distance de vente 5 yards (le serveur refuse au-delà de 5,5 ; l'ancien seuil de 8 yards faisait échouer des ventes en silence). `auto.ensureSpace` suit le marchand jusqu'à 5 yards.
- [x] Les objets vendus (liste de rachat) ne sont plus comptés dans `state.snapshot`.
- [ ] Le prix attendu (156 pc) dépasse le gain mesuré (134 pc) : 22 pc d'écart non expliqués (prix de vente ajusté par le serveur ?). Le 2e test (44 pc attendus, 44 reçus) était exact. Comparer par objet si ça se reproduit.
- [ ] Ne ramasser que ce qui vaut quelque chose quand les sacs sont presque pleins (ignorer les gris de très faible valeur, loot sélectif dans `loot.Take`).
- [x] Réparation : `town.repair` + passage automatique quand la durabilité < 60 %. Rachat : non nécessaire.
- [ ] Équiper automatiquement les sacs trouvés/achetés (aujourd'hui gardés, jamais équipés) : plus d'emplacements, donc moins de ventes.
- [ ] Manger/boire : `combat.rest` ne mange ni ne boit ; la nourriture gardée ne sert à rien tant que ce n'est pas fait (gain de temps de repos).
- [ ] Argent pour l'entraînement : `progress.train` achète du moins cher au plus cher ; si l'argent manque, noter le déficit dans le statut.

## Déplacement (idées des bots du serveur, 2026-10-10)
- [x] Lissage du chemin collé à la surface (port de `findSmoothPath`/`moveAlongSurface`) : fait, testé hors ligne et en jeu (auberge de Goldshire, poursuite en combat). A valider visuellement lors du « gros test » (escaliers, ponts, virages serrés).
- [x] Hauteurs vmaps (port de StaticMapTree/BIH/WorldModel du serveur, `internal/vmap`) : sols des bâtiments à 0,1-0,2 yard des créatures de référence (auberge sur 2 étages, abbaye, cathédrale), choix terrain/vmap comme `Map::GetHeight`. Testé en jeu (étage de l'auberge par l'escalier). Ligne de vue : `move.los`, testée (plafond bloque).
- [x] Ligne de vue en combat : le prêtre se rapproche (mi-distance) quand la cible n'est pas visible, le guerrier ne charge que s'il voit la cible ; l'ancien repli « mêlée après 3 échecs » reste en dernier recours. Testé en terrain dégagé (pas de régression) ; à observer près des bâtiments pendant le gros test (log « no line of sight, moving closer »).
- [ ] Sorts interrompus (code 40) : ne pas se déplacer pendant une incantation quand la cible bouge un peu.
- [ ] Idées mod-playerbots (src/Ai, mouvements) : suivre une cible (distance/angle), vérifier qu'une destination est atteignable avant d'y aller, se débloquer.
- [ ] Évitement des monstres hostiles dans l'A* (coût autour des monstres). Opus.

## Nage, eau et quêtes d'eau (question utilisateur, 2026-10-11)
- [ ] `move` ne sait PAS nager : le navmesh est filtré sur le sol (GroundOnly), pas de drapeau de nage (MOVEMENTFLAG_SWIMMING 0x200000), pas de vitesse de nage, pas de gestion du souffle (SMSG_START_MIRROR_TIMER type 1, noyade après ~60 s sous l'eau, fatigue). Une destination dans l'eau est refusée (« not on the navmesh ») ; `nav.GroundPartial` (opt-in) mène au point marchable le plus proche mais n'est utilisé nulle part. A faire pour nager : (1) polygones NAV_WATER (flag 0x08) et niveau d'eau (liquide des `.map` et des vmaps) ; (2) paquets de mouvement de nage avec tangage ; (3) écouter le minuteur de souffle et remonter à la surface ; (4) ne nager que si un chemin marchable n'existe pas. Opus.
- [ ] Les quêtes de murlocs du lac (112 Collecting Kelp, 114 The Escape) : les monstres sont sur la rive, en groupes de 5-6 (niveau 6-7) ; Claudio y est mort 2 fois (niveau 8, 193 PV) sans nager : combat contre le groupe. A faire : évitement/aggro de groupe (voir « Sécurité ») avant de refaire ces quêtes.
- [x] `hunt` essaie le point d'apparition suivant quand l'un est hors navmesh ; chemin partiel désactivé par défaut (seul `GroundPartial` l'active).

## Test du guerrier Claudio (2026-10-10) : ouvert
- [x] « Fight timeout » et mort : analysée (personnage laissé sans job parmi des loups après l'arrêt de `auto.grind` ; coups refusés par le serveur) ; corrigé : garde d'`auto` (`combat.defend`), `auto.grind` tolérant, refus de mêlée écoutés (portée/orientation), abandon d'une cible sans dégâts après 25 s. Validé 15 min sans mort.
- [x] Parry : en fait utile (permet de parer), pas un défaut. (Si un sort doit être exclu un jour : `training.skip` du fichier de classe.)
- [~] Overpower seulement après une esquive (journal de combat) et recharge de Charge suivie entre combats : fait. Reste : Victory Rush tenté hors fenêtre (code 67) — suivre l'aura « Victorious » ; Charge refusée pour ligne de vue (47) malgré `move.los` (doodads M2 ou relief).
- [ ] Soins du guerrier : aucun (pas de nourriture ni de Premiers soins) ; le repos à 75 % réduit les morts mais ralentit. A faire : acheter/manger de la nourriture, Premiers soins (compétence + bandages).
- [ ] Portée visible : le serveur n'envoie que les monstres proches (~100 yards), donc les chasses « rayon 150 » sur des créatures éloignées échouent vite ; `auto.grind` saute, mais `world.spawns` pourrait guider `hunt` vers les points d'apparition éloignés (déjà fait dans `hunt` ; à vérifier pour `grind`).
- [x] Danger par classe, rotation (Battle Shout/Charge/Heroic Strike), boucle de `grind.sh` (voir `journal/characters/Claudio.md`).

## Sécurité du personnage (morts répétées constatées le 2026-10-10)
- [x] Téléportations jamais confirmées (MSG_MOVE_TELEPORT_ACK / WORLDPORT_ACK) : après chaque mort le serveur ignorait nos mouvements. Corrigé dans `state` le 2026-10-11 (aussi vitesses forcées et enracinement). Cause probable des « attacked while idle: defending » répétés toutes les 4 min pendant des heures à la lac : à revoir si ça réapparaît.
- [ ] Pathfinding qui évite les monstres hostiles (idée utilisateur) : voir « Déplacement ». Cause des 3 morts dans la mine de Fargodeep.
- [~] `auto.collect` / `auto.hunt` : monstres élites ou de plus de 2 niveaux au-dessus refusés (fait) ; reste le danger des groupes (quêtes 60, 47, 87 de la mine).
- [x] Mort pendant une marche : `move` s'arrête désormais (« character died »).
- [x] `auto.recover` : résurrection chez l'esprit guérisseur du cimetière s'il est à moins de 150 yards (le corps est souvent dans le camp), niveaux 1-10 seulement (malus de résurrection dès le niveau 11, `Death.SicknessLevel`) ; sinon course au corps avec attente jusqu'à 4 minutes (délai de récupération croissant). Testé en jeu au niveau 5.
- [ ] Dès le niveau 11 : le corps reste souvent dans le camp qui nous a tués. A faire : avant de réclamer, attendre/éviter les monstres proches du corps (vue du fantôme), ou ne pas partir au corps tant que les monstres n'ont pas quitté la zone ; envisager de fuir plus tôt (voir pathfinding qui évite les monstres). Niveau requis : Opus.
- [~] Après une résurrection, se reposer avant toute autre action (sinon mort immédiate à 50 % de PV) : l'esprit guérisseur redonne ~75 % de PV loin du danger ; ajouter un repos automatique à la fin de `auto.recover`.
- [x] `scripts/grind.sh` : chasse en boucle jusqu'au niveau voulu avec résurrection et arrêt après 3 morts (sans mort pendant le test, 580 s, +510 XP).

## Quêtes spéciales à gérer dans `auto`
- [ ] Quêtes de soin / de buff sur des PNJ (beaucoup de quêtes de prêtre : ex. 5624 « Garments of the Light » : soigner Guard Roberts avec Soins inférieurs rang 2, puis Robustesse, etc.). Aujourd'hui `auto` les traite comme un objectif « kill » et risquerait d'attaquer le PNJ allié : elles sont sautées à la main. A faire : détecter les objectifs dont le PNJ est allié (faction non hostile / pas de statut « attaquable »), ne jamais appeler `combat.kill` dessus, et ajouter une action `quest.cast {spell, npc_entry}` (lancer un sort sur un PNJ, avec la liste des sorts demandés lue dans le texte de la quête ou saisie à la main). Niveau requis : Opus (protocole + logique de ciblage).
- [x] Quêtes « explorer une zone » : objectif `explore` (déclencheurs `areatrigger_involvedrelation`), testé en jeu sur 62 et 76 (2026-10-11).

## Sélection des quêtes de la route (décisions utilisateur, 2026-10-11)
- [x] Quêtes déjà faites : sautées.
- [x] Quêtes interdites à la race/classe : sautées.
- [ ] L'ORDRE DES ÉTAPES ZYGOR EST À RESPECTER : quand la prochaine quête demande un niveau plus haut (niveau minimum, ou monstres hors de la marge de danger), `auto` doit grinder près de là jusqu'au niveau voulu au lieu de passer à la suite. A changer dans `guidecheck -plan` / le futur plan intégré d'`auto`.
- [~] Quêtes qu'`auto` ne sait pas faire : liste `journal/quests_manual_human_warrior.md` (`guidecheck -manual`), 281 sur 1925 après le travail du 2026-10-11 (356 avant). Décisions de l'utilisateur par catégorie :
  - [x] Crédit de kill par d'autres créatures (KillCredit1/2) : fait (`credit_from`, `auto` chasse la plus proche). Non testé en jeu (quêtes 58+). Attention : certaines de ces créatures sont alliées (prisonniers à libérer, villageois à sauver : 11255, 12237) : ce sont alors des événements, pas des kills ; à traiter quand on y sera.
  - [ ] Crédit de kill par script (118 restantes) : À FAIRE AU CAS PAR CAS quand le personnage arrive à la quête en jeu.
  - [x] Zones à explorer (24) : fait et testé.
  - [ ] Escortes, scripts, duels (88, ex. 1640 Beat Bartleby, 945 Therylune's Escape) : À FAIRE quand on en aura besoin en jeu (module escorte : suivre le PNJ, défendre, ne pas le dépasser).
  - [ ] Objets qu'aucun monstre ne lâche (53 : achetés, fabriqués, donnés par un script, ou objet à utiliser sur une cible) : backlog, cas par cas.
  - [x] Objet de quête à utiliser sur une cible (45 des 48) : fait (`use_item`, `quest.use_item`). CMSG_USE_ITEM validé avec la pierre de foyer ; sur une cible, pas encore testé (première quête : niveau 14+). Restent 3 sans objet ni cible connus (ex. 2118 Plagued Lands : capturer un ours par script). Limite : les quêtes où il faut d'abord affaiblir/tuer la cible puis utiliser l'objet ne sont pas gérées.
  - [x] Groupe, élite, donjon, JcJ : abandon (décision utilisateur).

## Leveling automatique `level.sh` (2026-10-11)
- [x] « Continue le leveling » : `scripts/level.sh` (route Zygor dans l'ordre + grind d'un niveau quand la quête suivante est trop haute). Testé : Claudio 9 → 10.
- [ ] Les reports (« too dangerous », mort) et les SKIP ne sont gardés que pendant une exécution : relancer retente tout de suite (ex. 47 dans la mine juste après une mort). Les garder dans `logs/level_<Nom>.state` si ça gêne.
- [ ] Quêtes de classe (1638 au niveau 10, 1718 au niveau 30) : pas dans la route Zygor convertie ; à ajouter au plan selon `journal/classes/<classe>.yaml`.
- [ ] Mine de Fargodeep (47, 60) : mort en y marchant (groupes de kobolds) ; dépend du pull d'un seul monstre / évitement.
- [ ] Voyages (bateau, tram, vols, autre continent) : `level.sh` ne sait pas encore changer de zone autrement qu'à pied.

## Rotation et combat : sources étudiées le 2026-10-11 (à faire, Opus)
Sources :
- Ovale Spell Priority r261 (addon 3.3, Interface 30300, zip fourni par l'utilisateur, gardé en local dans `assets/sources/ovale/`) ;
- Toutes les classes et spécialisations sont résumées dans `journal/rotations/` (une fiche par classe + `auto/` régénéré par `scripts/rotation_sources.py` depuis `assets/sources/`) ;
- mod-playerbots (src/Ai/Class/Warrior) ;
- WeakAuras Mirash « Warrior combat priority » (wago X0AepQIWE) ;
- WeakAuras Afenar WotLK Guerrier (Tsttn04EF) et Prêtre (9W5YhSqB_).

Ordre proposé :
1. [ ] Aura state (UNIT_FIELD_AURASTATE) et auras (SMSG_AURA_UPDATE / _ALL : buffs, procs, DoT avec durée restante, « les miens »), pour le personnage et la cible. Conditions des sorts lues dans Spell.dbc : CasterAuraState (champ 20 ; Victory Rush = 10), TargetAuraState (champ 21 ; Execute = 2 « moins de 20 % »), Revenge = 1. Règle l'erreur 67 de Victory Rush (aura « Victorious » 32216).
2. [ ] « Peut lancer » générique : sort connu, recharge, ressource, aura state, procs (comme `usable=1` d'Ovale et CAN_CAST de playerbots).
3. [ ] Rotation en TABLE DE PRIORITÉS par classe (modèle Ovale : la première ligne dont les conditions passent et dont le sort est lançable gagne), dans `journal/classes/<classe>.yaml`. Conditions utiles d'Ovale :
   - BuffExpires/Present(n s, stacks), TargetDebuffExpires(mine) ;
   - TargetLifePercent, Mana/rage, Stance, TalentPoints ;
   - « 1s before Spell(X) » (sort presque prêt) ;
   - TargetDeadIn (temps avant la mort de la cible, estimé par la baisse de ses PV : pas de DoT/Rend sur une cible qui meurt dans moins de 6-8 s, mieux que notre « Rend si cible > 50 % »).
4. [ ] Guerrier Arms (Ovale, posture de combat), dans l'ordre :
   - Overpower si Taste for Blood expire dans moins de 1,5 s et que Rend manque ;
   - Rend s'il manque et que la cible vit encore plus de 8 s ;
   - Overpower sur esquive, ou si Taste for Blood expire dans moins de 4,5 s ;
   - Mortal Strike si la cible a plus de 20 % de PV ;
   - Execute sur Sudden Death ou si la cible a moins de 20 % de PV ;
   - Overpower, puis Victory Rush ;
   - Slam si Improved Slam.
   Hors recharge globale : Heroic Strike au-delà de 94 rage en posture de combat (66 dans les autres) ; Cleave au-delà de 50 rage si plusieurs cibles. Cris rafraîchis à moins de 3 s ; Commanding Shout si un Battle Shout ou une Bénédiction de puissance est déjà là. Seuils du WeakAura Mirash : Slam si Mortal Strike n'est pas prêt avant 1,6 s et rage ≥ 45 ; plusieurs cibles = 4 ennemis ou plus à 8 yards (Sweeping Strikes, Bladestorm, Thunder Clap).
5. [ ] Prêtre Ombre (Ovale), dans l'ordre :
   - hors combat : Forme d'ombre, Feu intérieur (si moins de 400 s), Étreinte vampirique ;
   - en combat :
     - Attaque mentale si Focalisation intérieure est active ;
     - Mot de l'ombre : Douleur si Shadow Weaving a 5 charges, qu'il manque et que la cible vit plus de 6 s ;
     - Toucher vampirique s'il expire dans moins de 1 s (temps d'incantation compris) et que la cible vit plus de 8 s ;
     - Attaque mentale ;
     - Peste dévorante si elle manque et que la cible vit plus de 8 s ;
     - Fouet mental en remplissage (canalisation interruptible).
   - Mana : Ombrefiel sous 4000 de mana, puis Dispersion.
   Afenar (leveling) : DoT rafraîchis à moins de 3 s (aujourd'hui Mot de l'ombre : Douleur toutes les 18 s à l'aveugle) ; Spirit Tap / Improved Spirit Tap après un kill = sauter le repos ; Âme affaiblie lue au lieu de supposer 15 s.
6. [ ] Défensifs, seuils de playerbots : PV critiques 25 %, bas 45 %, moyens 65 %, presque pleins 85 %.
   - Guerrier : Intimidating Shout à PV critiques, Enraged Regeneration à PV moyens, Retaliation, Berserker Rage contre la peur, Bloodrage si PV ≥ 65 % et rage < 20.
   - Prêtre : Psychic Scream face à plusieurs attaquants, Desperate Prayer, Fade.
7. [ ] Pull à distance (lien avec « Pull d'un seul monstre ») : tir ou lancer selon l'arme à distance équipée (playerbots), Heroic Throw au niveau 80.

## Points à réfléchir (utilisateur, 2026-10-11, redemandés le même jour)
- [ ] Implémenter la nage (voir « Nage, eau et quêtes d'eau »).
- [ ] Pull d'un seul monstre : tirer (body pull : s'approcher juste à portée d'aggro, ou attaque à distance — tir/lancer pour le guerrier) puis reculer pour combattre loin du groupe, au lieu de courir sur la cible et d'aggro tout le camp. Demande : rayon d'aggro (niveau, ~20 yards), position des voisins, point de repli dégagé. Lié à l'évitement des monstres dans l'A*.

## Leveling guidé par des guides extraits (décidé le 2026-10-10)
Ordre des travaux :
0. [x] SOURCE TROUVÉE : guides Zygor (1-80, Alliance et Horde, avec ids) lus par `cmd/zygor2route` + `guidecheck -plan`. L'extraction Claude Desktop est en pause (utile surtout pour les guides de classe : talents/rotation).
1. [ ] (en pause) Extraction par Claude Desktop (navigateur) dans `journal/guides/` : routes Alliance 1-30 (races de départ puis Elwynn 10-13, Westfall 13-19, Redridge 20-24, Duskwood 25-29), puis le reste Alliance et Horde 1-80, puis un guide par classe/spécialisation (`class_<classe>_<spec>.json` : ordre des talents, rotation, poids de stats, quêtes de classe).
2. [x] `cmd/guidecheck` : validation contre la base + liste pour `auto.run` (voir MODULES.md).
3. [ ] Une fois les fichiers reçus : lancer guidecheck, corriger/signaler les écarts, présenter à l'utilisateur la route retenue AVANT de jouer ; marquer les étapes manuelles (voyages, tram, vols, quêtes de soin/exploration/groupe).
4. [~] `auto` piloté par la route : `guidecheck -plan` donne la liste, `auto.run` l'exécute (testé : chaîne Maclure, niveau 5 → 6). A faire : plan intégré dans `auto` (fichier de route + niveau courant) au lieu de listes à la main ; sauter les quêtes de niveau trop haut ; gérer voyages (tram Stormwind-Ironforge, vols).
5. [~] Données de classe (`journal/classes/*.yaml`, validées par `cmd/classcheck` et le test de `classdata`) : warrior.yaml reçu le 2026-10-10 (0 problème) ; priest.yaml créé depuis l'ancien `priest.Build`. FAIT : `progress.talents` piloté par le fichier de la classe, profils de stats par classe, rotation guerrier de base. A faire : respec (CMSG_TALENT_WIPE_CONFIRM) pour suivre exactement les `removed_at` (aujourd'hui sautés) ; rotation par classe pour les autres classes (code, Opus).
5b. [ ] Guerrier, à valider en jeu (test du 2026-10-10) : postures (Défensive/Berserker), Premiers soins, multi-cibles (Sweeping Strikes, Bladestorm), Overpower sur esquive réelle (journal de combat SMSG_ATTACKERSTATEUPDATE au lieu d'essais toutes les 2 s).
5c. [ ] Classes autres que prêtre et guerrier : profils de stats approximatifs, aucune rotation.

## A demander à l'utilisateur quand le personnage atteint le palier
Icy Veins bloque ma lecture (403) : à chaque palier ci-dessous, prévenir l'utilisateur en une ligne AVANT d'avancer, pour qu'il colle le texte du guide complet (ou un build complet) dans la conversation.
- [ ] Niveau 9 (avant le niveau 10) : ordre des talents niveau par niveau et glyphes. Le build actuel vient de guides tiers (Highground Gaming) ; le comparer au guide Icy Veins complet.
- [ ] Niveau 19-20 : rotation d'ombre (Dévorer, Fouet mental) et équipement recommandé ; sorts à apprendre aux niveaux 20-30.
- [ ] Niveau 39-40 : passage en Forme d'ombre (bloque les sorts du Sacré) : rotation complète et gestion de la survie.
- [ ] Niveau 49-50 : Toucher vampirique et rotation multi-cibles.
- [ ] Niveau 59-60 : le guide d'Icy Veins demande de réinitialiser les talents au niveau 60 (Ombre puis Discipline) : fournir le texte exact de cette partie.
- [ ] Dès qu'on change de zone (Elwynn Forest, puis Westfall...) : parcours de quêtes par niveau recommandé par le guide.
- [ ] Autre classe ou autre spécialisation : demander le guide de leveling complet correspondant.

## Prochaines étapes suggérées (2026-10-10)
- Reprendre le test guerrier (Claudio, niveau 6 → 10) : d'abord le « fight timeout » et la mort inexpliquée (BACKLOG « Test du guerrier »), puis vérifier les talents automatiques au niveau 10.
- Gros test visuel du déplacement : escaliers, ponts, intérieurs, virages serrés, et le recul vers la ligne de vue en combat près des bâtiments.
- Claudia : niveaux 6 → 10 (quêtes du plan sans la mine, chasse), puis vérifier les talents automatiques au niveau 10.
- Évitement des monstres hostiles dans l'A* (Opus), puis quêtes de soin / buff sur PNJ.
