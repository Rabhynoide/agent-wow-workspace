# Revue de mod-playerbots : ce qu'il faut leur prendre (2026-10-11)

Source : github.com/liyunfan1223/mod-playerbots, commit 79bd428 (2026-10-09), environ 100 000 lignes hors
classes, raids et donjons. Copie locale des classes dans `assets/sources/playerbots_class/`.

## À savoir d'abord : ils trichent, pas nous
Les bots tournent DANS le serveur. Une partie de leur aisance vient d'aides que nous n'aurons pas en
jouant comme un vrai client :
- au passage de niveau (`AutoMaintenanceOnLevelupAction`), le serveur leur apprend tous les sorts de
  classe et de quête et leur pose les talents (`PlayerbotFactory::InitTalentsTree`) ;
- il leur crée équipement, nourriture, munitions, réactifs et consommables (`InitFood`, `InitAmmo`,
  `InitReagents`, `InitConsumables`) ;
- il les téléporte vers une zone de leur niveau (`AutoTeleportForLevel`), et vers leur destination
  quand ils sont bloqués (`MoveFarTo`, après un temps sans progrès) ;
- ils lisent directement l'état du serveur : menace, cible des monstres, rayon d'aggro, liquides,
  ligne de vue.

Conséquence : nous devons gagner ce qu'ils reçoivent. Il faut acheter nourriture, eau, munitions et
réactifs, apprendre chez l'entraîneur (déjà fait), et nous débloquer sans téléportation.

## Architecture (moteur)
- Des stratégies cumulables (combat, hors combat, mort, butin, quêtes, grind, voyage…). Chacune liste
  des **déclencheurs → actions avec une priorité**. Le moteur exécute l'action utile la plus prioritaire
  à chaque tick, avec des actions de repli (`NextAction` alternatives).
- Des « valeurs » calculées et mises en cache : attaquants, cibles possibles, cible de grind, adds
  possibles, usage d'un objet…
- Pour nous : le même modèle s'applique au combat (voir `journal/rotations/`) et pourrait piloter
  `auto` hors combat (manger, se reposer, vendre, apprendre).

## Ce qu'ils font, comparé à nous
| Domaine | mod-playerbots | Nous |
|---|---|---|
| Décider quoi faire | Machine à états RPG : IDLE, GO_GRIND, GO_CAMP (PNJ), WANDER_RANDOM/NPC, DO_QUEST, TRAVEL_FLIGHT, REST, avec poids et durées | `level.sh` : route Zygor + grind |
| Trouver où faire une quête | Tables `quest_poi` / `quest_poi_points` (zone de chaque objectif, et lieu du rendu) | Spawns des créatures et objets, sources des objets |
| Quêtes | Acceptent tout chez les donneurs rencontrés ; trient le journal (jettent les quêtes grises, répétables, élites ou de groupe, et celles d'autres zones) ; essaient au hasard les objets de quête | Route imposée ; liste manuelle |
| Cible de grind | Attaquants d'abord ; sinon le plus proche, en vue, au plus 4 niveaux au-dessus, pas élite, **à moins de 5 yards en hauteur** ; en route, seulement ce qui est utile aux quêtes, à portée d'aggro + 10 yards | Points d'apparition, marge de danger |
| Adds | `possible adds` : monstre hostile qui n'attaque pas encore, à portée d'aggro de notre cible mais pas collé à elle. Pendant un pull : fuite | Rien |
| Fuite et point sûr | `FleeManager` : points candidats sur des cercles, sans les directions des ennemis, sans eau, en vue, en gardant le plus éloigné des monstres | Rien |
| Mort | Libération, course au corps, puis **résurrection au bord du rayon (39 yards) au point le plus sûr** (FleeManager) ; « bloqué » ou « chute » : libération | Esprit guérisseur ≤ niv. 10, corps sinon |
| Repos | Manger si PV < moyen, boire si mana bas ; potions et pierre de soins à PV critiques | Attente des PV sans manger |
| Raciaux | Chaque racial a son déclencheur (Every Man for Himself contre la perte de contrôle, Gift of the Naaru à PV moyens, Stoneform contre poison/saignement…) | Rien |
| Position en mêlée | Se placer derrière la cible (pas de parade ni de blocage de face) ; face à la cible | Face à la cible |
| Lanceurs | « ennemi trop près » → fuir (garder la distance) | Rien |
| Blocage | Pas de progrès de 5 yards → reset, sinon téléport (triche) | Déblocage en ligne droite (fait) |
| Voyage | Graphe de nœuds avec types de chemin : marche, portail, **transport** (bateaux, zeppelins, ascenseurs), **vol**, sort de téléport | À pied, une carte |
| Objets | `ItemUsage` : équiper/remplacer, quête, métier, utilisable, désenchanter, HV, garder, vendre, munitions ; récompense de quête par poids de stats | sellrule + gear (équivalent) |
| Monture | Vérifie et utilise la monture sur les longues distances | Rien |
| Eau | Nage en ligne droite ; évitent l'eau comme destination | Rien (BACKLOG Nage) |
| Divers | Pêche, métiers, courrier, HV, guilde, champs de bataille, donjons/raids, discussion | Hors sujet pour l'instant |

## Ce que nous devrions développer (par ordre de rentabilité pour le leveling solo)
1. **Repos réel** : manger et boire après le combat (seuils PV/mana), acheter nourriture et eau chez le
   marchand selon le niveau. Potions et pierre de soins à PV critiques. Racials par déclencheur.
2. **FleeManager** : choisir un point sûr. Il sert à quatre choses :
   - se relever au bord du rayon du corps loin des monstres (niveau 11+) ;
   - reculer après un pull ;
   - fuir un groupe ;
   - garder la distance pour un lanceur.
3. **Adds possibles et pull d'un seul monstre** : ne pas engager une cible qui a des voisins hostiles à
   portée d'aggro ; sinon tirer de loin (arme à distance) et reculer vers un point sûr.
4. **Règles de cible de grind** :
   - attaquants d'abord ;
   - même hauteur (± 5 yards : pas les monstres sur une falaise ou dans une grotte) ;
   - en vue ;
   - pendant les déplacements, ne combattre que ce qui est utile aux quêtes ou ce qui nous attaque.
5. **Points d'intérêt des quêtes (`quest_poi`)** : zone de chaque objectif, utile pour les objets donnés
   par script, les zones, les cibles de sorts et le rendu.
6. **Combat** : moteur de priorités (voir BACKLOG « Rotation et combat ») ; se placer derrière la cible
   en mêlée ; lanceurs à distance.
7. **Voyages** : vols (maître de vol, CMSG_ACTIVATETAXI), bateaux et zeppelins (attendre au quai,
   transport), tram, pierre de foyer ; monture à partir du niveau 20 (achat + usage).
8. **Journal de quêtes** : jeter les quêtes devenues grises ou impossibles, garder des places libres.
9. **Blocage** : suivi du progrès (5 yards sur une durée) et récupération graduée sans téléport :
   déblocage, autre chemin, pierre de foyer.
