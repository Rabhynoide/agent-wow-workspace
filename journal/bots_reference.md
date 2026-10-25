# Référence Honorbuddy / mod-playerbots, module par module (2026-10-25)

**À relire avant de concevoir un nouveau besoin** (nouveau module, nouvelle étape du leveling, problème
qu'on n'a pas encore rencontré) pour voir comment ils l'ont traité et ne pas repartir de zéro. Mettre à
jour la ligne du module concerné quand on fait quelque chose de nouveau.

Détails déjà écrits ailleurs (ne pas les recopier ici) :
- `journal/playerbots_review.md` : revue de mod-playerbots du 2026-10-11 (architecture, tableau
  comparatif, triches).
- `BACKLOG.md` « Profils CopilotBuddy », « Voyages longue distance », « Revue de mod-playerbots ».

Sources locales (hors git, sans licence : lecture seulement) :
- `assets/sources/playerbots_class/` : IA de classe de mod-playerbots.
- `assets/sources/singular_wotlk/` : routine de combat Singular.
- `assets/sources/copilotbuddy/` : profils de quêtes Honorbuddy 3.3.5, comportements, `Transport/`.
- `assets/sources/hb_kicks/` : profils BC/LK de Kick, avec `Transports/`.
- mod-playerbots en ligne : github.com/liyunfan1223/mod-playerbots.

Fiabilité : mod-playerbots a été lu dans son code. Honorbuddy est fermé et arrêté en 2017 : ce qui est
écrit ici vient de ses profils et routines publics, de son API connue (Styx) et de souvenirs. Les points
marqués (?) sont à vérifier avant de s'y appuyer.

## Règle « sans triche » (comment on juge la faisabilité)

Nous jouons comme un vrai client, par paquets.

**Autorisé :**
- ce que le serveur envoie à un client ;
- les données du client (`.map`, mmaps, vmaps, DBC) ;
- les données statiques publiques qu'un joueur trouverait sur Wowhead ou dans un guide (base `acore_world`
  en lecture : PNJ, quêtes, apparitions, butins) ;
- les calculs hors jeu faits à partir de tout cela ;
- l'apprentissage sur nos propres parties.

**Interdit :**
- commandes de MJ, SOAP pendant le jeu (le SOAP ne sert qu'aux snapshots du banc d'essai) ;
- modifier le serveur ou sa base ;
- téléportations ;
- objets, sorts ou talents offerts ;
- lire l'état dynamique du serveur qu'un client ne voit pas (menace exacte, cible réelle d'un monstre hors
  de vue, positions hors de portée).

**Zone grise (acceptée jusqu'ici, à garder en tête) :**
- `detection_range` de creature_template : la formule d'aggro est publique, la valeur exacte ne l'est pas.
- Temps de réapparition lus en base : un joueur les estime en observant.

Les deux bots de référence se situent ainsi :
- **mod-playerbots** tourne DANS le serveur : accès direct à tout, plus des aides (téléportation de
  niveau ou de déblocage, sorts, talents, équipement et consommables offerts).
- **Honorbuddy** était injecté dans le vrai client : il lisait sa mémoire et appelait son API Lua.
  Côté données, il voyait exactement ce que voit un client, comme nous ; le reste venait de profils
  écrits à la main et de maillages de navigation calculés par Bossland.

Conséquence : **tout ce que faisait Honorbuddy est faisable pour nous sans triche** ; il nous manque
seulement l'API Lua, qu'on remplace par les paquets et la base. Pour playerbots, il faut trier
idée par idée.

## Module par module

### state (paquets → état)
- **Honorbuddy** :
  - Objets lus dans la mémoire du client (ObjectManager), plus l'API Lua : menace via
    `UnitThreatSituation`, journal de combat, textes des quêtes, cooldowns.
  - Événements Lua (combat log, loot, gossip).
- **playerbots** : objets C++ du serveur, avec la vraie table de menace, les liquides et la ligne de vue
  du serveur.
- **Faisable sans triche :** oui, c'est ce qu'on fait. Le journal de combat complet est faisable par
  les paquets SMSG_SPELLLOGEXECUTE, SMSG_ATTACKERSTATEUPDATE et SMSG_SPELLNONMELEEDAMAGELOG.
  La menace approchée est faisable comme dans le client : SMSG_THREAT_UPDATE est envoyé au client
  pour les monstres qui nous ont en table (?).

### world (base statique)
- **Honorbuddy** : pas de base de données. Les quêtes, PNJ et points venaient des profils XML
  (coordonnées écrites à la main) et du cache WDB du client.
- **playerbots** : `sObjectMgr` du serveur, plus les tables `quest_poi` / `quest_poi_points`.
- **Faisable sans triche :** oui, la base est en lecture et ne contient que des données publiques.
- **Mieux et propre :**
  - CMSG_QUEST_POI_QUERY : le client 3.3.5 demande lui-même les zones d'objectifs (flèches de la
    carte), et le serveur répond avec `quest_poi`. C'est une source légitime pour les objectifs
    « par script ».
  - CMSG_QUEST_QUERY / CMSG_CREATURE_QUERY : textes et noms, comme le client.

### quest
- **Honorbuddy** :
  - Profils « Questing » en XML : `PickUp`, `Objective` (KillMob, CollectItem…), `TurnIn`, `If/While`,
    `RunTo`.
  - Plus de 100 « Quest Behaviors » en C# pour les cas spéciaux : `InteractWith` (gossip, objets),
    `CombatUseItemOn`, `UseItemOn`, `VehicleMover`, `EscortGroup`, `DefendObject`, `UseTransport`,
    `FlyTo`.
  - Profils de Kick et CopilotBuddy : une recette par quête difficile.
- **playerbots** :
  - Accepte toutes les quêtes des donneurs croisés et trie le journal (jette les grises, de groupe,
    d'élite, hors zone).
  - Objectifs par `quest_poi` ; essaie au hasard les objets de quête.
  - `IsQuestWorthDoing` ; récompense choisie par poids de stats.
- **Nous :** route Zygor plus `auto.run`, avec une liste de quêtes manuelles.
- **Faisable sans triche :** tout. Les recettes CopilotBuddy/Kick sont des données à convertir (103 de
  nos quêtes manuelles y ont une recette, voir BACKLOG).
- **Reste à faire :** gossip (CMSG_GOSSIP_HELLO / SELECT_OPTION), véhicules, escortes. Voir « Utile pour
  la suite ».

### combat
- **Honorbuddy** :
  - « Combat Routines » en C# (Singular par défaut) : arbre de priorités (`PrioritySelector`, behavior
    trees) par classe et spécialisation.
  - Pull, repos, buffs et soins sont des comportements séparés.
  - Pas d'apprentissage. Fuite rare : Honorbuddy mourait et faisait la course au corps.
- **playerbots** :
  - Stratégies cumulables, déclencheurs → actions avec priorité.
  - `FleeManager` pour les points sûrs ; derrière la cible en mêlée ; évitement des AoE.
  - Cibles par « valeurs » : attaquants, adds possibles.
  - Menace réelle lue sur le serveur (avantage de triche).
- **Nous :** table de priorités YAML, fuite apprise (`flee.json`, `tuning.Decide`), choix de pull par perte
  attendue apprise (`fights.json`), contournement. On est **en avance** sur les deux pour le risque et la
  fuite.
- **Faisable sans triche :** tout, sauf la menace exacte (approchée).
- **À reprendre :**
  - Se placer derrière la cible (le moins de parades et de blocages).
  - Interruptions (Shield Bash / Pummel).
  - Berserker Rage contre la peur.
  - Postures : Défensive pour la quête 1640, Berserker à 30 (Intercept, Whirlwind).

### town (vendre, réparer, acheter)
- **Honorbuddy** :
  - Le profil liste vendeurs, réparateurs et boîtes aux lettres (`<Vendors>`, `<Mailboxes>`).
  - Retour en ville quand les sacs sont pleins ou la durabilité est basse ; vente par qualité, liste
    « protégée ».
  - Envoi de courrier vers un autre personnage.
- **playerbots** :
  - `ItemUsage` : équiper, quête, métier, désenchanter, vendre, garder.
  - Réparation et vente aux PNJ croisés, mais la nourriture, les munitions et les réactifs sont créés par
    le serveur (triche).
- **Nous :** équivalent (`sellrule`, `make_room`, vendeur sur le chemin), sans triche.
- **À reprendre :**
  - Hôtel des ventes : acheter des améliorations bon marché aux niveaux creux (CMSG_AUCTION_LIST_ITEMS,
    légal ; mais sur notre serveur, le marché n'existe que si un bot HV tourne).
  - Courrier : rien à reprendre en solo.

### auto (orchestrateur)
- **Honorbuddy** :
  - « BotBases » : Questing (profil), Grind (hotspots avec niveau min/max, `<Blackspots>`,
    `<AvoidMobs>`), Gatherbuddy, BG.
  - La logique est dans le profil, écrit et réglé par un humain zone par zone.
  - Enchaînement de profils par niveau (`LoadProfile`).
- **playerbots** :
  - Machine à états RPG (`NewRpgAction`) : GO_GRIND, GO_CAMP, DO_QUEST, WANDER, TRAVEL_FLIGHT, REST,
    avec poids et durées.
  - `TravelMgr` choisit une destination (quête, grind, PNJ, exploration) selon niveau, distance et
    intérêt.
  - `AutoTeleportForLevel` (triche).
- **Nous :** route Zygor plus `level.sh` (reports appris, grind par XP/min).
- **Faisable sans triche :** tout, sauf la téléportation de niveau. Leur choix de destination pondéré
  ressemble à notre idée « valeur d'une quête en XP/minute » (BACKLOG).

### progress (entraîneur, talents, équipement)
- **Honorbuddy** : plugins `Talented` (ordre de talents écrit), `AutoEquip2` (poids de stats), apprentissage
  chez le maître par le profil ou un plugin (?).
- **playerbots** : au passage de niveau, le serveur apprend sorts et talents (`PlayerbotFactory`,
  « premade specs ») et crée l'équipement (triche). Il existe aussi une vraie action d'entraîneur et
  `StatsWeightCalculator` pour comparer les objets.
- **Nous :** comme Honorbuddy, sans triche (entraîneur, ordre YAML, profils de stats).
- **À reprendre :** maîtres d'armes (épées et haches à deux mains selon les drops ; CMSG_TRAINER_BUY_SPELL),
  et poids de stats qui changent avec le niveau.

### tuning / apprentissage
- **Honorbuddy et playerbots :** aucun apprentissage, réglages fixes (fichier de configuration, options de
  routine).
- **Nous :** fuite, pulls, quêtes et exploration appris sur nos parties. C'est **notre avance**, rien à
  reprendre chez eux.

### travel (voyages)
- **Honorbuddy** :
  - Profils écrits à la main : `UseTransport` (id du transport, point d'attente, de montée, de
    descente, de sortie), `FlyTo`, `InteractWith` sur le maître de vol, puis Lua `TakeTaxi`, `UseItem`
    pierre de foyer.
  - Les versions tardives prenaient seules les vols connus (?).
  - Monture volante : navigateur 3D « Flightor » (vol libre en ligne droite avec évitement du relief).
- **playerbots** :
  - `TravelNodeMap` **précalculé** (tables `playerbots_travelnode`, `_link`, `_path`).
  - Nœuds : maîtres de vol, portails, quais, ascenseurs, entrées de donjons.
  - Liens typés : marche (chemin MMAP du serveur enregistré), portail, transport, vol, sort de
    téléportation.
  - Généré une fois par commande, puis A* sur ce graphe ; les bots montent sur les transports côté
    serveur.
- **Nous :** graphe précalculé hors jeu comme playerbots (`cmd/travelgraph`, 2026-10-25), avec en plus le
  danger de chaque marche (niveau des monstres près du chemin) ; vols, portails, bateaux (horaires exacts),
  pierre de foyer. Seules les marches de raccord sont vérifiées en jeu.
- **Faisable sans triche :** tout. Le graphe précalculé est un calcul hors jeu sur les mmaps et la base,
  comme les maillages d'Honorbuddy.
- **Prochaines étapes :**
  - Monture (20, 40), puis vol libre en Outreterre (60, 70 en 3.3.5 selon la monture) et Norfendre
    (77, Vol par temps froid).
  - Ascenseurs (type 11, comme les rames du tram).
  - Coordonnées de quais toutes prêtes dans `copilotbuddy/.../Transport/` et `hb_kicks/Transports/` pour
    vérifier les nôtres.

### move (navigation)
- **Honorbuddy** :
  - Maillages Recast/Detour générés par Bossland depuis les données du client (téléchargés par tuiles).
  - `ClickToMove` du client ; distance pour monter à cheval.
  - « Stuck handler » gradué : saut, pas de côté, recul, puis zone marquée.
  - Blackspots dans les profils ; nage gérée.
- **playerbots** :
  - `PathGenerator` du serveur (MMAP), `MotionMaster`.
  - Bloqué sans progrès de 5 yards → reset, sinon téléportation (triche).
  - Évitent l'eau comme destination et nagent en ligne droite.
- **Nous :** navmesh dans le module, contournement des monstres dans l'A* (aucun des deux ne le fait),
  préférence pour les routes lues dans les textures du terrain du client (2026-10-25, aucun des deux non
  plus), déblocage en ligne droite, `safe_point`.
- **Faisable sans triche :** tout. Le vol 3D se fait avec les vmaps comme obstacles.
- **À reprendre :**
  - Déblocage gradué : saut (MSG_MOVE_JUMP), pas de côté, blackspot appris là où on reste coincé.
  - Nage : couche liquide des `.map`, MOVEMENTFLAG_SWIMMING, souffle par SMSG_START_MIRROR_TIMER.
  - Monture.

### Outils hors jeu (zygor2route, guidecheck, xpscore, bench)
- **Honorbuddy :** équivalent de nos routes = profils de quêtes (Kick, CopilotBuddy), écrits à la main.
  Pas d'outil de mesure.
- **playerbots :** pas de route : choix dynamique.
- **Nous :** routes converties depuis Zygor, validation contre la base, XP/min par monstre, banc d'essai
  chronométré. Rien d'équivalent chez eux.

## Utile pour la suite (pas encore fait), par moment du leveling

Gains estimés en minutes : à mesurer selon le protocole avant de les croire.

**Tout de suite (10-20)**
1. **Graphe de trajets précalculé** (playerbots) : supprime les 30 s de planification et les faux
   « DEATH » ; rend Darkshore, Westfall et le Loch Modan accessibles sans risque.
2. **Lien de la pierre de foyer** dans les auberges de la route (Zygor `set_hearth`) : chaque retour gagne la
   marche.
3. **Gossip** (CMSG_GOSSIP_HELLO / SELECT_OPTION, `InteractWith` d'Honorbuddy) : débloque des quêtes
   manuelles et les PNJ à options.
4. **Recettes de quêtes CopilotBuddy/Kick** converties en données (`UseItemOn`, `InteractWith`) : 103
   quêtes manuelles.
5. **Secourisme** (métier légal, maître à Stormwind) : bandages, moins de repos pour un guerrier.
6. **Interruptions et Berserker Rage** (Singular).

**Niveau 20-40**
1. **Monture à 20** (achat chez le vendeur de la race, ~60 % de vitesse) : le plus gros gain de temps
   de trajet ; à 40, monture épique. Honorbuddy et playerbots montent au-delà d'une distance (~30-50
   yards) et jamais en intérieur.
2. **Posture Berserker à 30** (Intercept, Whirlwind) ; maîtres d'armes.
3. **Autres bateaux et ascenseurs** : Menethil ↔ Theramore, Auberdine ↔ Teldrassil / Azuremyst (même code
   que The Bravery) ; ascenseurs (Thunder Bluff, Undercity, Thousand Needles : type 11, comme les rames
   du tram). Les zeppelins sont réservés à la Horde.
4. **Donjons par l'outil de recherche** (LFG, 3.3.5) : beaucoup d'XP, mais il faut un groupe. Sans autres
   joueurs, seul notre propre multiboxing (plusieurs clients agent-wow) est légal : gros chantier,
   à évaluer plus tard.

**Niveau 58-80 (Outreterre, Norfendre)**
1. **Véhicules** (CMSG_REQUEST_VEHICLE_EXIT, sorts de véhicule, SMSG_PET_SPELLS) : des dizaines de
   quêtes en Norfendre (Kick : `VehicleMover`, 6537 comportements spéciaux en LK).
2. **Escortes** (`EscortGroup`) : suivre le PNJ et défendre.
3. **Vol libre** à 60/70 en Outreterre et 77 en Norfendre (Flightor) : navigation 3D avec les vmaps.
4. **Portail vers l'Outreterre** (Porte des Ténèbres : areatrigger, déjà géré), bateaux et zeppelins vers
   le Norfendre (Stormwind ↔ Borean Tundra, Menethil ↔ Howling Fjord).

**Robustesse**
- Déblocage gradué et blackspots appris (Honorbuddy).
- Nage et souffle (playerbots plus nous).
- Reconnexion automatique après une déconnexion (Honorbuddy le faisait via un relogger).
- Approximation de menace par SMSG_THREAT_UPDATE (?), pour savoir qui nous a pris en cible avant le
  coup.

**Écarté (et pourquoi)**
- Téléportations, sorts, talents et objets offerts de playerbots : triche.
- Profils écrits à la main pour chaque trajet ou chaque zone (Honorbuddy) : on veut que le bot décide seul ;
  les profils servent seulement de recettes et de vérification.
- Courrier, guilde, discussion, champs de bataille : pas utiles au speedrun solo (les champs de bataille
  donnent de l'XP en 3.3.5, mais demandent une IA JcJ).
