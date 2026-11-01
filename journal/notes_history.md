# NOTES : historique (archivé depuis NOTES.md le 2026-11-01)

États de personnages anciens, conseils de modèle, pièges déjà corrigés. À chercher, pas à lire en entier.

## Etat du personnage : Claudio (guerrier humain, GUID 3)
- Niveau 11 (2026-10-11), ~4 pa, vivant, PV pleins, DURABILITÉ 30 % (réparer au prochain marchand : `auto` le fait), près du camp de Defias au nord-est du lac (-9312,-223), session ouverte. Talents : Improved Rend 2/2. 3 morts pendant les tests du pull (envoyé avec `move.goto` qui ne se défend pas).
- Fait : vallée de Northshire, 54, 2158, 106, 111, 107, 86, 84, 62, 76, 60, 47, 61, 88, 40, 35, 112, 114, 37 (+ précédentes). Reste à refaire : 87 Goldtooth (mine de Fargodeep, sautée pour trop de combats en route ; limite relevée depuis).
- Continuer : `scripts/start.sh Claudio` (si besoin) puis `scripts/level.sh Claudio <niveau>` en arrière-plan. `auto.recover` ressuscite au corps (esprit guérisseur seulement en secours avant le niveau 11). Quête de classe du niveau 10 (1638, posture défensive) toujours à faire.

## Etat du personnage : Claudia (fin de session 2026-10-10)
- Humain prêtre (guid 1), niveau 6, XP 1818/3600, 29 cuivres, 171 PV max, équipement réparé à 100 %. Sorts : Châtiment 2, Soins inférieurs 2, Bouclier, Mot de l'ombre : Douleur, Robustesse. Position : Elwynn, sud-ouest de Goldshire (~-9540,67), map 0. Session fermée.
- Quêtes faites : Northshire, 54 « Report to Goldshire », 2158 « Rest and Relaxation », chaîne Maclure (85, 86, 106, 84, 111, 107). Dans le journal : 60 (Kobold Candles) — NE PAS la faire avant 8+ (kobolds en groupes dans la mine).
- Suite : `guidecheck -plan -char Claudia -race Human -class Priest -level 6` (MODULES « zygor2route »). Le plan propose encore 60, 47, 87 (mine de Fargodeep) : les écarter tant que l'évitement des groupes de monstres n'existe pas.
- Prochains sorts au niveau 8 (Priestess Josetta 377 à Goldshire, -9461,33 ; ou le « Priest Trainer » 26328 tout près). Talents automatiques dès le niveau 10 (`journal/classes/priest.yaml`).
- Chasse utile à son niveau (~35 XP/kill) : Loup galeux 525, Sanglier 113, Araignée de forêt 30, Coupe-bourse défias 94, autour de Goldshire : `scripts/grind.sh 7 113 94 30 525`.
- Marchands de Goldshire : Brog Hamfist 151 (-9465,10,57), Farley 295 ; réparateurs : Andrew Krighton 2046 (-9462,88,58), Corina Steele 54.
- NE PAS aller dans la mine de Fargodeep (-9741,114) : kobolds 5-7 en groupes + élite niveau 10 (3 morts le 2026-10-10).

## Choix du modèle (à suggérer à l'utilisateur, en une ligne, avant de commencer la tâche)
Je ne peux pas changer de modèle moi-même : je propose `/model <nom>` quand le modèle courant ne colle pas à la tâche, puis je continue avec celui que l'utilisateur garde. Changer via `/model` dans la même session (le contexte est conservé).
- Sonnet : jouer (start.sh, job.sh auto.run, choix de quêtes, suivi), dev simple (RPC calqués sur l'existant, SQL world, scripts, doc, tests), mise à jour NOTES/MODULES/BACKLOG.
- Opus : code protocole (décodage/envoi de paquets, update fields, navmesh, terrain, déplacement), debug d'un comportement incompris après 2 tentatives infructueuses, revue avant push.
- Revue Opus : sur le diff seulement (`git diff origin/claudia-modules...HEAD` ou `/code-review`), par lot de fonctionnalités, en ciblant les points « à vérifier » de BACKLOG.
- En Sonnet : noter dans BACKLOG les doutes de protocole (« à vérifier : … ») et écrire des tests (paquets réels de `logs/state_bad_N.hex`) pour alléger la revue.

- (corrigé le 2026-10-11) Après une mort, le serveur téléporte le fantôme au cimetière et attend MSG_MOVE_TELEPORT_ACK ; sans réponse, il ignorait tous nos mouvements : le corps se réclamait (le serveur nous croyait encore là) mais ensuite le personnage « marchait » seul de notre côté, ne voyait plus aucun PNJ (« not visible within 40 yards »), et à la reconnexion revenait au cimetière. `state` répond maintenant. Symptôme si ça revient : `state.nearby` vide dans une zone peuplée, ou vitesse 10,5 (fantôme) après résurrection : grep « teleported » dans logs/state.log.
- (corrigé le 2026-10-20) Créatures techniques des événements : la Sanssaint (Hallow's End, ~18 oct.-1er nov.) met 35 « Headless Horseman Flame Bunny » (23686, niveau 70, faction hostile) dans Goldshire : le bot les prenait pour des monstres (chemins vers la mine bloqués, quêtes 47/60/61/62 reportées, benchs 6 → 8 de 54-65 min au lieu de 46 du 19 au 20 oct.). `state` ne leur donne plus de réaction (UNIT_FLAG_NOT_SELECTABLE). Avant de conclure à une régression, vérifier les événements actifs : `scripts/soap.sh ".event activelist"` (Darkmoon, fêtes, « Evening »…).
