# Guerrier

## Armes (Ovale, posture de combat)
1. Overpower si Taste for Blood expire dans moins de 1,5 s et que Rend manque.
2. Rend s'il manque et que la cible vit encore plus de 8 s.
3. Overpower après une esquive (sans Taste for Blood) ; Overpower si Taste for Blood expire dans moins de 4,5 s.
4. Mortal Strike si la cible a plus de 20 % de PV.
5. Execute sur Sudden Death ou si la cible a moins de 20 % de PV.
6. Overpower, Victory Rush (si utilisable), Slam si Improved Slam (2 points).
- Hors recharge globale : Heroic Strike au-delà de 94 rage (posture de combat) ; Cleave au-delà de 50 rage contre plusieurs cibles.
- Recharges : Bladestorm, Retaliation.

## Fureur (Ovale, posture berserker)
1. Shield Slam si bouclier ; Shockwave ; Concussion Blow.
2. Whirlwind, Bloodthirst.
3. Execute si la cible a moins de 20 % ; Victory Rush.
4. Bloodsurge (Slam instantané) : Slam s'il expire dans moins de 2,5 s, ou s'il reste plus de 6 s et que Bloodthirst et Whirlwind sont à moins de 1 s ; sinon Slam en priorité basse.
5. Mortal Strike, Devastate.
- Hors recharge globale : Heroic Strike au-delà de 66 rage. Recharges : Death Wish, Recklessness.

## Protection (Ovale, posture défensive)
1. Boss : Thunder Clap s'il expire dans moins de 2 s, Concussion Blow, Shockwave ; plusieurs cibles : Thunder Clap, Shockwave.
2. Revenge (si utilisable), Shield Slam, Bloodthirst, Mortal Strike, Devastate au-delà de 10 rage.
- Sunder Armor / Devastate jusqu'à 5 charges, et les maintenir.
- Hors recharge globale : Heroic Strike au-delà de 66 rage. Recharges : Shield Block, Last Stand, Shield Wall.

## Commun
- Cris : Battle Shout ou Commanding Shout rafraîchi à moins de 3 s (pas les deux du même type qu'un buff de groupe : Bénédiction de puissance).
- Demoralizing Shout sur les boss si aucun effet du même type (Demoralizing Roar, Curse of Weakness).
- WeakAura Mirash (niveau 80) : Slam seulement si Mortal Strike n'est pas prêt avant 1,6 s et rage ≥ 45 ; Mortal Strike à 30 rage ; Bladestorm même à rage basse (≤ 29) à 10 yards ou moins ; plusieurs cibles = 4 ennemis ou plus à 8 yards (Sweeping Strikes, Bladestorm, Thunder Clap) ; Charge à 9 yards ou plus.
- Aura state (Spell.dbc) : Victory Rush = état 10 (« Victorious », aura 32216) ; Execute = cible en état 2 (< 20 %) ; Revenge = état 1.
- Leveling solo : pas de Sunder Armor (sert en groupe) ; défensifs de playerbots (Intimidating Shout à PV critiques, Enraged Regeneration, Retaliation, Berserker Rage contre la peur, Bloodrage si PV ≥ 65 % et rage < 20).

Voir aussi `auto/warrior.md` (Afenar : auras suivies et seuils ; mod-playerbots : déclencheurs → actions), régénéré par `scripts/rotation_sources.py`.
