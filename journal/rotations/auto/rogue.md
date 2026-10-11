# rogue — résumé automatique des sources (scripts/rotation_sources.py)

## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille
- **Unholy_Frenzy_Rogue** (Rogue_Up): aura 49016 on player
- **Power_Infusion_Rogue** (Rogue_Up): aura 10060 on player
- **Tricks_Rogue** (Rogue_Up): aura 57934 on player
- **Remorseless Attacks** (Rogue_Up): aura 14143,14149 on player
- **Master of Subtlety** (Rogue_Up): aura 31665 on player
- **Overkill** (Rogue_Up): aura 58427 on player
- **Clearcasting - Rogue** (Rogue_Up): aura 67210 on player
- **Garrote** (Rogue_Up): aura 703 on target
- **Stealth** (Rogue_Up): aura Stealth on player; spell Stealth (Cooldown Progress (Spell))
- **Shadowstep** (Rogue_Core): aura 36554,36563 on player; spell Shadowstep (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Blade_Flurry** (Rogue_Core): aura 13877 on player; spell Blade Flurry (Cooldown Progress (Spell)); spell 193315 (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Rapture ** (Rogue_Core): aura 1943 on target; spell Rupture (Cooldown Progress (Spell)) — expirationTime < 3 -> glow
- **Slice_and_Dice** (Rogue_Core): aura 5171 on player — expirationTime < 10.8 -> glow
- **Envenom** (Rogue_Core): aura 32645 on player; aura 11354 on player
- **Expose Armor** (Rogue_Core): aura 8647,7386 on target — matchCount == 0 -> grey
- **Hunger For Blood** (Rogue_Core): aura 63848 on player
- **Shadow_Dance** (Rogue_Core): aura 51713 on player; spell Shadow Dance (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown == 1 -> grey
- **Killing_Spree** (Rogue_Core): aura 51690 on player; spell Killing Spree (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown == 1 -> grey
- **Ghostly_Strike** (Rogue_Utility): aura 14278 on target; spell Ghostly Strike (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Adrenalin_Rush** (Rogue_Utility): aura 13750 on player; spell Adrenaline Rush (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Cold Blood** (Rogue_Utility): aura 14177 on player; spell Cold Blood (Cooldown Progress (Spell))
- **Premeditation** (Rogue_Utility): spell Premeditation (Cooldown Progress (Spell))
- **Preparation** (Rogue_Utility): spell Preparation (Cooldown Progress (Spell))
- **Kick** (Rogue_Utility): spell Kick (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Kidney_Shot** (Rogue_Utility): aura 8643 on player; spell Kidney Shot (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Gouge** (Rogue_Utility): spell Gouge (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Vanish** (Rogue_Utility): aura 11327 on player; spell Vanish (Cooldown Progress (Spell))
- **Feint** (Rogue_Utility): aura 1966 on player; spell Feint (Cooldown Progress (Spell))
- **Cloak_of_Shadows** (Rogue_Utility): aura 31224 on player; spell Cloak of Shadows (Cooldown Progress (Spell))
- **Evasion** (Rogue_Utility): aura 5277 on player; spell Evasion (Cooldown Progress (Spell))
- **Dismantle** (Rogue_Utility): aura 51722 on target; spell Dismantle (Cooldown Progress (Spell))
- **Riposte** (Rogue_Utility): aura 14251 on player; spell Riposte (Cooldown Progress (Spell))
- **Blind** (Rogue_Utility): spell Blind (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Sprint** (Rogue_Utility): aura 2983 on player; spell Sprint (Cooldown Progress (Spell))
- **Trick_of_Trade** (Rogue_Utility): aura 57934 on player; spell Tricks of the Trade (Cooldown Progress (Spell))
- **Distract** (Rogue_Utility): spell Distract (Cooldown Progress (Spell)) — expirationTime < 1 -> grey
- **Cheat Death** (Rogue_Utility): aura 45182 on player
- **Racial_Traits_Rogue** (Rogue_Utility): spell Gift of the Naaru (Cooldown Progress (Spell)); spell Stoneform (Cooldown Progress (Spell)); spell Escape Artist (Cooldown Progress (Spell)); spell Arcane Torrent (Cooldown Progress (Spell))
- **BloodLust_Rogue** (Rogue_Utility): aura 32182,2825 on player
- **Saves_Rogue** (Rogue_Utility): aura 33206,6940,1022,15286,47788,31821 on player

## mod-playerbots : déclencheur → actions (priorité)

### AssassinationRogueStrategy
- par défaut : melee
- `high energy available` → garrote (ACTION_HIGH+7), ambush (ACTION_HIGH+6)
- `high energy available` → mutilate (ACTION_NORMAL+3)
- `hunger for blood` → hunger for blood (ACTION_HIGH+6)
- `slice and dice` → slice and dice (ACTION_HIGH+5)
- `combo points 4 available` → cold blood (ACTION_HIGH+6), envenom (ACTION_HIGH+5)
- `target with combo points almost dead` → envenom (ACTION_HIGH+4)
- `expose armor` → expose armor (ACTION_HIGH+3)
- `medium threat` → vanish (ACTION_HIGH)
- `low health` → evasion (ACTION_HIGH+9), feint (ACTION_HIGH+8)
- `critical health` → cloak of shadows (ACTION_HIGH+7)
- `kick` → kick (ACTION_INTERRUPT+2)
- `kick on enemy healer` → kick on enemy healer (ACTION_INTERRUPT+1)
- `medium aoe` → fan of knives (ACTION_NORMAL+5)
- `low tank threat` → tricks of the trade on main tank (ACTION_HIGH+7)
- `enemy out of melee` → stealth (ACTION_HIGH+3), sprint (ACTION_HIGH+2), reach melee (ACTION_HIGH+1)

### CombatRogueStrategy
- par défaut : killing spree, melee
- `high energy available` → garrote (ACTION_HIGH+7), ambush (ACTION_HIGH+6)
- `high energy available` → sinister strike (ACTION_NORMAL+3)
- `slice and dice` → slice and dice (ACTION_HIGH+5)
- `riposte` → riposte (ACTION_HIGH+4)
- `combo points 5 available` → rupture (ACTION_HIGH+1), eviscerate (ACTION_HIGH)
- `target with combo points almost dead` → eviscerate (ACTION_HIGH+2)
- `medium threat` → vanish (ACTION_HIGH)
- `low health` → evasion (ACTION_HIGH+9), feint (ACTION_HIGH+8)
- `critical health` → cloak of shadows (ACTION_HIGH+7)
- `kick` → kick (ACTION_INTERRUPT+2)
- `kick on enemy healer` → kick on enemy healer (ACTION_INTERRUPT+1)
- `enemy out of melee` → stealth (ACTION_HIGH+3), sprint (ACTION_HIGH+2), reach melee (ACTION_HIGH+1)
- `expose armor` → expose armor (ACTION_HIGH+3)
- `low tank threat` → tricks of the trade on main tank (ACTION_HIGH+7)
- `combo points 5 available` → eviscerate (ACTION_HIGH)
- `kick` → cheap shot (ACTION_INTERRUPT)
- `kick on enemy healer` → cheap shot (ACTION_INTERRUPT)
- `behind target` → ambush (ACTION_HIGH)
- `not behind target` → cheap shot (ACTION_HIGH)
- `enemy flagcarrier near` → sprint (ACTION_EMERGENCY+1)
- `unstealth` → unstealth (ACTION_NORMAL)
- `no stealth` → check stealth (ACTION_EMERGENCY)
- `sprint` → sprint (ACTION_INTERRUPT)
- `stealth` → stealth (ACTION_INTERRUPT)
- `light aoe` → blade flurry (ACTION_HIGH+4)
- `medium aoe` → fan of knives (ACTION_NORMAL+5)
- `adrenaline rush` → adrenaline rush (ACTION_HIGH+2)
- `blade flurry` → blade flurry (ACTION_HIGH+4)
- `sap` → stealth (ACTION_INTERRUPT), sap (ACTION_INTERRUPT)

### GenericRogueNonCombatStrategy
- `player has flag` → sprint (ACTION_EMERGENCY+1)
- `enemy flagcarrier near` → sprint (ACTION_EMERGENCY+2)

### GenericRogueStrategy
