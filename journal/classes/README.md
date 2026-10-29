# Base de leveling WoW 3.3.5a (AzerothCore)

Un fichier YAML par classe (`warrior.yaml`, …), extrait des guides Icy Veins WotLK Classic.

- `recommended_leveling_spec` / `alternatives` : spec à jouer et options viables.
- `talent_order` : dépenser les points dans l'ordre de la liste ; `levels` = niveaux concernés, `tree` = arbre du client 3.3.5a ; `removed_at: N` = points retirés au respec du niveau N.
- `respec_notes` : réinitialisation de talents à faire à un niveau donné.
- `key_abilities` : sorts clés, `level` = niveau d'apprentissage (`null` = non précisé par la source).
- `rotation_solo` : priorité en combat, de haut en bas ; `(N+)` = valable à partir du niveau N.
- `downtime`, `gear_notes`, `pitfalls` : règles de gestion hors combat, équipement, erreurs à éviter.
- Champs optionnels selon la classe : `glyphs`, `class_quests` (avec ID de quête), `training` (sorts à monter, ou à ne pas acheter).
- `null` = info absente ou ambiguë dans la source. `classic_only: true` = peut différer en 3.3.5a, à vérifier en jeu.
- Noms de sorts/talents en anglais, tels qu'affichés par le client 3.3.5a.

## Tables de priorités (`priorities`, `buffs`)
Lues par le module combat (modèle Ovale). `priorities` en combat, `buffs` hors combat avant un pull : la première ligne
dont la condition `if` passe et dont le sort est lançable (recharge, coût, aura state, portée, arme, d'après Spell.dbc)
est lancée. Champs : `spell` (nom anglais, rangs dans `module/internal/<classe>`), `target: self` (sinon la cible ; `target: other` : le premier autre attaquant devant nous que la condition, lue avec ses variables `target_*`, et le sort acceptent, `target_dies_in` 99),
`every` (secondes entre deux essais), `cast` (secondes d'incantation, défaut : recharge globale), `after` (attente après
le sort : la course de Charge), `reactive` (sort sur événement, les refus sont normaux), `off_gcd` (Heroic Strike :
envoyé sans attendre).
Conditions : `and`, `or`, `not`, `< <= > >= == !=`, parenthèses. Variables : `hp`, `mana` (%), `rage`, `level`,
`in_combat`, `attackers` (unités qui nous ciblent à 30 yards), `near_attackers` (les mêmes à 8 yards : portée de Thunder Clap et Cleave), `fight_s`, `dist`, `visible`, `target_hp` (%),
`target_level`, `target_on_me`, `target_dies_in` (s, d'après la baisse de ses PV ; 99 sans mesure). Fonctions :
`aura("X")` (sur nous), `my_aura_left("X")` (s, la nôtre ; 0 si absente), `target_aura("X")`, `target_aura_left("X")`,
`known("X")`, `dodged(s)`, `aura_state(n)`, `target_aura_state(n)`. Validées par le test de `combat` (`go test ./internal/combat`).

## Validation (workspace)
`cd module && go run ./cmd/classcheck ../journal/classes/<classe>.yaml` : rejoue `talent_order` niveau par niveau avec les règles
du serveur (1 point par niveau dès 10, 5 points par palier dans l'arbre, prérequis, rangs max, `removed_at`), vérifie l'arbre
de chaque talent et tous les noms de sorts dans les données du client (`assets/dbc`), les quêtes de classe dans la base
(identifiant, titre, race, classe, niveau), et donne le niveau d'apprentissage réel des sorts (`level: null` complété).
- warrior.yaml (2026-10-10) : 0 problème, 71 points au niveau 80.
