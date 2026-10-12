# MODULES (référence courte ; version détaillée et historique : `journal/modules_full.md`, vérifications : `journal/modules_tests.md`)

Code : `module/` (un go.mod, un binaire par module `module/cmd/<nom>`). Manifestes : `modules/<nom>/` (module.yaml, compose.yaml, module.pb). Protos : `cd module && make generate`. Images reconstruites au `char play`. Tests : `cd module && go test ./...` (utilisent `assets/` s'il existe).
Appels : `scripts/rpc.sh <module.methode> '<json>' [chemin]` ; jobs async : `scripts/job.sh <module.methode> '<json>' [max_s]` (une ligne par étape, chaque événement `progress` une fois). Logs : `logs/<module>.log`.
`assets/` (hors git) : mmaps, maps, vmaps, dbc (Spell, SpellRange, Talent, TalentTab, SkillLineAbility, Lock, FactionTemplate), zygor/repo, routes, sources (Ovale, WeakAuras, playerbots).

## Outils hors jeu
- `zygor2route` : guides Zygor (`assets/zygor/repo`, jamais dans git) → `assets/routes/*.json` (25 routes Alliance, avec ids et variantes race/classe).
- `guidecheck <fichiers>` : valide une route contre la base ; `-plan -char -race -class -level -n` : prochaines quêtes ; `-manual` : quêtes qu'`auto` ne sait pas faire (liste : `journal/quests_manual_human_warrior.md`) ; `-ids`.
- `guidecheck -score map,x,y -level N -class warrior` : classement des monstres autour par XP/minute.
- `xpscore` : XP/min = XP_kill / (T_kill + T_repos + T_recherche + P_mort × 150 s). Monstre depuis la base (niveaux, HealthModifier, DamageModifier, BaseAttackTime, ExperienceModifier, apparitions, respawn, voisins hostiles à 15 yards + errance). Nous : `journal/learn/fights.json`, appris des lignes `fight` (combats gagnés seul, commencés ≥ 90 % de PV) par classe et écart de niveau ; écart non mesuré extrapolé (×1,85 PV perdus, ×1,15 durée par niveau).
- `classcheck journal/classes/<classe>.yaml` : valide talents, sorts, quêtes de classe.
- `scripts/level.sh <Nom> <niveau> [min=180]` (= `guidecheck -play`) : suit la route Zygor dans l'ordre (`auto.run` par lots de 5) ; respecte les étapes « reach this level N » ; quête trop haute → `auto.grind` d'un niveau sur les 4 meilleurs monstres autour en XP/minute (`xpscore`, risque de mort ≤ 10 % ; candidats : non élites, attaquables, pas gris, ≥ 3 apparitions, hors événements ; classement dans le log de level) ; quête dont les monstres sont morts → reportée 4 min (3 essais) ; saute les quêtes manuelles, rendues, interdites. Arrêt : niveau atteint (0), 3 morts ou 4 tours sans progrès (1), temps (2). Détail : `logs/level_<Nom>.log`.
- `scripts/bench.sh [niveau=6] [min=60] [race] [classe]` : personnage neuf, `level.sh`, rapport `journal/bench/<date>_<nom>.md` + `results.csv`, personnage supprimé.
- `scripts/rotation_sources.py` : résumés `journal/rotations/auto/` depuis `assets/sources`.

## state (paquets → état)
- `state.brief {}` : self (pos, map, level, hp, power, xp, money, target, dead/ghost, class_id, race, talent_points, unit_flags, aura_state, auras), quêtes (id, state, counts), free_slots, durability_pct. À préférer.
- `state.snapshot {}` : + objets portés (bag/bag_slot pour les paquets, durability/max_durability).
- `state.nearby {kind, entry, radius, max(5), alive_only, npc_flags, hostile_only, with_auras}` : par distance ; `reaction` hostile/neutral/friendly (FactionTemplate.dbc), aura_state.
- `state.get_obj {guid}` (avec auras, aura_state, reaction), `state.spells {}`, `state.set_pos`.
- Auras : SMSG_AURA_UPDATE(_ALL) de toute unité visible (sort, charges, temps restant, `mine`, `positive`) ; UNIT_FIELD_AURASTATE = champ 61, bit n-1 = état n (10 Victory Rush sur nous, 2 « < 20 % » sur la cible).
- Répond au serveur comme le client : MSG_MOVE_TELEPORT_ACK, MSG_MOVE_WORLDPORT_ACK, vitesses forcées, enracinement (sans eux le serveur ignorait nos mouvements après une mort).
- Limites : positions des monstres = destination de spline ; pas de chat ni de journal de combat complet.

## world (MySQL acore_world, lecture seule)
- `world.find_npc {name|entry, map_filter, map, x, y, radius, npc_flags, sells_spell_category, sells_max_level, max}` (ignore les « [DND] »). npc_flags : vendeur 128, réparateur 4096, aubergiste 65536.
- `world.quest_info {id|title}` : niveaux, objectifs (kill/item/gameobject, `counter`), `explore` (zones), `credit_from` (KillCredit), `use_item/use_spell/use_on` (crédit par sort), starters/enders (créature ou objet), reward_choices, special_flags.
- `world.spawns {entry, map, x, y, radius, kind}` (avec rank), `world.item_info {entry|name}` (stats, dps, use_spell, spell_category, stackable), `world.item_sources {entry}`, `world.quests_for_npc`, `world.creature`, `world.class_trainers`.

## quest
- `quest.accept`/`quest.turnin {npc_entry, quest_id, reward}` : PNJ (ou objet) à ≤ 5 yards 3D.
- `quest.use {entry, loot}` (objet du décor, sort 22810 si verrouillé), `quest.explore {trigger}` (CMSG_AREATRIGGER), `quest.use_item {item_entry, spell, target_entry, target_kind}` (CMSG_USE_ITEM, cible ≤ 30 yards).

## combat (jobs)
- `combat.kill {entry | guid, count, item_entry, item_count, radius(80), max_s, no_loot, max_pull, body_pull}`, `combat.defend`, `combat.rest`, `combat.status`, `combat.stop`.
- Choix de cible : adds = hostiles inoccupés dont l'aggro (20 yards ±1/niveau, 5-45, ≥ 10) atteint la cible ; charge = cible (≤ 1) + adds pondérés par niveau ; on prend un candidat dont la charge tient dans la capacité (`max_pull`, 1 / 1,5 dès 20 / 2 dès 40), les autres sont laissés 2 min. `body_pull` expérimental (désactivé). Cible inaccessible : abandonnée (4 d'affilée max) ; sans dégâts 25 s : abandonnée.
- « Peut lancer » (Spell.dbc + SpellRange.dbc) : recharges comptées côté client (sort, catégorie, globale) + SMSG_SPELL_COOLDOWN, coût, aura states, portée ; sorts à arme bloqués pendant un désarmement (UNIT_FLAG_DISARMED) ou si l'arme est absente/cassée (revérifiée chaque minute), sinon 3 s après un refus code 30.
- Guerrier : liste ordonnée : Battle Shout (manque ou < 3 s), Victory Rush (aura state), Execute (cible < 20 % ou Sudden Death), Overpower (esquive ou Taste for Blood), Rend (manque ou < 2 s, cible > 50 %), Mortal Strike, Slam (41+), Thunder Clap (< 20) ; Charge à l'ouverture ; Heroic Strike au-delà de 30 rage (60 dès 41). Repos à 75 %.
- Prêtre : soin < 40 %, Bouclier (sans Âme affaiblie) < 80 %, Rénovation < 65 %, Douleur et Attaque mentale vérifiées, Châtiment, baguette sans mana ; ligne de vue avant les sorts. Robustesse/Feu intérieur s'ils durent < 1 min. Repos à 55 %.
- Journal : une ligne `fight` par combat (résultat, cible, PV début/min/fin, attaquants max et leurs entrées, durée) ; résumé des combats perdus dans le rapport de bench.
- Repos : mange (catégorie 11) sous 80 % de PV, boit (59) sous 70 % de mana.
- Limites : rotations prêtre/guerrier seulement ; pas de fuite ; rotation pas encore en table de priorités.

## town
- `town.sell_junk {dry_run, min_free, show_kept}` (règles `internal/sellrule`), `town.repair {}`, `town.restock {food, drink, max_copper}` (meilleure nourriture/boisson de notre niveau, budget = 1/3 de l'argent) : PNJ à ≤ 5 yards.

## auto (orchestrateur, jobs)
- `auto.run {quests, rewards, no_progress}` : par quête : donneur, objectifs (kill, objets, objets du décor, crédits par d'autres créatures, objet utilisé sur une cible, zone à explorer), rendu (place dans les sacs d'abord) ; entre les quêtes : équipement, talents, entraîneur à < 400 yards.
- Statut des jobs : `deaths` = morts dont le job s'est relevé (level.sh les compte toutes).
- `auto.grind {entries, until_level, max_s}` (points d'apparition triés distance + 40 yards par voisin à < 20 yards ; créature qui ne vit qu'en camps de 4+ sautée), `auto.goto {x, y, z, radius}` (marche qui riposte), `auto.recover`, `auto.status`, `auto.stop`.
- Marches : riposte si un monstre nous cible (20 combats max par marche). Zones à explorer et objets du décor : nettoyage progressif du chemin (tronçons de 25 yards, monstres dont l'aggro touche le tronçon tués un par un, repos < 70 %, « path blocked » si trop fort ; gardien au-dessus de notre niveau → « too dangerous », quête reportée au niveau suivant).
- Marchand : sacs presque pleins, durabilité < 60 % ou moins de 5 aliments → vente, réparation (avant la nourriture), achat de nourriture ; après un échec faute d'argent, pas de nouvel essai avant d'avoir gagné 1 pa.
- Résurrection : toujours au corps, au point le plus sûr à < 37 yards (le serveur accepte 39) (attente si un monstre est à < 25 yards), puis repos ; esprit guérisseur seulement en secours avant le niveau 11.
- Danger : monstres élites ou au-dessus de niveau + 1 (+ 2 pour prêtre/mage/démoniste) refusés. Garde : entre deux jobs, défense si attaqué.
- Limites : une carte à la fois, pas de voyages (vols, bateaux), pas de quête de groupe/élite.

## progress
- `progress.train_check`, `progress.train` (job : entraîneur, sorts abordables), `progress.talents {dry_run}` (ordre de `journal/classes/<classe>.yaml`, pas de respec), `progress.gear {dry_run}` (profils de stats par classe), `progress.pick_reward {items}`.

## move
- `move.goto {x, y, z, radius(1.5), walk}` (navmesh au sol, chemin lissé sur la surface, hauteur du sol comme le serveur ; ne se défend PAS : utiliser `auto.goto`), `move.status`, `move.stop`, `move.path {x, y, z, with_points}`, `move.los {x, y, z}` (vmaps).
- `move.safe_point {x, y, z, max_dist(30), min_dist, path_clear}` : point le plus loin des monstres hostiles, atteignable à pied.
- Déblocage : départ sur un îlot de navmesh (< 300 polygones) ou hors navmesh → marche droite au sol jusqu'à un point d'où un chemin existe.
- Limites : pas de nage, de vol, de transports ni d'évitement des monstres dans le chemin.
