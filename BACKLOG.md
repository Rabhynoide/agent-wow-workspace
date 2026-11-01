# BACKLOG (remarques utilisateur + problèmes connus)

Légende : [ ] à faire, [~] partiel, [x] fait.

## Objectif speedrun 1 → 80 (objectif final, fixé par l'utilisateur le 2026-10-14)
Monter un personnage de 1 à 80 le plus vite possible, en autonomie. Chaque point se mesure en minutes gagnées (protocole : NOTES « Protocole de mesure », 2 benchs 6 → 8 par variante). État au 2026-10-14 : 1 → 10 en 224 min (Benchtrhoxd), seul Elwynn est joué, rien au-delà du niveau 11.

### Étape 1 : Elwynn rapide (niveaux 1-10), mesurable dès maintenant
Variantes de prudence mesurées le 2026-10-14 (6 → 8 depuis `warrior_human_6`, commit 375f6c2, rapports dans journal/bench/) :
| Variante | Runs (min) | Morts | Verdict provisoire |
|---|---|---|---|
| A référence (+ contrôle 58,5) | 55,4 ; 61,8 | 0 | base ~58,5 min, écart ±3 |
| B `postpone_after_flights` 2 | 60,7 ; 57,4 | 0 | pas de gain |
| C `crowd_flee` 0 | 66,2 ; 60,6 | 0 | plutôt plus lent |
| D `chase_margin` 5 | 67,4 ; 55,0 | 3 | à écarter |
| E `avoid` false | 45,1 ; 43,5 ; 51,1 | 2 | **adopté le 2026-10-17** (moyenne 46,6, −12 min) : nouvelle référence |
Apprentissage des cas limites : 6 combats restés au lieu de fuir sur la variante A, 0 mort (journal/learn/flee.json).
Référence humaine (journal/sessions/20261014_humain_1-8.md) : **1 → 8 en ~60 min** contre ~117 pour le bot. Ce qui fait la différence, à reproduire dans l'ordre :
- [~] (2026-10-23 : le repos ne fait que 1-2,6 min par 6 → 8 dans la série 4f243f0, ~9 repas par run, déjà mangés ; variante RB55 `warrior_rest_below` 75 → 55 ADOPTÉE : 32,1 / 37,8 / 43,5 / 35,7 = 37,3 min contre 38,1, repos 0,85 min par run contre 2,1 ; reste à mesurer le repos à 90 % après une fuite) Pas de repos assis : reprendre le combat dès qu'on est hors combat avec assez de vie (régén hors combat rapide à bas niveau) ; repos/nourriture seulement sous un seuil bas. Mesurer `restBelow` 75 → 50 et le repos à 90 % après une fuite.
- [~] (2026-10-17, `combat/pullsize.go`, `pull_adapt` + `add_cost` dans tuning.json ; variante F mesurée : 42,9 / 49,9 min = E, sans gain, désactivée : le pull n'a presque jamais d'adds au repos à choisir, les combats à plusieurs viennent des monstres qui nous rejoignent. pull de groupe essayé puis retiré, voir plus bas) Mesure des engagements du 2026-10-13/14, niveaux 6-9, commencés ≥ 90 % : seul 12,9 s et 18 % de PV par kill (3 % ratés) ; à 2 monstres 9,8 s et 15 % (21 % ratés, surtout des fuites) ; à 3 : 7,9 s et 16 % (64 % ratés). Le modèle (+150 % par add) surestime les adds et fait fuir. Taille de pull adaptative : augmenter le nombre de monstres pris tant que les combats coûtent moins que prévu, redescendre après un combat difficile (aujourd'hui le bot fuit presque toujours à 3 ; le joueur combat à 2-4 la moitié du temps).
- [ ] (idée mise de côté le 2026-10-24, décision de l'utilisateur plus tard) Lancer les 4 runs d'une série en parallèle (3 comptes de plus, départs décalés ~10 min) : série ~1 h 10 au lieu de ~2 h 30. À faire si retenu : dossier de logs par session (les modules écrivent tous /logs/<module>.log), écritures de journal/learn sans conflit (ou apprentissage gelé puis rejoué sur les logs), snapshot chargé dans un autre compte, script de série parallèle ; puis une série de calibration sur le code actuel. Risques : runs qui se croisent dans la mine (monstres, objets de quête, tags), 4 cœurs pour 32 conteneurs de modules, mesures non comparables aux séries seules. Variante prudente : 2 en parallèle décalés de 20 min.
- [ ] (2026-10-24, idée) Raccourcis en sautant d'une falaise : le navmesh n'a aucun lien de descente, le bot fait toujours le tour ; il envoie ses mouvements avec un temps de chute 0 et jamais de MSG_MOVE_FALL_LAND. Dégâts de chute (Player::HandleFall, à vérifier) : rien jusqu'à ~13 yards, puis ~1,8 % de la vie max par yard (~50 % à 40 yards, mort vers 70). Étape 1, mesure : journaliser par trajet la longueur du chemin et la distance à vol d'oiseau (move), pour chiffrer les détours évitables. Étape 2 si ça vaut le coup : liens de descente à sens unique au bord du navmesh vers une zone praticable en contrebas (pas d'eau ni de vide), pris seulement s'ils raccourcissent ; 12 yards max au départ (zéro dégât, pas de suicide possible), plus haut plus tard seulement si les dégâts prévus ≤ ~20 % de la vie, hors combat et au-dessus de ~60 % ; chute déclarée honnêtement au serveur (temps de chute, atterrissage) ; pas de saut vers une zone avec des monstres. Gain faible en Elwynn (marche 18-25 % du temps), plus net en zones accidentées ; risque : acceptation du mouvement de chute par le serveur.
- [ ] (2026-10-24, validé, à faire après la série de mesure en cours) Comparer les combats du bot aux tiens sorts compris : (1) capturer les paquets d'un run du bot (PacketLogFile + `packetlog <perso> on`, redémarrage du worldserver entre deux séries, voir NOTES) et passer le même décodeur que la session humaine (scratchpad permob.py, à verser dans scripts/) ; (2) une ligne de log par sort lancé dans combat (sort, cible, attaquants, vie, rage, règle choisie), résumée dans le rapport de bench. Premiers constats sur ta session (L5-7) contre le log du bot : durées par cible proches ; contre 4+ ennemis tu lances Victory Rush ~2 fois par combat (bot ~0), Heroic Strike ~10 fois plus souvent, Rend sur plusieurs cibles, et tu ouvres par Charge dans 60 % des combats seul.
- [~] (2026-10-24, mesuré série M : 30 % des coups de dos à 2 attaquants, 46 % à 3+, évitement 17,7 % devant contre 11,1 % derrière ; gain si tout devant ~30 points de vie par run, négligeable : pas prioritaire) Garder les attaquants devant soi (mesure : « blows taken », « defense », rapport de bench) : un joueur frappé de dos ne peut ni esquiver, ni parer, ni bloquer, et le bot ne fait face qu'à sa cible. À faire après la mesure : si une part notable des coups vient de dos dans les combats à 2+, reculer de 3-4 yards quand un attaquant est derrière (les monstres suivent et se regroupent devant) ; garder ou équiper un bouclier tant qu'il n'y a pas d'arme à deux mains si le blocage compte.
- [~] (2026-10-24, `safe_rest_below` 40 mesuré en série SR : 7 déclenchements, jamais de point meilleur dans la mine car le chemin dégagé exigé ramène le dégagement à celui de notre position ; sans effet. À revoir : sans path_clear, ou vers l'entrée de la mine) Combats enchaînés à basse vie dans la mine (2026-10-23, série RB75 : repos sous 75 % mesuré 36,2 / 41,6 / 44,7 / 33,3 = 39,0 min, 4 morts, contre 39,5 et 5 au repos sous 55 % sur le même code, donc seuil remis à 55 : il ne joue pas). Les kobolds réapparus attaquent avant que le bot ne s'assoie ; il enchaîne des combats entamés à 10-49 % de vie et des cases apprises (1:0, 1:10, 3:0) restent. Pistes : sous ~40 % sans attaquant, sortir de la mine (point sûr) avant de se reposer ; après une résurrection dans la mine, se reposer à l'entrée.
- [~] **Traverser comme un joueur** (décision utilisateur, 2026-10-26 : améliorations du bot avant la route). Adoptés (séries du 2026-10-27) : contournement serré `nav.Bend` + marge d'errance (`avoid_wander`), résurrection chez l'ange gardien quand le corps reste entouré (< niveau 11), engagement en chemin (`engage_on_way`, non concluant : gardé), fuite sur la carte de danger (`internal/danger`). Rejeté : body pull (`body_pull` false, +8 min, 6 réussites sur 24 ; code gardé). À faire, plan carte de danger (décision utilisateur 2026-10-27) : repos sur la carte ; vérifier sur la carte avant une décision apprise de rester (la case 3:40 ignore les kobolds qui arrivent) ; coût des trajets et choix des pulls sur la carte (unifier avec les disques de `move/avoid.go`) ; couche apprise (morts, fuites) et calque du tableau de bord ; retirer l'A* pondéré si le log « bent path failed » reste rare. Règles du serveur lues dans AzerothCore (Creature.cpp, worldserver.conf) : un monstre qui nous poursuit abandonne (evade) quand il est à plus de `CreatureLeashRadius` 30 yards de son point de départ ET qu'il n'a pas été frappé/attaqué depuis `GetLeashTimer` = 11 s (+ (niveau min/10 − 2), 11 au moins) → traverser sans riposter marche après ~11 s de course ; entraide : `CallAssistance` à 10 yards du monstre engagé (`CreatureFamilyAssistanceRadius`), d'abord après 2 s puis toutes les 3 s → un body pull marche si on éloigne la cible de plus de 10 yards de ses voisins dans les 2 s (reculer dès qu'elle bouge). À faire ensuite : body pull (`combat.kill body_pull`, expérimental depuis le 2026-10-11, jamais activé : le bandit n'avait pas réagi en 8 s) avec ces règles, puis traversée sans riposte (marche vers un objet/PNJ proche, un seul poursuivant).
- [ ] Placement : avec 2+ attaquants, reculer pour les avoir tous devant (parade, blocage) ; reculer en se battant quand une patrouille approche plutôt que fuir.
- [ ] Mine de Fargodeep dès le niveau 5-6 par l'entrée basse ; ramasser le butin d'un monstre de quête (Goldtooth) avant tout autre combat.
- [~] (réglages `journal/learn/tuning.json` + apprentissage des cas limites, 2026-10-14 ; variantes B-E à mesurer, NOTES) Prudence qui coûte du temps : 19-33 fuites par run, presque toutes les quêtes 9-10 reportées. Essayer (une variante à la fois) : reporter une quête après 2 fuites au lieu d'1 ; règle des 3 attaquants assouplie quand ils sont plus faibles ou qu'une potion est prête ; `chaseMargin` 10 → 5.
- [~] (2026-10-23, variante MF mesurée : 44,8 / 37,5 / 42,8 / 33,0 = 39,5 min, 5 morts contre 37,3 et 1, non adoptée comme gain, code gardé (morts surtout hors des changements, voir audit Benchmxoefz) ; clé 4+ à part, cible abandonnée sans report de la quête, marche vers une cible en contrebas ; branche mine-fixes fusionnée) Stays appris mortels dans la mine (série 59a81a3 : 5 morts sur stays appris, cases 3:10 4/11, 3:0 2/7, 2:0 2/11, 1:0 2/9). La clé plafonne les attaquants frais à 3 : 3, 4 et 5 kobolds tombent dans la même case, alors que 4-5 meurent bien plus. Piste : séparer 4+ dans `tuning.Key` (repartir de la règle pour ces cases) ; regarder aussi le prior 1/2 avec min_fights 6. Le log « flee outcome » ne garde que la clé (3+) : journaliser aussi le nombre exact d'attaquants frais (mesuré le 2026-10-23, impossible de séparer 4-5 de 3 dans les logs actuels). Risque : les cases 4+ repartent vides, donc la règle fuit, et ses fuites meurent aussi (8/19 depuis le 21/10) ; gain attendu ~0,5 min par mort évitée.
- [~] (2026-10-23, variante QD60 `quest_defer` 0.6 REJETÉE : 26,1 (3 morts, échec) / 40,9 / 40,7 / 42,8 contre 37,3, 6 morts contre 1 ; la mine au niveau 7 tue tant que la clé de fuite met 4-6 kobolds dans la case 3, et tenter les quêtes passées en fin de liste refait les essais ratés ; code gardé, désactivé ; quêtes apprises par niveau, `journal/learn/quests.json` ; sur 8 runs 6 → 8, 47/60/62 reportées au niveau 6 dans 7 runs sur 8 après 4-13 min d'essais, faites en 2-6 min au niveau 7 ; historique : 62 échoue 90 % au niveau 6 et 69 % au 7, 47 84 % au 6, 87 et 88 n'ont jamais réussi) Mine de Fargodeep (quêtes 47, 60, 62, 87) : moitié des fuites de 5 à 8. Reporter dès la première fuite toute quête dont la cible est dans la mine, ou n'y aller qu'au niveau 8-9 ; mesurer. Vu aussi : quand 62 réussit au niveau 6, l'acceptation de 76 (niveau 8) arrête la route (`nextBatch` s'arrête à la première quête trop haute) et 61/85/86/106/84 attendent : grind 7 → 8 de 15-25 min ; à traiter à part.
- [~] (2026-10-19 : code 97 = hors de portée ; interpoler les positions pour tout, variante G : 54,2 min, plus lent ; variante I : `dist_now` pour la portée des sorts seulement, à mesurer) Sorts tentés pour rien (Victory Rush 34428/97 ×8-20 par run, Charge 100/128, Rend 772/97) : vérifier l'état d'aura, la distance et la cible avant de les lancer.
- [ ] Repos (5-7 %) : manger/boire dès le début du repos, Premiers soins (bandages) pour le guerrier.
- [ ] Maître de classe trop loin : `progress()` n'y va que si le maître est à moins de 400 yards (`maxTrainerDist`). Le snapshot `warrior_human_6` n'a pas Victory Rush : appris à 6-8 min dans 3 runs sur 4 de la série 59a81a3, jamais dans Benchekeuzz (aucun Victory Rush lancé de tout le run, 4 morts). À faire : y aller même plus loin quand un sort important manque (Victory Rush, nouveau rang de Rend, Thunder Clap, Battle Shout), puis mesurer.
- [ ] Battle Shout : maintenu (buffs hors combat sous 30 s, en combat sous 3 s, 1 seul échec sur la série 59a81a3), mais les sorts réussis ne sont pas journalisés (seulement « group cast » à 2+ attaquants et « cast failed ») : journaliser le temps de présence du buff pour le vérifier. Vérifié aussi : Thunder Clap sert bien de sort de zone (`near_attackers >= 2`, 77 lancers à 2+ attaquants sur la série) et de coup normal avant le niveau 20 ; Cleave (20) et Sweeping Strikes (30) sont configurés.

- [ ] Pull de groupe (`chain_pull`, 2026-10-19/20, RETIRÉ après la revue du 2026-10-20) : se placer là où la cible et un 2e monstre au repos nous voient, attendre qu'ils viennent. Sans objet tel quel : le choix de pull ne sert que ~9 fois sur ~80 combats d'un 6 → 8, presque tous commencent par un monstre qui nous attaque pendant la marche. À repenser autour de la marche (aller exprès au contact d'un 2e monstre en chemin) ; si on le refait : filtrer élites et rangs, compter seulement les monstres venus, une seule requête `nearby` pour l'attente. Code dans l'historique git (commit e544bb8, fichier `module/internal/combat/grouppull.go`).
- [~] Ralentissement de nuit (2026-10-19/20 : 10 runs 6 → 8 de 51-68 min entre 20 h et 7 h ; de jour 45,2 / 50,6 avec le même code) : benchs de jour seulement (NOTES). Cause à trouver : comparer un run de nuit et de jour (monstres vus, positions, fuites en marchant) ; redémarrer le worldserver pour voir si ça persiste (accord de l'utilisateur).
- [ ] Remesurer de jour : G (positions interpolées partout) n'est pas jugé (mesuré de nuit).
- [ ] À REMESURER (2026-10-20) : les variantes G (positions interpolées partout), H (pull de groupe), I et K ont tourné pendant la Sanssaint, avec les flame bunnies de Goldshire pris pour des monstres de niveau 70 (corrigé) : leurs temps (51-68 min) ne jugent pas ces changements. La référence de jour (N, 47,9 min) est faite ; reste l'interpolation complète, de jour.

### Idées (recherche du 2026-10-20 : guides de speedrun, code de mod-playerbots, notre code), par gain attendu
- [~] (2026-10-20/21, `auto/hub.go`, `quest_hub`, désactivé : 4 runs 6 → 8 en 48,3 min de moyenne contre 45,3 pour la référence, marche identique, pas de gain mesurable à Elwynn ; à réessayer sur les routes 10-20 aux trajets plus longs ou avec des lots plus grands) Quêtes par « hub » au lieu d'une à la fois : `auto.doQuest` fait prendre → objectifs → rendre pour chaque quête, donc un aller-retour par quête (mine de Fargodeep : 47, 60, 62 = trois trajets). Prendre toutes les quêtes d'un donneur / d'une ville, faire les objectifs proches ensemble (un kill compte pour toutes les quêtes qui le veulent), rendre tout en un passage. Vise la marche (22-26 %) et une partie du combat.
- [ ] Valeur d'une quête en XP/minute (comme `IsQuestWorthDoing` de playerbots et la règle des speedrunners : sauter escortes, objets du décor lents à réapparaître, quêtes « facteur » hors route) : XP de la quête / (trajet + objectifs estimés) comparé à l'XP/min du grind (`xpscore`) ; sauter ou garder pour plus tard celles qui rapportent moins que le grind sur place.
- [ ] Tuer ce qui est sur le chemin (playerbots : tout monstre qui donne de l'XP à moins de min(30, aggro + 10) yards pendant une quête ; speedrunners : les monstres verts/jaunes sur la route) : engager avec Charge (rage d'ouverture) au lieu d'être frappé de dos en marchant, et sans détour.
- [ ] Mort comme voyage avant le niveau 10 (pas de malus de résurrection, `Death.SicknessLevel` 11) : esprit guérisseur quand le cimetière est plus près de la prochaine étape que le corps. Contraire à la remarque utilisateur du 2026-10-12 (durabilité) : à proposer seulement si la mesure le justifie.
- [ ] Pierre de foyer liée à l'auberge du hub de quêtes, utilisée pour rendre les quêtes loin (recharge 30 min) : surtout utile hors d'Elwynn (étape 2).
- [ ] Premiers soins (bandages) pour le guerrier, butin sélectif quand les sacs sont pleins : petits gains (repos 2-5 %, butin 1-5 %).
  - Secourisme (2026-10-26) : rien à gagner hors combat avant ~20-30 (repos mesurés : 394, 4 s en moyenne, récupération de 5,4 à 7,8 %/s aux niveaux 6-11 ; un bandage de lin rend ~5 %/s). Plus tard : Heavy Runecloth 2000 PV en 8 s, une fois par minute (Recently Bandaged 60 s). Aucune action possible pendant les vols : craft des bandages au repos ou en ville. Garder le tissu d'ici là. Suivre le temps de repos par niveau dans le rapport de bench pour décider quand.
  - En combat (idée de l'utilisateur) : un monstre à bas PV qui fuit (il appelle ses voisins dans CreatureFamilyFleeAssistanceRadius) laisse 8 s sans coups : bandage si notre vie est basse, sinon Hamstring (appris au 8, pas dans la table de priorités) ou Charge/coup pour le finir avant qu'il amène des adds. Détection : cible < 20 % qui s'éloigne, ou l'émote « attempts to run away in fear! ».

### Étape 2 : quitter Elwynn (niveaux 10-20)
- [ ] Quête de classe guerrier 1640 « Beat Bartleby » (duel scripté) : débloque la Posture défensive ; puis postures dans la rotation si elles font gagner du temps.
- [ ] Nage (quêtes de murlocs, rivières) : `move` ne nage pas (section « Nage »).
- [ ] Respec de talents (niveau 25 du build guerrier) et talents par niveau.

### Étape 3 : niveaux 20-60
- [ ] Monture au niveau 20 (Apprentice Riding + monture) : réduit directement les 38 % de marche ; prévoir l'argent (or à garder dès le niveau 15).
- [ ] Transports : bateaux, tram, zeppelins (`UseTransport` des profils CopilotBuddy), autres continents (Kalimdor).
- [ ] Routes de quêtes 20-60 converties et vérifiées (`guidecheck`), quêtes de groupe et élites sautées.
- [ ] Équipement : améliorations achetées (hôtel des ventes ? marchands), armes de quête.

### Étape 4 : niveaux 60-80
- [ ] Outreterre (Portail des Ténèbres, 58-70) puis Norfendre (bateaux, 68-80) ; vol (monture volante 60/70, Vol par temps froid 77).
- [ ] Autres classes ou races seulement si elles montent plus vite (à mesurer sur 1 → 10).

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

## Prochaines étapes (validées par l'utilisateur le 2026-10-25, après le run de mesure en cours)
Run de mesure lancé le 2026-10-25 : 10 → 12 depuis `warrior_human_10` avec les corrections du commit
4cd16de (arrêts potions, quêtes proches d'abord), à comparer au run `20261025_1221_Benchrrtfnw` (162 min).
0. [ ] **Combat contre plusieurs monstres** (idées de l'utilisateur du 2026-10-25 ; le point a, marge mesurée,
   est fait, commit 467ea09, run de mesure en cours) :
   - b. **Fuir au dernier moment sûr** au lieu de fuir dès que l'issue prévue est mauvaise : continuer tant que
     la vie reste au-dessus de dps reçu × flee_exposure + flee_reserve ET que la cible peut mourir avant ce
     seuil (sa vie ÷ nos dps < (vie − seuil) ÷ dps reçu) ; ce qu'on tue avant de fuir est acquis (XP,
     quête, un monstre de moins au prochain essai), les dégâts faits aux survivants sont perdus (ils
     récupèrent leur vie en rentrant). Mesurer : monstres tués avant chaque fuite, morts pendant les
     fuites par vie au départ (rapport de bench).
   - c. **Ordre de focus** : tuer d'abord le plus grand rapport dps qu'il nous inflige ÷ temps pour le tuer
     (règle de Smith : minimise les dégâts totaux reçus), remplace la règle des 60 % (0 changement de cible
     en 70 combats à plusieurs au run 20261025_1221). Demande nos propres dps mesurés.
   - d. **Soigneurs** en tête du focus (incantations de soin vues par SMSG_SPELL_START + Spell.dbc, sorts
     des monstres dans la base), interruption quand le guerrier l'aura (Pummel).
   - e. Prévision de la perte restante par les mesures du combat en cours (dps reçus × temps de vie restant
     de chaque monstre), mêlée à la prévision apprise.
0bis. [x] (2026-10-25, `gearIfLooted` après `combat.defend`) **Équiper dès le butin** (vu par l'utilisateur le 2026-10-25) : un sac ramassé à 19:08:11 n'a été
   équipé qu'à 19:12:54 (`progress.gear` n'est appelé qu'entre deux quêtes ou deux tours) ; pendant ce temps
   6 places de moins, et des butins laissés « sacs pleins » (9 au run 20261025_1628). Appeler `gearUp` au
   premier moment hors combat après un butin qui contient un sac ou une amélioration.
0ter. [x] (2026-10-26, `travel.questNode`) **Nœuds de vol de quête** : TaxiNodes 285 et 287 (« Quest - Wintergarde -> Stars' Rest », « Quest - Valgarde
   -> Westguard Keep ») sont sur la carte 0 au maître de vol de Stormwind ; `travel.near_unknown` les croit
   inconnus et le bot va « apprendre » à chaque passage (run Benchwevujx, 2026-10-25). Exclure les noms
   « Quest - » du graphe (cmd/travelgraph) et de near_unknown.
0quater. [ ] **Morts du run Benchijjxyv (2026-10-26, camp Defias/Prowler vers (-8990,-790))** :
   - exploration « rester » à 61 % contre 3 monstres frais avec une perte prévue de 117 % (slack −56) :
     autorisée parce que fuir tue souvent dans cette case (`explore_flee_death`) ; ne jamais explorer
     quand la perte prévue dépasse la vie de plus de ~20 % ;
   - fuite à 75 % contre Prowler (118) + Riverpaw Outrunner, morte pendant la fuite (Prowler : 0,86 de
     notre vitesse en base, pas plus rapides) ; le bot venait de ressusciter dans le même camp ;
   - tenue « en attendant la mesure » contre 4 Defias (groupe de Morgan the Collector) : 93 → 64 % en 2 s ;
   - corrigé le 2026-10-26 : pas de tenue avant mesure contre 3 attaquants ou plus, pas d'exploration
     quand la perte prévue dépasse la vie de plus de 20 %.
   - cause commune : choix du pull (camps de 3-4 monstres engagés d'un coup).
1. [ ] **Monture au niveau 20** : achat chez le vendeur de montures de la race (humain : Katie Hunter,
   Eastvale Logging Camp, Elwynn ; apprentissage du sort Apprenti cavalier chez le maître d'équitation à
   côté, 35-40 po en 3.3.5 avec la réduction de réputation), puis l'utiliser pour toute marche longue en
   extérieur (hors combat ; descendre avant un pull ; pas en intérieur ni dans les grottes : sort refusé, se
   souvenir de l'endroit). Plus gros gain de temps attendu de la tranche 20-40 (60 % de vitesse en plus sur
   la marche, 32 % du temps au dernier run). Garder l'or nécessaire avant 20 : budget dans `town.restock`.
2. [ ] **Respec au niveau 25** (`journal/classes/warrior.yaml` le prévoit : retirer Improved Thunder
   Clap) : parler au maître de classe (dialogue « je veux désapprendre mes talents » = gossip, puis
   confirmation MSG_TALENT_WIPE_CONFIRM), payer (1 po la première fois), puis replacer les points dans
   l'ordre. Demande le support minimal des menus de dialogue (CMSG_GOSSIP_HELLO / SELECT_OPTION).
3. [ ] **Tram en rame** (détail plus bas, « Voyages longue distance ») : rames de TransportAnimation.dbc
   au lieu du tunnel à pied ; gain ~3-4 min par passage (2 passages au run 10 → 12).

## Voyages longue distance (module travel, idées du 2026-10-25)
Constat des runs 10 → 12 du 2026-10-25 : `travel.plan` vérifie les marches en jeu (`move.path` sur des milliers de yards) et dépasse les 30 s d'un appel de module ; le premier plan d'une session calcule aussi les pontons de tous les bateaux. Repli sur la marche directe (supprimé) : 3 morts dans la Gorge des Vents brûlants.
- [ ] **Tram des Profondeurs en rame** au lieu du tunnel à pied (~6 min → ~1 min + attente, gain 3-5 min par passage, quelques passages par leveling) : les 6 rames « Subway » (carte 369, entrées 176080-176085) sont des transports de type 11 animés par `TransportAnimation.dbc` (pas TaxiPathNode). Embarquement identique aux bateaux (mouvement avec MOVEMENTFLAG_ONTRANSPORT, accepté pour le type 11) ; reste à calculer quand une rame est à quai et laquelle prendre.
- [ ] **Lier la pierre de foyer** aux auberges de la route (étapes `set_hearth` de Zygor, ou auberge de la zone de quêtes en cours) : aujourd'hui seul le lien existant (Northshire) sert.
- [ ] Étapes `flight_path` de Zygor : aller apprendre ces nœuds même hors du chemin (aujourd'hui seulement à moins de 40 yards d'une marche).
- [ ] La marche réelle (`move.goto`, contournement des monstres jusqu'à ×7) peut s'écarter du chemin vérifié par le plan : vérifier aussi le danger des zones sur le chemin réellement suivi.
- [ ] Estimations : le plan a choisi une fois la pierre de foyer (242 s) quand la marche directe était estimée à 213 s (`walk_seconds` calculé autrement que les marches du plan) : une seule estimation.
- [ ] guidecheck compte comme « DEATH » les échecs « too dangerous for now: no safe way … DeadlineExceeded » (pas de mort réelle) : les classer comme fuite/danger.
- [ ] Zeppelins : même système que les bateaux (déjà gérés) ; seul celui de Westguard Keep (Fjord Hurlant) sert à l'Alliance, à tester le moment venu.
- [~] (2026-10-25 : carte faite, `cmd/minimap`, carte « Carte » du tableau de bord ; reste icônes et GlobalStrings) Autres données du client (`assets/client/Data`, lecteur `internal/mpq`) : images de la minimap (`Textures\Minimap`, BLP) et de la carte du monde pour le tableau de bord (position, chemin, graphe, morts) ; icônes `Interface\Icons` en local ; `GlobalStrings.lua` (frFR) pour des messages d'erreur lisibles dans les journaux.
- [ ] Rappel Honorbuddy : longue distance écrite à la main dans les profils (pierre de foyer, vols, `UseTransport` avec quai, pont et arrivée) ; écarté, on veut que le bot décide seul.

## Déplacement (idées des bots du serveur, 2026-10-10)
- [ ] Sorts interrompus (code 40) : ne pas se déplacer pendant une incantation quand la cible bouge un peu.
- [ ] Idées mod-playerbots (src/Ai, mouvements) : suivre une cible (distance/angle), vérifier qu'une destination est atteignable avant d'y aller, se débloquer.

## Nage, eau et quêtes d'eau (question utilisateur, 2026-10-11)
- [ ] `move` ne sait PAS nager : le navmesh est filtré sur le sol (GroundOnly), pas de drapeau de nage (MOVEMENTFLAG_SWIMMING 0x200000), pas de vitesse de nage, pas de gestion du souffle (SMSG_START_MIRROR_TIMER type 1, noyade après ~60 s sous l'eau, fatigue). Une destination dans l'eau est refusée (« not on the navmesh ») ; `nav.GroundPartial` (opt-in) mène au point marchable le plus proche mais n'est utilisé nulle part. A faire pour nager : (1) polygones NAV_WATER (flag 0x08) et niveau d'eau (liquide des `.map` et des vmaps) ; (2) paquets de mouvement de nage avec tangage ; (3) écouter le minuteur de souffle et remonter à la surface ; (4) ne nager que si un chemin marchable n'existe pas. Opus.
- [ ] Ce que fait mod-playerbots (lu le 2026-10-11, `assets/sources/playerbots_class` + src/Ai/Base) : les bots tournent DANS le serveur et utilisent ses données de liquide (`GetLiquidData` : dans l'eau / sous l'eau). Dans l'eau : drapeau de nage activé ou désactivé selon ce statut, et PAS de pathfinding (ligne droite) ; marche sur l'eau si une aura le permet. Ils ÉVITENT l'eau : déplacements aléatoires et « go » refusent une destination dans l'eau. Respiration : rien de spécial (pas de gestion du souffle), seulement les buffs de classe quand on nage (Respiration interminable du démoniste, Respiration aquatique / Marche sur l'eau du chaman). À reprendre : (1) niveau d'eau lu dans les `.map` (couche liquide ; notre `terrain` ne lit que les hauteurs) ; (2) dans l'eau, ligne droite avec MOVEMENTFLAG_SWIMMING et la vitesse de nage ; (3) éviter les destinations dans l'eau sauf besoin ; (4) en plus d'eux : minuteur de souffle (SMSG_START_MIRROR_TIMER) pour remonter.
- [ ] Les quêtes de murlocs du lac (112 Collecting Kelp, 114 The Escape) : les monstres sont sur la rive, en groupes de 5-6 (niveau 6-7) ; Claudio y est mort 2 fois (niveau 8, 193 PV) sans nager : combat contre le groupe. A faire : évitement/aggro de groupe (voir « Sécurité ») avant de refaire ces quêtes.

## Quêtes spéciales à gérer dans `auto`
- [ ] Quêtes de soin / de buff sur des PNJ (beaucoup de quêtes de prêtre : ex. 5624 « Garments of the Light » : soigner Guard Roberts avec Soins inférieurs rang 2, puis Robustesse, etc.). Aujourd'hui `auto` les traite comme un objectif « kill » et risquerait d'attaquer le PNJ allié : elles sont sautées à la main. A faire : détecter les objectifs dont le PNJ est allié (faction non hostile / pas de statut « attaquable »), ne jamais appeler `combat.kill` dessus, et ajouter une action `quest.cast {spell, npc_entry}` (lancer un sort sur un PNJ, avec la liste des sorts demandés lue dans le texte de la quête ou saisie à la main). Niveau requis : Opus (protocole + logique de ciblage).

## Sélection des quêtes de la route (décisions utilisateur, 2026-10-11)
- [ ] L'ORDRE DES ÉTAPES ZYGOR EST À RESPECTER : quand la prochaine quête demande un niveau plus haut (niveau minimum, ou monstres hors de la marge de danger), `auto` doit grinder près de là jusqu'au niveau voulu au lieu de passer à la suite. A changer dans `guidecheck -plan` / le futur plan intégré d'`auto`.
- [~] Quêtes qu'`auto` ne sait pas faire : liste `journal/quests_manual_human_warrior.md` (`guidecheck -manual`), 281 sur 1925 après le travail du 2026-10-11 (356 avant). Décisions de l'utilisateur par catégorie :
  - [ ] Crédit de kill par script (118 restantes) : À FAIRE AU CAS PAR CAS quand le personnage arrive à la quête en jeu.
  - [ ] Escortes, scripts, duels (88, ex. 1640 Beat Bartleby, 945 Therylune's Escape) : À FAIRE quand on en aura besoin en jeu (module escorte : suivre le PNJ, défendre, ne pas le dépasser).
  - [ ] Objets qu'aucun monstre ne lâche (53 : achetés, fabriqués, donnés par un script, ou objet à utiliser sur une cible) : backlog, cas par cas.

## Leveling automatique `level.sh` (2026-10-11)
- [ ] Mine de Fargodeep (47, 60) : mort en y marchant (groupes de kobolds) ; dépend du pull d'un seul monstre / évitement.
- [ ] Voyages (bateau, tram, vols, autre continent) : `level.sh` ne sait pas encore changer de zone autrement qu'à pied.

## Revue de mod-playerbots (2026-10-11) : à développer
Détail et comparaison : `journal/playerbots_review.md`. Attention, une partie de leur aisance vient de
triches serveur (sorts, talents, équipement et consommables offerts, téléportations) que nous n'avons pas.
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
3. [x] (2026-10-12 : `combat/priority.go`, format dans `journal/classes/README.md` ; reste « sort presque prêt », posture, talents) Rotation en TABLE DE PRIORITÉS par classe (modèle Ovale : la première ligne dont les conditions passent et dont le sort est lançable gagne), dans `journal/classes/<classe>.yaml`. Conditions utiles d'Ovale :
   - BuffExpires/Present(n s, stacks), TargetDebuffExpires(mine) ;
   - TargetLifePercent, Mana/rage, Stance, TalentPoints ;
   - « 1s before Spell(X) » (sort presque prêt) ;
   - TargetDeadIn (temps avant la mort de la cible, estimé par la baisse de ses PV : pas de DoT/Rend sur une cible qui meurt dans moins de 6-8 s, mieux que notre « Rend si cible > 50 % »).
4. [~] (2026-10-12 : tables de leveling bas niveau reprises de l'ancien code ; à enrichir avec les sorts des niveaux 20+) Tables de priorités Guerrier (Armes) et Prêtre (Ombre) : priorités détaillées dans `journal/rotations/warrior.md` et `priest.md` (Ovale, Mirash, Afenar).
6. [~] (2026-10-12 : dans les tables, sauf Berserker Rage contre la peur et Fade ; pas encore vus en jeu, appris à partir du niveau 20) Défensifs, seuils de playerbots : PV critiques 25 %, bas 45 %, moyens 65 %, presque pleins 85 %.
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

## Choix des monstres par XP/minute (idée utilisateur, 2026-10-12)
- [~] Fait le 2026-10-12 : `xpscore` (choix des cibles de grind), P_mort mesurée par monstre et écart (combats enchaînés compris), « niveau + 1 » d'auto et capacité de pull remplacés par le score (perte attendue). Reste : régénération mesurée ; perte par add apprise (1,5 fixe) ; tables du prêtre (aucun combat appris : valeurs du guerrier par défaut). Modèle : XP/min = XP_kill / (T_kill + T_récup + T_recherche + P_mort × T_mort).
  - XP_kill : formule du serveur (×(20+écart)/20 au-dessus, réduction grise en dessous).
  - Monstre (base, vérifié) : PV = creature_classlevelstats.basehp{exp} × HealthModifier ; dégâts = damage_base (+ attackpower) × DamageModifier, BaseAttackTime ; densité = nombre d'apparitions et spawntimesecs (table creature).
  - Nous (mesuré) : DPS effectif et dégâts reçus par niveau d'écart depuis les lignes `fight` ; régénération depuis les repos ; P_mort depuis les combats perdus (adds, camps).
  - Usage : choix des cibles de grind (level.sh), puis remplace la règle fixe « niveau + 1 » et la capacité de pull (évolue seule avec l'équipement).
  - Repères : mêlée +0/+1, lanceurs ≤ +2 (raté des sorts 17 % à +3), personnage fragile −1/−2.

## Profils CopilotBuddy (lus le 2026-10-13, `assets/sources/copilotbuddy/`, hors git, sans licence : référence seulement)
Profils de quêtes Honorbuddy 3.3.5 (232, dont 97 Alliance), comportements spéciaux, transports. L'utilisateur lève l'obligation de suivre Zygor si mieux.
- Route : à Elwynn 1-11, même ordre que Zygor (53 quêtes communes) ; ensuite Darkshore 11-14 en bateau (Zygor : Dun Morogh/Loch Modan à pied, mieux pour nous tant qu'il n'y a pas de voyages). Couverture 11-60 trouée (zones isolées « 25-25 »), pas de grind de leveling : Zygor reste la route principale.
- [ ] Recettes de quêtes : 66 de nos 281 quêtes manuelles ont un comportement chez eux (44 « crédit par script » = `UseItemOn` objet X sur monstre Y, 14 `InteractWith`, `DefendObject`…). Convertir ces paramètres en données pour `auto` (objet, cible, nombre).
- [ ] Quêtes de classe : profil « Class Quests_Warrior » (1638-1640, 1665, 1678-1684, 1712-1719, 1791-1792…), et un par classe : les insérer dans le plan de `level.sh` au bon niveau.
- [ ] Quêtes de groupe : fichiers « … Group Quests » séparés (Outreterre) : les marquer à sauter.
- [ ] Transports : `UseTransport` (388 usages : id du transport, point d'attente, de montée, de descente) + fichiers `Profile v3/Transport/` : base des voyages en bateau/zeppelin.
- Autres profils Honorbuddy (cherchés le 2026-10-13) : dépôt officiel BosslandGmbH/Honorbuddy-Profiles et pack de Kick (SVNtoGit/Kicks). Leurs profils 1-60 sont ceux de Cataclysm (12-13 % des quêtes existent en 3.3.5) : inutilisables. Outreterre et Northrend valides à 99-100 %. Pack de Kick copié dans `assets/sources/hb_kicks/` (BC 58-70, LK 68-80, transports) : 6537 comportements spéciaux en Northrend (escortes, véhicules, objets), mais conçus pour le vol (« [Fly] », 1396 FlyTo : vol à 60 en Outreterre sous Cataclysm, 70/77 en 3.3.5). Recettes : 58 de nos quêtes manuelles chez Kick, 66 chez CopilotBuddy, 103 en tout. Route : Zygor reste la base ; CopilotBuddy recoupe mieux Zygor (Northrend 704 quêtes communes contre 498 pour Kick).
- Routines de combat Honorbuddy (2026-10-13) : Singular (routine livrée par défaut) en portage WotLK, copiée dans `assets/sources/singular_wotlk/` (référence, sans licence). L'original de Bossland (github BosslandGmbH/Singular, 2017) vise Legion : sorts différents. Les routines communautaires de l'époque (forums Honorbuddy) ne sont pas sur GitHub. Ce que Singular fait et pas nous :
  - [~] Guerrier : bloc multi-cibles quand ≥ 2 ennemis à < 6-8 yards (FAIT le 2026-10-13 : Thunder Clap, Sweeping Strikes, Cleave à la place de Heroic Strike, attaquant le plus faible d'abord ; bench 48,3 min / 0 mort mais seulement 3 combats à 2 au niveau 5 : à mesurer sur un bench 1 → 10 ; reste Rend sur chacun ; Demoralizing Shout est dans `training.skip` du guide) ; Berserker Rage contre la peur ; Commanding Shout faute de Battle Shout ; interruption des incantations (Shield Bash / Pummel). Pour nous en plus : Hamstring sur un monstre qui fuit à bas PV (il appelle ses voisins à 30 yards, CreatureFamilyFleeAssistanceRadius).
  - [ ] Toutes classes : potion / pierre de soins sous un seuil de PV (réglable), avant la fuite.
  - [ ] Prêtre ombre (niveau 20+) : priorités complètes (Vampiric Touch, Devouring Plague, Mind Blast, Mind Flay, SW:Death, Shadowfiend, Dispersion sur mana bas, Inner Focus, Holy Fire/Smite si la cible est immunisée à l'ombre, DoT seulement si cible élite ou > 40 %) : base pour la rotation d'ombre du BACKLOG.
  - Déjà chez nous : Charge, Victory Rush, Execute (Sudden Death), Overpower, Rend, Mortal Strike, Slam, Retaliation, Enraged Regeneration, Bloodrage, Battle Shout ; prêtre bas niveau identique (Bouclier, Soins rapides < 40 %, Douleur, Attaque mentale, Châtiment).
- Idées (à réécrire, code Honorbuddy décompilé) : en combat, taper l'attaquant au plus bas de PV ; liste de monstres à éviter (zones interdites sur le navmesh) ; déblocage gradué (saut, pas de côté, zone marquée puis recul).
- [ ] Quête de classe 1640 « Beat Bartleby » (guerrier humain) : Bartleby devient hostile après l'acceptation (script) ; le battre jusqu'au seuil sans le tuer. Débloque 1665-1667 et la Posture défensive.
- [ ] Contournement des monstres : granularité limitée par la taille des polygones du navmesh (un grand polygone de prairie ne se contourne pas à l'intérieur). Mesurer les combats en marchant avant / après.
