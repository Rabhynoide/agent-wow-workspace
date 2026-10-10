# BACKLOG (remarques utilisateur + problèmes connus)

Légende : [ ] à faire, [~] partiel, [x] fait.

## Remarques de l'utilisateur
- [x] Les objets de quête ne tombent pas à 100 % : vérifié en base (90 % pour les peaux de loup) ; le job `combat.kill` boucle jusqu'au compte voulu. La vraie cause de l'échec était le sac plein.
- [ ] Équiper automatiquement les objets utilisables qui améliorent le personnage (module `gear` : comparer emplacement, niveau requis, armure/stats, priorité stats prêtre ; commande d'équipement).
- [~] Build de leveling : données disponibles (`npc_trainer`/`trainer_spell` en base, `Talent.dbc`/`TalentTab.dbc`/`Spell.dbc` dans le volume du serveur) mais rien d'implémenté. A faire : choisir un build (ex. Ombre ou Discipline pour le leveling), module `train` (apprendre les sorts au niveau 4/6/8...), attribution des talents (CMSG_LEARN_TALENT), choix des sorts (Mot de pouvoir : Bouclier, Rénovation, Mot de l'ombre : Douleur...) dans `combat`.
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

## Prochaines étapes suggérées
- Entraîneur de prêtre (sorts niveau 4/6 : Bouclier, Châtiment rang 2...), talents, module `gear`.
- Quêtes suivantes : Elwynn Forest (Goldshire, quête 54 puis zone suivante), Auberge (2158).
- Tests visuels (utilisateur) de la marche.
