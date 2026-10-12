# BACKLOG (remarques utilisateur + problèmes connus)

Légende : [ ] à faire, [~] partiel, [x] fait.

## Remarques de l'utilisateur
- [ ] À vérifier en jeu : talents au niveau 10 (SMSG_TALENTS_INFO, CMSG_LEARN_TALENT), Bouclier/Rénovation au niveau 6-8, baguette (Tir) une fois une baguette équipée.
- [ ] Rotation d'ombre (guides de leveling) : le build prend Mind Flay (20), Shadowform (40), Vampiric Touch (50) mais `combat` ne les lance pas. A ajouter avec les rangs vérifiés par test : Dévorer (Devouring Plague, niveau 20), Attaque mentale puis Fouet mental, Étreinte vampirique, Forme d'ombre (bloque les sorts du Sacré : Bouclier, Soins, Châtiment ; donc à gérer ensemble), Toucher vampirique. Niveau requis : Opus pour la logique de combat.
- [ ] Baguette : les guides la jugent très importante dès le niveau 5 (Tir, sans mana). `progress.gear` la privilégie (poids 2,0), `combat` la utilise une fois équipée ; il faut en obtenir une (butin, récompense de quête ou achat).
- [ ] Maître d'armes (dagues pour le prêtre) : non géré ; `progress.gear` ignore les armes sans compétence.
- [~] Saccades/téléportations sur escaliers et terrain accidenté (remarque utilisateur) : chemin lissé sur la surface du navmesh (2 yards), sol comme le serveur (terrain ou vmaps), heartbeat 250 ms. A revérifier visuellement (gros test).
- [~] Hauteur : le personnage flottait ou s'enfonçait. Corrigé : hauteur du sol calculée comme le serveur (vmaps : 0,1-0,2 yard des références dans l'auberge, l'abbaye, la cathédrale). A revérifier visuellement.
- [~] Le personnage passait sous les ponts / à travers des barrières. Corrigé : chemin qui glisse le long des polygones marchables (plus de coupe de coin) et ponts/bâtiments lus dans les vmaps. Les barrières absentes du navmesh restent possibles : à revérifier visuellement.

## Problèmes connus (moi)
- [~] Éviter de mourir : garde d'`auto` entre deux jobs, riposte pendant les marches, nourriture, cibles trop entourées évitées, résurrection au point sûr (faits) ; reste : fuir, potions, pull qu'on est susceptible de gagner.
- [ ] `session.logout` est refusé en combat (reason 1) : attendre la fin du combat.
- [~] Durée de démarrage : `char play --module-timeout 8m` nécessaire (9 images Docker) ; `scripts/start.sh` attend à notre place.
- [ ] Compte MySQL en lecture seule pour `world` : créer `agent_ro` (SELECT sur acore_world) et remplir `modules/world/.env`.

## Audit du 2026-10-10 (après la première revue) : corrigé
- [ ] Reste : danger des GROUPES de monstres (la règle de niveau laisse passer les quêtes de la mine : 60, 47, 87) → pathfinding/aggro (Opus).

## Revente et inventaire (fait le 2026-10-10)
- [ ] Le prix attendu (156 pc) dépasse le gain mesuré (134 pc) : 22 pc d'écart non expliqués (prix de vente ajusté par le serveur ?). Le 2e test (44 pc attendus, 44 reçus) était exact. Comparer par objet si ça se reproduit.
- [ ] Ne ramasser que ce qui vaut quelque chose quand les sacs sont presque pleins (ignorer les gris de très faible valeur, loot sélectif dans `loot.Take`).
- [ ] Équiper automatiquement les sacs trouvés/achetés (aujourd'hui gardés, jamais équipés) : plus d'emplacements, donc moins de ventes.
- [ ] Manger/boire : `combat.rest` ne mange ni ne boit ; la nourriture gardée ne sert à rien tant que ce n'est pas fait (gain de temps de repos).
- [ ] Argent pour l'entraînement : `progress.train` achète du moins cher au plus cher ; si l'argent manque, noter le déficit dans le statut.

## Déplacement (idées des bots du serveur, 2026-10-10)
- [ ] Sorts interrompus (code 40) : ne pas se déplacer pendant une incantation quand la cible bouge un peu.
- [ ] Idées mod-playerbots (src/Ai, mouvements) : suivre une cible (distance/angle), vérifier qu'une destination est atteignable avant d'y aller, se débloquer.
- [ ] Évitement des monstres hostiles dans l'A* (coût autour des monstres). Opus.

## Nage, eau et quêtes d'eau (question utilisateur, 2026-10-11)
- [ ] `move` ne sait PAS nager : le navmesh est filtré sur le sol (GroundOnly), pas de drapeau de nage (MOVEMENTFLAG_SWIMMING 0x200000), pas de vitesse de nage, pas de gestion du souffle (SMSG_START_MIRROR_TIMER type 1, noyade après ~60 s sous l'eau, fatigue). Une destination dans l'eau est refusée (« not on the navmesh ») ; `nav.GroundPartial` (opt-in) mène au point marchable le plus proche mais n'est utilisé nulle part. A faire pour nager : (1) polygones NAV_WATER (flag 0x08) et niveau d'eau (liquide des `.map` et des vmaps) ; (2) paquets de mouvement de nage avec tangage ; (3) écouter le minuteur de souffle et remonter à la surface ; (4) ne nager que si un chemin marchable n'existe pas. Opus.
- [ ] Ce que fait mod-playerbots (lu le 2026-10-11, `assets/sources/playerbots_class` + src/Ai/Base) : les bots tournent DANS le serveur et utilisent ses données de liquide (`GetLiquidData` : dans l'eau / sous l'eau). Dans l'eau : drapeau de nage activé ou désactivé selon ce statut, et PAS de pathfinding (ligne droite) ; marche sur l'eau si une aura le permet. Ils ÉVITENT l'eau : déplacements aléatoires et « go » refusent une destination dans l'eau. Respiration : rien de spécial (pas de gestion du souffle), seulement les buffs de classe quand on nage (Respiration interminable du démoniste, Respiration aquatique / Marche sur l'eau du chaman). À reprendre : (1) niveau d'eau lu dans les `.map` (couche liquide ; notre `terrain` ne lit que les hauteurs) ; (2) dans l'eau, ligne droite avec MOVEMENTFLAG_SWIMMING et la vitesse de nage ; (3) éviter les destinations dans l'eau sauf besoin ; (4) en plus d'eux : minuteur de souffle (SMSG_START_MIRROR_TIMER) pour remonter.
- [ ] Les quêtes de murlocs du lac (112 Collecting Kelp, 114 The Escape) : les monstres sont sur la rive, en groupes de 5-6 (niveau 6-7) ; Claudio y est mort 2 fois (niveau 8, 193 PV) sans nager : combat contre le groupe. A faire : évitement/aggro de groupe (voir « Sécurité ») avant de refaire ces quêtes.

## Test du guerrier Claudio (2026-10-10) : ouvert
- [~] Overpower après une esquive ou Taste for Blood, Victory Rush selon l'aura state (fait, 2026-10-11) ; recharge de Charge suivie entre combats. Reste : Charge refusée pour ligne de vue (47) malgré `move.los` (doodads M2 ou relief).
- [~] Soins du guerrier : nourriture achetée et mangée (fait) ; reste Premiers soins (compétence + bandages).
- [ ] Portée visible : le serveur n'envoie que les monstres proches (~100 yards), donc les chasses « rayon 150 » sur des créatures éloignées échouent vite ; `auto.grind` saute, mais `world.spawns` pourrait guider `hunt` vers les points d'apparition éloignés (déjà fait dans `hunt` ; à vérifier pour `grind`).

## Sécurité du personnage (morts répétées constatées le 2026-10-10)
- [ ] Pathfinding qui évite les monstres hostiles (idée utilisateur) : voir « Déplacement ». Cause des 3 morts dans la mine de Fargodeep.
- [~] `auto.collect` / `auto.hunt` : monstres élites ou de plus de 2 niveaux au-dessus refusés (fait) ; reste le danger des groupes (quêtes 60, 47, 87 de la mine).
- [~] (point sûr près du corps : fait, à valider sur une vraie mort) Dès le niveau 11 : le corps reste souvent dans le camp qui nous a tués. A faire : avant de réclamer, attendre/éviter les monstres proches du corps (vue du fantôme), ou ne pas partir au corps tant que les monstres n'ont pas quitté la zone ; envisager de fuir plus tôt (voir pathfinding qui évite les monstres). Niveau requis : Opus.
- [~] Après une résurrection, se reposer avant toute autre action (sinon mort immédiate à 50 % de PV) : l'esprit guérisseur redonne ~75 % de PV loin du danger ; ajouter un repos automatique à la fin de `auto.recover`.

## Quêtes spéciales à gérer dans `auto`
- [ ] Quêtes de soin / de buff sur des PNJ (beaucoup de quêtes de prêtre : ex. 5624 « Garments of the Light » : soigner Guard Roberts avec Soins inférieurs rang 2, puis Robustesse, etc.). Aujourd'hui `auto` les traite comme un objectif « kill » et risquerait d'attaquer le PNJ allié : elles sont sautées à la main. A faire : détecter les objectifs dont le PNJ est allié (faction non hostile / pas de statut « attaquable »), ne jamais appeler `combat.kill` dessus, et ajouter une action `quest.cast {spell, npc_entry}` (lancer un sort sur un PNJ, avec la liste des sorts demandés lue dans le texte de la quête ou saisie à la main). Niveau requis : Opus (protocole + logique de ciblage).

## Sélection des quêtes de la route (décisions utilisateur, 2026-10-11)
- [ ] L'ORDRE DES ÉTAPES ZYGOR EST À RESPECTER : quand la prochaine quête demande un niveau plus haut (niveau minimum, ou monstres hors de la marge de danger), `auto` doit grinder près de là jusqu'au niveau voulu au lieu de passer à la suite. A changer dans `guidecheck -plan` / le futur plan intégré d'`auto`.
- [~] Quêtes qu'`auto` ne sait pas faire : liste `journal/quests_manual_human_warrior.md` (`guidecheck -manual`), 281 sur 1925 après le travail du 2026-10-11 (356 avant). Décisions de l'utilisateur par catégorie :
  - [ ] Crédit de kill par script (118 restantes) : À FAIRE AU CAS PAR CAS quand le personnage arrive à la quête en jeu.
  - [ ] Escortes, scripts, duels (88, ex. 1640 Beat Bartleby, 945 Therylune's Escape) : À FAIRE quand on en aura besoin en jeu (module escorte : suivre le PNJ, défendre, ne pas le dépasser).
  - [ ] Objets qu'aucun monstre ne lâche (53 : achetés, fabriqués, donnés par un script, ou objet à utiliser sur une cible) : backlog, cas par cas.

## Leveling automatique `level.sh` (2026-10-11)
- [ ] Les reports (« too dangerous », mort) et les SKIP ne sont gardés que pendant une exécution : relancer retente tout de suite (ex. 47 dans la mine juste après une mort). Les garder dans `logs/level_<Nom>.state` si ça gêne.
- [ ] Quêtes de classe (1638 au niveau 10, 1718 au niveau 30) : pas dans la route Zygor convertie ; à ajouter au plan selon `journal/classes/<classe>.yaml`.
- [ ] Mine de Fargodeep (47, 60) : mort en y marchant (groupes de kobolds) ; dépend du pull d'un seul monstre / évitement.
- [ ] Voyages (bateau, tram, vols, autre continent) : `level.sh` ne sait pas encore changer de zone autrement qu'à pied.

## Revue de mod-playerbots (2026-10-11) : à développer
Détail et comparaison : `journal/playerbots_review.md`. Attention, une partie de leur aisance vient de
triches serveur (sorts, talents, équipement et consommables offerts, téléportations) que nous n'avons pas.
- [ ] Potions/pierre de soins à PV critiques ; racials par déclencheur.
- [~] FleeManager (point sûr) : `move.safe_point` fait et testé, utilisé pour la résurrection au corps (pas encore vu en jeu sur une vraie mort). Reste : recul après pull, fuite, distance des lanceurs.
- [~] Adds possibles + pull où l'on est susceptible de gagner (voir « Points à réfléchir »).
- [ ] Cible de grind : attaquants d'abord, même hauteur (± 5 yards), en vue ; en route, seulement ce qui sert aux quêtes ou nous attaque.
- [ ] `quest_poi` / `quest_poi_points` pour trouver la zone de chaque objectif et du rendu.
- [ ] Mêlée : se placer derrière la cible ; lanceurs : garder la distance.
- [ ] Voyages : vols, bateaux/zeppelins, tram, pierre de foyer ; monture (achat au niveau 20, usage sur les longs trajets).
- [ ] Journal de quêtes : jeter les grises/impossibles, garder des places.
- [ ] Blocage : suivi du progrès et récupération graduée sans téléport.

## Rotation et combat : sources étudiées le 2026-10-11 (à faire, Opus)
Sources :
- Ovale Spell Priority r261 (addon 3.3, Interface 30300, zip fourni par l'utilisateur, gardé en local dans `assets/sources/ovale/`) ;
- Toutes les classes et spécialisations sont résumées dans `journal/rotations/` (une fiche par classe + `auto/` régénéré par `scripts/rotation_sources.py` depuis `assets/sources/`) ;
- mod-playerbots (src/Ai/Class/Warrior) ;
- WeakAuras Mirash « Warrior combat priority » (wago X0AepQIWE) ;
- WeakAuras Afenar WotLK Guerrier (Tsttn04EF) et Prêtre (9W5YhSqB_).

Ordre proposé :
3. [ ] Rotation en TABLE DE PRIORITÉS par classe (modèle Ovale : la première ligne dont les conditions passent et dont le sort est lançable gagne), dans `journal/classes/<classe>.yaml`. Conditions utiles d'Ovale :
   - BuffExpires/Present(n s, stacks), TargetDebuffExpires(mine) ;
   - TargetLifePercent, Mana/rage, Stance, TalentPoints ;
   - « 1s before Spell(X) » (sort presque prêt) ;
   - TargetDeadIn (temps avant la mort de la cible, estimé par la baisse de ses PV : pas de DoT/Rend sur une cible qui meurt dans moins de 6-8 s, mieux que notre « Rend si cible > 50 % »).
4. [ ] Tables de priorités Guerrier (Armes) et Prêtre (Ombre) : priorités détaillées dans `journal/rotations/warrior.md` et `priest.md` (Ovale, Mirash, Afenar).
6. [ ] Défensifs, seuils de playerbots : PV critiques 25 %, bas 45 %, moyens 65 %, presque pleins 85 %.
   - Guerrier : Intimidating Shout à PV critiques, Enraged Regeneration à PV moyens, Retaliation, Berserker Rage contre la peur, Bloodrage si PV ≥ 65 % et rage < 20.
   - Prêtre : Psychic Scream face à plusieurs attaquants, Desperate Prayer, Fade.
7. [ ] Pull à distance (lien avec « Pull d'un seul monstre ») : tir ou lancer selon l'arme à distance équipée (playerbots), Heroic Throw au niveau 80.

## Points à réfléchir (utilisateur, 2026-10-11, redemandés le même jour)
- [ ] Implémenter la nage (voir « Nage, eau et quêtes d'eau »).
- [~] Pull où l'on est susceptible de GAGNER (reformulation utilisateur, 2026-10-11) : le critère est l'issue probable du combat (cible + adds, leurs niveaux, nos PV/ressource, nos sorts de zone et défensifs), pas « un seul monstre » ; isoler un monstre n'est qu'un moyen d'y arriver. FAIT = choix d'une cible dont la charge (cible + adds pondérés par niveau) tient dans la capacité, sinon on la laisse et `auto` va ailleurs. EXPÉRIMENTAL (`body_pull`, désactivé) : se placer à portée d'aggro de la seule cible puis reculer. Test : le bandit n'a pas réagi en 8 s, puis mort contre un camp de Defias. À faire :
  - avancer pas à pas (2 yards) vers la cible en vérifiant la marge avec les autres, au lieu d'attendre sur place ; vérifier la formule de portée (Creature::GetAggroRange + DetectionRange de creature_template ; ligne de vue) ;
  - pull à distance : acheter une arme de lancer (guerrier : Throw) ou utiliser l'arme à distance, tirer à 25-30 yards, reculer ;
  - groupes liés (creature_formations, SmartAI « call for help ») : ils viennent ensemble quelle que soit la distance → compter tout le groupe comme adds.
- [ ] Capacité qui évolue (demande utilisateur ; c'est l'estimation « susceptible de gagner ») : aujourd'hui 1 / 1,5 (niv. 20) / 2 (niv. 40). La faire dépendre des sorts de zone appris (Thunder Clap, Sweeping Strikes, Bladestorm, Cleave), de l'équipement (PV, armure) et surtout APPRENDRE des combats : PV perdus par monstre selon l'écart de niveau → augmenter la capacité si les combats à 2 finissent au-dessus de 50 % de PV, la baisser après une mort.

## Leveling guidé par des guides extraits (décidé le 2026-10-10)
Ordre des travaux :
0. [x] SOURCE TROUVÉE : guides Zygor (1-80, Alliance et Horde, avec ids) lus par `cmd/zygor2route` + `guidecheck -plan`. L'extraction Claude Desktop est en pause (utile surtout pour les guides de classe : talents/rotation).
1. [ ] (en pause) Extraction par Claude Desktop (navigateur) dans `journal/guides/` : routes Alliance 1-30 (races de départ puis Elwynn 10-13, Westfall 13-19, Redridge 20-24, Duskwood 25-29), puis le reste Alliance et Horde 1-80, puis un guide par classe/spécialisation (`class_<classe>_<spec>.json` : ordre des talents, rotation, poids de stats, quêtes de classe).
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
- [ ] Quête 62 au niveau 5 : mort pendant le nettoyage (kobolds 5-6) ; la porte de niveau 6 la reporte, à confirmer.

## Prochaines étapes suggérées (2026-10-11)
- FAIT : suivi des auras et aura state (« Rotation et combat » étape 1). FAIT aussi : « peut lancer ». SUITE : moteur de priorités en table, rotations Guerrier/Prêtre en tables ; tester le prêtre (Claudia) en jeu.
- Pull où l'on est susceptible de gagner (voir « Points à réfléchir ») et capacité qui apprend des combats.
- Continuer Claudio avec `level.sh` (11 → 20) : quête 87, quête de classe 1638 (posture défensive), Westfall à partir de 13-14 (voyage à pied).
- Gros test visuel du déplacement (escaliers, ponts, intérieurs).
- Claudia (prêtre 6 → 10) avec `level.sh` une fois la rotation prêtre refaite.
