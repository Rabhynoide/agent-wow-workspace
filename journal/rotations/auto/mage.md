# mage — résumé automatique des sources (scripts/rotation_sources.py)

## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille
- **Power_Infusion_Mage** (Mage_Dynamic): aura 10060 on player
- **Tricks_Mage** (Mage_Dynamic): aura 57934 on player
- **Clearcasting** (Mage_Dynamic): aura 12536 on player
- **Winter's Chill** (Mage_Dynamic): aura 11180 on target
- **Improved Scorch** (Mage_Dynamic): aura 22959,17800 on target
- **Firestarter** (Mage_Dynamic): aura 54741 on player
- **Burning Determination** (Mage_Dynamic): aura 54748 on player
- **Impact** (Mage_Dynamic): aura 64343 on player
- **Blazing Speed** (Mage_Dynamic): aura 57092 on player
- **Mana Surge** (Mage_Dynamic): aura 37445 on player
- **Praxis** (Mage_Dynamic): aura 64868 on player
- **Pushing the Limit** (Mage_Dynamic): aura 70753 on player
- **Quad Core** (Mage_Dynamic): aura 70747 on player
- **Brain Freeze** (Mage_Dynamic): aura 57761 on player
- **Arcane_Barage 2** (Mage_Dynamic): spell Arcane Barrage (Cooldown Progress (Spell))
- **Cone of Cold** (Mage_Dynamic): spell Cone of Cold (Cooldown Progress (Spell))
- **Fire_Blast** (Mage_Dynamic): spell Fire Blast (Cooldown Progress (Spell))
- **Freeze** (Mage_Dynamic): spell Freeze (Cooldown Progress (Spell))
- **Mana Shield** (Mage_Dynamic): aura 1463 on player
- **Armors** (Mage_Dynamic): aura 168,7302,30482,6117 on player (missing)
- **Arcane_Intellect** (Mage_Dynamic): aura 1459,27127,46302 on player (missing)
- **Blast_Wave** (Mage_Core): spell Blast Wave (Cooldown Progress (Spell))
- **Living_Bomb** (Mage_Core): aura 55359 on target — expirationTime < 2 -> glow; incombat < 0 -> glow
- **Presence_of_Mind** (Mage_Core): aura 12043 on player; spell Presence of Mind (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Missile Barrage** (Mage_Core): aura 44401 on player
- **Arcane Blast** (Mage_Core): aura 36032 on player
- **Arcane_Barage** (Mage_Core): spell Arcane Barrage (Cooldown Progress (Spell))
- **Summon_Water_Elemental** (Mage_Core): spell Summon Water Elemental (Cooldown Progress (Spell))
- **Cone_of_Cold 2** (Mage_Core): spell Cone of Cold (Cooldown Progress (Spell))
- **Fingers_of_Frost** (Mage_Core): aura 44544 on player; spell Ice Lance (Cooldown Progress (Spell))
- **Pyroblast** (Mage_Core): aura 48108 on player; spell Pyroblast (Cooldown Progress (Spell))
- **Dragon_Breath** (Mage_Core): spell Dragon's Breath (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown == 1 -> glow
- **Frost Nova** (Mage_Core): aura 122 on target; spell Frost Nova (Cooldown Progress (Spell))
- **Combustion** (Mage_Core): aura 11129 on player; spell Combustion (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Icy_Veins** (Mage_Core): aura 12472,198144 on player; spell Icy Veins (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Arcane_Power** (Mage_Core): aura 12042 on player; spell Arcane Power (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Evocation** (Mage_Utility): spell Evocation (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Conterspell** (Mage_Utility): spell Counterspell (Cooldown Progress (Spell))
- **Frost_Nova_Fire&Arcane** (Mage_Utility): spell Frost Nova (Cooldown Progress (Spell))
- **Ice Barrier** (Mage_Utility): aura 11426 on player; spell Ice Barrier (Cooldown Progress (Spell))
- **Wards** (Mage_Utility): aura 543,6143 on player; spell Fire Ward (Cooldown Progress (Spell))
- **Deep_Freeze** (Mage_Utility): aura 44572 on target; spell Deep Freeze (Cooldown Progress (Spell))
- **Blink_182** (Mage_Utility): spell Blink (Cooldown Progress (Spell))
- **Invisibility** (Mage_Utility): aura 32612 on player; spell Invisibility (Cooldown Progress (Spell))
- **Ice_Block** (Mage_Utility): aura 45438 on player; aura 41425 on player; spell Ice Block (Cooldown Progress (Spell))
- **Mirror_Image** (Mage_Utility): spell Mirror Image (Cooldown Progress (Spell))
- **Cold_Snap** (Mage_Utility): spell Cold Snap (Cooldown Progress (Spell))
- **BloodLust_Mage** (Mage_Utility): aura 32182,2825 on player
- **Amplify & Dampen Magic** (Mage_Utility): aura 604,1008 on player
- **Ritual of Refreshment** (Mage_Utility): spell Ritual of Refreshment (Cooldown Progress (Spell))
- **Racial_Traits_Mage** (Mage_Utility): spell Gift of the Naaru (Cooldown Progress (Spell)); spell Stoneform (Cooldown Progress (Spell)); spell Escape Artist (Cooldown Progress (Spell)); spell Arcane Torrent (Cooldown Progress (Spell))
- **Saves_Mage** (Mage_Utility): aura 33206,6940,1022,15286,47788,31821 on player

## mod-playerbots : déclencheur → actions (priorité)

### ArcaneMageStrategy

### FireMageStrategy
- `enemy too close for spell` → dragon's breath (ACTION_INTERRUPT+1)
- `enemy is close` → blast wave (ACTION_INTERRUPT)

### FrostFireMageStrategy
- `enemy too close for spell` → dragon's breath (ACTION_INTERRUPT+1)
- `enemy is close` → blast wave (ACTION_INTERRUPT)

### FrostMageStrategy

### GenericMageNonCombatStrategy

### GenericMageStrategy
