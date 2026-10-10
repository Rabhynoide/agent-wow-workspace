# Guides de leveling (données extraites par Claude Desktop)

Un fichier JSON par zone ou par plage de niveaux, dans l'ordre de jeu. Nom : `<faction>_<nn>_<zone>_<niveaux>.json`
(ex. `alliance_01_human_1-10.json`, `alliance_06_elwynn_10-13.json`). Les fichiers sont lus par ordre alphabétique :
le numéro `nn` à 2 chiffres fixe l'ordre de la route. Les guides de classe sont dans `class_<classe>_<spec>.json`.

Contrôle : `cd module && go run ./cmd/guidecheck -dir ../journal/guides` (compare chaque quête à la base du serveur).
Liste pour `auto.run` : `go run ./cmd/guidecheck -dir ../journal/guides -ids -from 5 -to 10`.

## Format d'une route
```json
{
  "source": "https://...", "faction": "Alliance", "race": "Human", "zone": "Elwynn Forest", "levels": "10-13",
  "steps": [
    { "n": 1, "action": "accept", "quest": "Kobold Candles", "quest_id": 60, "npc": "William Pestle",
      "targets": null, "count": null, "place": "Goldshire", "coords": null, "level": "10",
      "class": null, "races": null, "note": null }
  ]
}
```
`action` : accept, turnin, complete, kill, collect, use_object, talk, travel, flight_path, set_hearth, hearth, train, buy, grind, note.
`race` vaut "any" pour une route valable pour toutes les races de la faction. `class` / `races` : restriction de l'étape (quête de classe ou de race).
Tout champ absent du guide vaut null : rien n'est inventé.
