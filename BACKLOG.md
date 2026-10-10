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
- [ ] Éviter de mourir : `combat` doit fuir/soigner/utiliser Bouclier, et ne pas laisser le personnage inactif au milieu des mobs après un arrêt.
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

## Test du guerrier Claudio (2026-10-10) : ouvert
- [ ] « Fight timeout » : `fightWarrior` expire après 120 s sans tuer (0 kill, PV 100 %) ; puis le personnage est trouvé mort à l'arrêt. Hypothèses : cible hors de portée de mêlée (le seuil `meleeRange+1` avec la distance 2D, une cible sur une pente ou derrière un mur), cible qui s'enfuit à faible vie, attaque automatique non engagée (CMSG_ATTACKSWING sans facing correct). A faire : journaliser l'état à chaque 20 s de combat (distance 3D, PV de la cible, rage, cible sélectionnée), abandonner et passer à une autre cible après 45 s sans dégât sur la cible, test hors ligne de `fightWarrior` avec un faux client.
- [ ] Entraînement inutile : Parry (passive) acheté (95 pc) ; `training.skip` du fichier warrior.yaml ne le liste pas. A faire : liste d'exclusion locale par classe (`journal/classes/<classe>.local.yaml` ou règle « passifs de défense ») et n'acheter que max_rank / train_asap_single_rank en premier quand l'argent manque.
- [ ] Charge « pas prête » (code 67) répétée et Overpower essayé toutes les 2 s : lire le journal de combat (SMSG_ATTACKERSTATEUPDATE) pour savoir quand Overpower est utilisable ; suivre le temps de recharge de Charge (15 s).
- [ ] Soins du guerrier : aucun (pas de nourriture ni de Premiers soins) ; le repos à 75 % réduit les morts mais ralentit. A faire : acheter/manger de la nourriture, Premiers soins (compétence + bandages).
- [ ] Portée visible : le serveur n'envoie que les monstres proches (~100 yards), donc les chasses « rayon 150 » sur des créatures éloignées échouent vite ; `auto.grind` saute, mais `world.spawns` pourrait guider `hunt` vers les points d'apparition éloignés (déjà fait dans `hunt` ; à vérifier pour `grind`).
- [x] Danger par classe, rotation (Battle Shout/Charge/Heroic Strike), boucle de `grind.sh` (voir `journal/characters/Claudio.md`).

## Sécurité du personnage (morts répétées constatées le 2026-10-10)
- [ ] Pathfinding qui évite les monstres hostiles (idée utilisateur) : voir « Déplacement ». Cause des 3 morts dans la mine de Fargodeep.
- [~] `auto.collect` / `auto.hunt` : monstres élites ou de plus de 2 niveaux au-dessus refusés (fait) ; reste le danger des groupes (quêtes 60, 47, 87 de la mine).
- [x] Mort pendant une marche : `move` s'arrête désormais (« character died »).
- [x] `auto.recover` : résurrection chez l'esprit guérisseur du cimetière s'il est à moins de 150 yards (le corps est souvent dans le camp), niveaux 1-10 seulement (malus de résurrection dès le niveau 11, `Death.SicknessLevel`) ; sinon course au corps avec attente jusqu'à 4 minutes (délai de récupération croissant). Testé en jeu au niveau 5.
- [ ] Dès le niveau 11 : le corps reste souvent dans le camp qui nous a tués. A faire : avant de réclamer, attendre/éviter les monstres proches du corps (vue du fantôme), ou ne pas partir au corps tant que les monstres n'ont pas quitté la zone ; envisager de fuir plus tôt (voir pathfinding qui évite les monstres). Niveau requis : Opus.
- [~] Après une résurrection, se reposer avant toute autre action (sinon mort immédiate à 50 % de PV) : l'esprit guérisseur redonne ~75 % de PV loin du danger ; ajouter un repos automatique à la fin de `auto.recover`.
- [x] `scripts/grind.sh` : chasse en boucle jusqu'au niveau voulu avec résurrection et arrêt après 3 morts (sans mort pendant le test, 580 s, +510 XP).

## Quêtes spéciales à gérer dans `auto`
- [ ] Quêtes de soin / de buff sur des PNJ (beaucoup de quêtes de prêtre : ex. 5624 « Garments of the Light » : soigner Guard Roberts avec Soins inférieurs rang 2, puis Robustesse, etc.). Aujourd'hui `auto` les traite comme un objectif « kill » et risquerait d'attaquer le PNJ allié : elles sont sautées à la main. A faire : détecter les objectifs dont le PNJ est allié (faction non hostile / pas de statut « attaquable »), ne jamais appeler `combat.kill` dessus, et ajouter une action `quest.cast {spell, npc_entry}` (lancer un sort sur un PNJ, avec la liste des sorts demandés lue dans le texte de la quête ou saisie à la main). Niveau requis : Opus (protocole + logique de ciblage).
- [ ] Quêtes « explorer une zone » (ex. 62 The Fargodeep Mine) : non gérées par `auto` (déclencheur de zone).

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
