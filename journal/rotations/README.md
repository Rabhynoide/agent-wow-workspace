# Rotations par classe et spécialisation (référence pour le jour où on les code)

Notes écrites à partir de quatre sources (2026-10-11). Les sources brutes sont gardées en local dans
`assets/sources/` (jamais commité, droits des auteurs) :
- `ovale/` : addon Ovale Spell Priority r261 (Interface 30300 = 3.3, notre version) ; `defaut/<Classe>.lua` = script de priorités de chaque classe (noms de fichiers en français).
- `wago/afenar_<classe>_<id>.json` : WeakAuras Afenar WotLK décodés (tableaux de bord : procs, buffs, DoT, recharges).
- `wago/mirash_warrior_X0AepQIWE.json` : WeakAura de priorité guerrier (toutes spés, niveau 80).
- `playerbots_class/<Classe>/` : IA de classe de mod-playerbots (stratégies déclencheur → action).

Fichiers : un `<classe>.md` par classe (priorités Ovale reformulées par spécialisation + remarques
pour le leveling solo) et `auto/<classe>.md` (résumé automatique d'Afenar et de playerbots).
Si `assets/sources/` a disparu : wago.io (`data.wago.io/lookup/wago?id=<id>` puis l'URL `codeURL`),
github.com/liyunfan1223/mod-playerbots (`src/Ai/Class`), et redemander le zip Ovale à l'utilisateur.

Lecture des conditions Ovale :
- « expire dans < n s » : rafraîchir avant la fin (le temps d'incantation compte pour les sorts à incantation) ;
- « la cible vit > n s » : temps avant la mort estimé d'après la baisse des PV (ne pas poser de DoT sur une cible qui meurt) ;
- « mine » : seulement nos propres débuffs ;
- « usable » : le serveur l'autorise (aura state, proc) ;
- « 1 s avant X » : X sera prêt dans moins d'une seconde.

Ce qui manque au code pour appliquer tout ça : voir BACKLOG « Rotation et combat » (suivi des auras, aura state, « peut lancer », table de priorités).
