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
- [~] Saccades/téléportations sur escaliers et terrain accidenté (remarque utilisateur) : lissage z, vitesse horizontale, heartbeat 250 ms ajoutés ; à revérifier visuellement, sinon envoyer plus de paquets ou ajuster les flags de mouvement.
- [~] Hauteur : le personnage flottait ou s'enfonçait. Corrigé en partie : pas de 1,5 yard, hauteur du terrain réel (`assets/maps`) uniquement si l'écart au navmesh < 0,8 yard. A revérifier visuellement.
- [~] Le personnage passait sous les ponts / à travers des barrières. Corrigé en partie : hauteurs prises dans les seuls polygones du couloir A* (plus de coupe entre niveaux superposés), test `TestPathStaysOnMesh` (0 % hors couloir). Les barrières absentes du navmesh ne peuvent pas être évitées par ce moyen : à revérifier visuellement, sinon ajouter les vmaps/doodads.
- [x] Le personnage était mort/fantôme (1 PV) : détection du fantôme via PLAYER_FLAGS, `auto.recover` (libérer l'âme, courir au cadavre, ressusciter), reprise automatique dans `auto.run`.
- [ ] Consigner ces remarques dans un fichier/issues git (ce fichier). Eventuellement créer des issues sur le fork.

## Problèmes connus (moi)
- [ ] Éviter de mourir : `combat` doit fuir/soigner/utiliser Bouclier, et ne pas laisser le personnage inactif au milieu des mobs après un arrêt.
- [ ] `session.logout` est refusé en combat (reason 1) : attendre la fin du combat.
- [x] Coffre « Milly's Harvest » : verrou « Open Kneeling » => sort 22810 lancé par `quest.use`.
- [~] Vente de camelote : fonctionne ; la marche a échoué une fois (cause : arrivée mesurée en 2D) ; vendre aussi les objets blancs inutiles.
- [~] Hostilité (factions) non résolue : `combat` ne chasse que des entrées explicites ; les agresseurs doivent être en combat (UNIT_FLAG_IN_COMBAT).
- [~] Durée de démarrage : `char play --module-timeout 8m` nécessaire (8 images Docker) ; `scripts/start.sh` attend à notre place.
- [x] `go vet` signalait des copies de messages protobuf (move/combat/auto) : package `jobs` + `proto.Clone`.
- [x] Compteurs d'objectifs de quête : 4 x uint16 sur 2 champs (était lu comme 4 octets d'un seul champ) ; indexés par emplacement RequiredNpcOrGo.
- [ ] Compte MySQL en lecture seule pour `world` : créer `agent_ro` (SELECT sur acore_world) et remplir `modules/world/.env`.

## Revente et inventaire (prochain chantier, session 2026-10-10 : sacs pleins = chasse bloquée)
Constat : `town.sell_junk` ne vend que les objets gris (4 objets = 4 cuivres) ; les sacs se remplissent d'objets blancs (nourriture en trop, armes/armures inutilisables : dague 24 pc, bouclier 15 pc, ceinture 6 pc, cape 7 pc...) et `combat` s'arrête sur « bags full ». Or il faut de l'argent pour les sorts (≈ 200 pc au niveau 6). Aucun appel pour vendre un objet précis.
- [ ] `town.sell {rules}` : vendre aussi les objets non gris inutiles. Règle par défaut : garder les objets de quête (`quest_template` Required/Source), la pierre de foyer (6948), les consommables utiles (eau/nourriture : garder 10 de chaque meilleur rang selon le niveau, vendre le reste), l'équipement porté et tout objet qui améliore l'équipement (`gear.Gain` > 0) ; vendre le reste, du plus cher au moins cher.
- [ ] Vendre aussi les objets d'armure/arme que la classe ne peut pas porter (`gear.Usable` faux) ET qui ne sont pas des matériaux de quête ; garder les armes pour lesquelles une compétence est apprenable bientôt (dague) seulement si leur score est meilleur.
- [ ] `town.sell_junk` : afficher le prix obtenu par objet (les 4 cuivres du test sont dans `copper`), proposer `dry_run` détaillé (liste des objets et du prix) pour valider la règle sans vendre.
- [ ] `auto.ensureSpace` : appeler cette vente (pas seulement le gris), marcher jusqu'au marchand le plus proche ATTEIGNABLE et vérifier qu'il est à moins de 8 yards ; l'échec « no vendor within 8 yards » du 2026-10-10 (après une marche « arrivée ») n'a pas été expliqué : tracer positions/PNJ visibles quand il se reproduit.
- [ ] Avant la chasse : `combat.kill` doit vendre ou refuser proprement au lieu de s'arrêter à 0 emplacement libre ; ne ramasser que ce qui vaut quelque chose (gris/blanc à faible valeur : ignorer si sacs < 3 libres).
- [ ] Rachat / réparation : non gérés (ajouter quand l'équipement a de la durabilité utile).
- [ ] Argent pour l'entraînement : `progress.train` achète du moins cher au plus cher ; si l'argent manque, noter le déficit dans le statut pour que `auto` vende ou chasse plus.
- Niveau requis : Sonnet suffit (logique + règles + SQL ; le paquet CMSG_SELL_ITEM existe déjà dans `town`). Opus seulement si on touche à la marche ou au combat.

## Sécurité du personnage (morts répétées constatées le 2026-10-10)
- [ ] Pathfinding qui évite les monstres hostiles (idée utilisateur) : aujourd'hui `move` va tout droit dans le couloir navmesh, y compris au milieu d'un camp (ex. kobolds de la mine de Fargodeep : 3 morts de suite). A faire : (1) coût supplémentaire dans l'A* autour des monstres connus (positions de spawn `creature` de la base + unités vivantes vues par `state`), rayon d'aggro ≈ 20 yards + 1 yard par niveau d'écart (hostiles seulement : niveau > niveau du joueur - 3 ou faction hostile) ; (2) option `avoid_hostiles` dans `move.goto` (activée par défaut dans `auto`) ; (3) si aucun chemin sûr, échouer avec la raison plutôt que traverser. Niveau requis : Opus (navmesh + hostilité).
- [ ] `auto.collect` / `auto.hunt` : choisir la source d'objet selon le niveau (ne pas chasser des mobs de niveau > joueur + 1 ; ex. quêtes 60 et 47 ne se font que sur des kobolds 5-8 dans la mine : à reporter au niveau 7+). Passer aussi un indicateur « trop dangereux » dans le statut.
- [x] Mort pendant une marche : `move` s'arrête désormais (« character died »).
- [x] `auto.recover` : résurrection chez l'esprit guérisseur du cimetière s'il est à moins de 150 yards (le corps est souvent dans le camp), niveaux 1-10 seulement (malus de résurrection dès le niveau 11, `Death.SicknessLevel`) ; sinon course au corps avec attente jusqu'à 4 minutes (délai de récupération croissant). Testé en jeu au niveau 5.
- [ ] Dès le niveau 11 : le corps reste souvent dans le camp qui nous a tués. A faire : avant de réclamer, attendre/éviter les monstres proches du corps (vue du fantôme), ou ne pas partir au corps tant que les monstres n'ont pas quitté la zone ; envisager de fuir plus tôt (voir pathfinding qui évite les monstres). Niveau requis : Opus.
- [~] Après une résurrection, se reposer avant toute autre action (sinon mort immédiate à 50 % de PV) : l'esprit guérisseur redonne ~75 % de PV loin du danger ; ajouter un repos automatique à la fin de `auto.recover`.
- [x] `scripts/grind.sh` : chasse en boucle jusqu'au niveau voulu avec résurrection et arrêt après 3 morts (sans mort pendant le test, 580 s, +510 XP).

## Quêtes spéciales à gérer dans `auto`
- [ ] Quêtes de soin / de buff sur des PNJ (beaucoup de quêtes de prêtre : ex. 5624 « Garments of the Light » : soigner Guard Roberts avec Soins inférieurs rang 2, puis Robustesse, etc.). Aujourd'hui `auto` les traite comme un objectif « kill » et risquerait d'attaquer le PNJ allié : elles sont sautées à la main. A faire : détecter les objectifs dont le PNJ est allié (faction non hostile / pas de statut « attaquable »), ne jamais appeler `combat.kill` dessus, et ajouter une action `quest.cast {spell, npc_entry}` (lancer un sort sur un PNJ, avec la liste des sorts demandés lue dans le texte de la quête ou saisie à la main). Niveau requis : Opus (protocole + logique de ciblage).
- [ ] Quêtes « explorer une zone » (ex. 62 The Fargodeep Mine) : non gérées par `auto` (déclencheur de zone).

## A demander à l'utilisateur quand le personnage atteint le palier
Icy Veins bloque ma lecture (403) : à chaque palier ci-dessous, prévenir l'utilisateur en une ligne AVANT d'avancer, pour qu'il colle le texte du guide complet (ou un build complet) dans la conversation.
- [ ] Niveau 9 (avant le niveau 10) : ordre des talents niveau par niveau et glyphes. Le build actuel vient de guides tiers (Highground Gaming) ; le comparer au guide Icy Veins complet.
- [ ] Niveau 19-20 : rotation d'ombre (Dévorer, Fouet mental) et équipement recommandé ; sorts à apprendre aux niveaux 20-30.
- [ ] Niveau 39-40 : passage en Forme d'ombre (bloque les sorts du Sacré) : rotation complète et gestion de la survie.
- [ ] Niveau 49-50 : Toucher vampirique et rotation multi-cibles.
- [ ] Niveau 59-60 : le guide d'Icy Veins demande de réinitialiser les talents au niveau 60 (Ombre puis Discipline) : fournir le texte exact de cette partie.
- [ ] Dès qu'on change de zone (Elwynn Forest, puis Westfall...) : parcours de quêtes par niveau recommandé par le guide.
- [ ] Autre classe ou autre spécialisation : demander le guide de leveling complet correspondant.

## Prochaines étapes suggérées
- Quêtes suivantes : Elwynn Forest (Goldshire, quête 54 puis zone suivante), Auberge (2158).
- Tests visuels (utilisateur) de la marche.
