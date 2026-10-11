# hunter — résumé automatique des sources (scripts/rotation_sources.py)

## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille
- **Hunter's Mark_Nodebuff** (Hunter_Dynamic): aura 1130 on target (missing)
- **Unholy_Frenzy_Hunter** (Hunter_Dynamic): aura 49016 on player
- **Power_Infusion_Hunter** (Hunter_Dynamic): aura 10060 on player
- **Tricks_Warlock_Hunter** (Hunter_Dynamic): aura 57934 on player
- **Exploit Weakness** (Hunter_Dynamic): aura 70728 on player
- **Stinger** (Hunter_Dynamic): aura 71007 on player
- **Precision Shots** (Hunter_Dynamic): aura 64861 on player
- **Aimed Shot Debuff** (Hunter_Dynamic): aura 49050 on target
- **Expose Weakness** (Hunter_Dynamic): aura 34501 on player
- **Improved Steady Shot** (Hunter_Dynamic): aura 53220 on player
- **Quick Shots** (Hunter_Dynamic): aura 6150 on player
- **Rapid Killing** (Hunter_Dynamic): aura 35098 on player
- **Sniper_Traning** (Hunter_Dynamic): aura 64420 on player — expirationTime < 2 -> glow
- **Arcane_Shot** (Hunter_Core): spell Arcane Shot (Cooldown Progress (Spell)); spell Arcane Shot (Spell Known); aura 56453 on player — expirationTime < 1 -> glow; onCooldown == 1 -> grey
- **Chimaera_Shot** (Hunter_Core): spell Chimera Shot (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Black_Arrow** (Hunter_Core): aura 3674 on target; spell Black Arrow (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Aimed_Shot** (Hunter_Core): spell Aimed Shot (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown == 1 -> grey
- **Explosive_Shot** (Hunter_Core): spell Explosive Shot (Cooldown Progress (Spell)); aura 56453 on player — expirationTime < 1 -> glow; onCooldown == 1 -> grey
- **Multishot** (Hunter_Core): spell Multi-Shot (Cooldown Progress (Spell)); spell Multi-Shot (Spell Known) — expirationTime < 1 -> glow; onCooldown == 1 -> grey
- **Stings ** (Hunter_Core): aura 27016,3043,3034,19386 on target; spell Devouring Plague (Cooldown Progress (Spell)) — expirationTime <= 2 -> glow
- **Kill_Shot** (Hunter_Core): spell Kill Shot (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown < 1 -> glow; onCooldown == 1 -> grey
- **Kill_Command_BM** (Hunter_Core): aura 58914 on pet; spell Kill Command (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Kill_Command** (Hunter_Utility): aura 58914 on pet; spell Kill Command (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Bestial_Wrath** (Hunter_Utility): aura 19574 on pet; spell Bestial Wrath (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Rapid_Fire** (Hunter_Utility): aura 3045 on player; spell Rapid Fire (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Readiness ** (Hunter_Utility): spell Readiness (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown < 1 -> glow
- **Silencing_Shot** (Hunter_Utility): spell Silencing Shot (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown < 1 -> glow
- **Scatter_Shot** (Hunter_Utility): spell Scatter Shot (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown < 1 -> glow
- **Intimidation** (Hunter_Utility): spell Intimidation (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown < 1 -> glow
- **Wyvern_Sting** (Hunter_Utility): spell Wyvern Sting (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown < 1 -> glow
- **Disengage** (Hunter_Utility): spell Disengage (Cooldown Progress (Spell))
- **Deterrence ** (Hunter_Utility): aura 19263 on player; spell Deterrence (Cooldown Progress (Spell))
- **Master's Call** (Hunter_Utility): aura 62305 on pet; spell Master's Call (Cooldown Progress (Spell))
- **Cower** (Hunter_Utility): aura 1742 on pet; spell Cower (Cooldown Progress (Spell))
- **Last Stand** (Hunter_Utility): aura 53478 on pet; spell Last Stand (Cooldown Progress (Spell))
- **Call of the Wild** (Hunter_Utility): aura 53434 on pet; spell Call of the Wild (Cooldown Progress (Spell))
- **Lick Your Wounds** (Hunter_Utility): aura 53426 on pet; spell Lick Your Wounds (Cooldown Progress (Spell))
- **Heart of the Phoenix** (Hunter_Utility): spell Heart of the Phoenix (Cooldown Progress (Spell))
- **Roar of Sacrifice** (Hunter_Utility): aura 53480 on group; spell Roar of Sacrifice (Cooldown Progress (Spell))
- **Intervene - Pet** (Hunter_Utility): aura 53476 on group; spell Intervene (Cooldown Progress (Spell))
- **Bullheaded** (Hunter_Utility): aura 63896 on pet; spell Bullheaded (Cooldown Progress (Spell))
- **Misdirect** (Hunter_Utility): aura 35079 on player; spell Misdirection (Cooldown Progress (Spell))
- **Faint_Death** (Hunter_Utility): aura 5384 on player; spell Feign Death (Cooldown Progress (Spell))
- **Tranquilizing_Shot** (Hunter_Utility): spell Tranquilizing Shot (Cooldown Progress (Spell))
- **Distracting_Shot** (Hunter_Utility): spell Distracting Shot (Cooldown Progress (Spell))
- **CS** (Hunter_Utility): spell Concussive Shot (Cooldown Progress (Spell))
- **Frost_Trap** (Hunter_Utility): spell Frost Trap (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Fire_Trap** (Hunter_Utility): spell Explosive Trap (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Snake_Trap** (Hunter_Utility): spell Snake Trap (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Aspects** (Hunter_Utility): aura 13165,13161,5118,13159,13163,34074,20043,61846 on player
- **Trueshot_Aura** (Hunter_Utility): aura 19506 on player (missing)
- **LS** (Hunter_Utility): spell Flare (Cooldown Progress (Spell))
- **136** (Hunter_Utility): aura 136 on pet
- **Feed_Pet** (Hunter_Utility): spell Feed Pet (Cooldown Progress (Spell))
- **Hunter's Mark** (Hunter_Utility): aura 1130 on target
- **Racial_Traits_Hunter** (Hunter_Utility): spell Gift of the Naaru (Cooldown Progress (Spell)); spell Stoneform (Cooldown Progress (Spell)); spell Escape Artist (Cooldown Progress (Spell)); spell Arcane Torrent (Cooldown Progress (Spell))
- **BloodLust_Hunter** (Hunter_Utility): aura 264667,90355,80353,32182,2825,292686,230935,256740,178207 on player
- **Saves_Hunter** (Hunter_Utility): aura 33206,97462,6940,1022,204018,15286,47788,31821,102342,116849,77764 on player

## mod-playerbots : déclencheur → actions (priorité)

### BeastMasteryHunterStrategy

### GenericHunterNonCombatStrategy
- `low ammo` → say::low ammo (ACTION_NORMAL)
- `no track` → track humanoids (ACTION_NORMAL)
- `no ammo` → equip upgrades packet action (ACTION_HIGH+1)

### GenericHunterStrategy

### HunterBuffStrategies
- `aspect of the dragonhawk` → aspect of the dragonhawk (ACTION_HIGH)
- `aspect of the wild` → aspect of the wild (ACTION_HIGH)
- `aspect of the pack` → aspect of the pack (ACTION_HIGH)

### MarksmanshipHunterStrategy

### SurvivalHunterStrategy
