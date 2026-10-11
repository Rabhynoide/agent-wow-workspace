# paladin — résumé automatique des sources (scripts/rotation_sources.py)

## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille
- **Unholy_Frenzy_Paladin** (Paladin_Up): aura 49016 on player
- **Power_Infusion_Paladin** (Paladin_Up): aura 10060 on player
- **Tricks_Paladin** (Paladin_Up): aura 57934 on player
- **Blessings** (Paladin_Up): aura 19740,25782,19742,25894,20217,25898,20911,25899 on player; aura 19740,25782,19742,25894,20217,25898,20911,25899 on player (missing)
- **Seals** (Paladin_Up): aura 21084,53736,31801,20164,20166,20165,20375 on player; aura 21084,53736,31801,20164,20166,20165,20375 on player (missing)
- **Righteous Fury** (Paladin_Up): aura 25780 on player (missing)
- **Exorcism[Holy_Protection]** (Paladin_Up): spell Exorcism (Cooldown Progress (Spell))
- **Consecration[Holy_Protection]** (Paladin_Up): spell 26573 (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Hammer_of_Wrath** (Paladin_Up): spell Hammer of Wrath (Cooldown Progress (Spell)); spell Hammer of Wrath (Action Usable) — expirationTime < 0.5 -> glow; onCooldown == 1 -> grey
- **Reckoning** (Paladin_Up): aura 20178 on player
- **Vengeance&Corruption** (Paladin_Up): aura 356110,31803 on target; spell 348704 (Spell Known); spell 31801 (Spell Known) — stacks == 5 -> glow
- **Redoubt** (Paladin_Up): aura 20132 on player
- **Art_of_War** (Paladin_Up): aura 59578 on player
- **Judgement_Debuff** (Paladin_Up): aura 20185,20186,20184 on target
- **Vengeance** (Paladin_Up): aura 20050,20052,20053,20054,20055 on player
- **Light's_Grace** (Paladin_Up): aura 31834 on player
- **Judgments_of_the_Pure** (Paladin_Up): aura 54153 on player
- **Sacred_Shield** (Paladin_Up): aura 53601 on group
- **Deliverance** (Paladin_Up): aura 70760 on player
- **Holiness** (Paladin_Up): aura 70757 on player
- **Aegis** (Paladin_Up): aura 64883 on player
- **Hand_of_Reckoning** (Paladin_Up): spell Hand of Reckoning (Cooldown Progress (Spell))
- **Righteous_Defense** (Paladin_Up): spell Righteous Defense (Cooldown Progress (Spell))
- **innervate_B_Paladin** (Paladin_Up): aura 29166 on player
- **Paladin_Mana** (Paladin_Afenar[WotLK]): aura 54149 on player
- **Exorcism** (Paladin_Core): spell Exorcism (Cooldown Progress (Spell)); aura 59578 on player — expirationTime < 1 -> glow
- **Beacon_of_Light** (Paladin_Core): aura 53563 on group — expirationTime < 3 -> glow; matchCount < 1 -> grey
- **Avenger's_Shield** (Paladin_Core): spell Avenger's Shield (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Judgment** (Paladin_Core): spell Judgement of Light (Cooldown Progress (Spell)) — charges < 1 -> grey; expirationTime < 1 -> glow
- **Shield_of_Righteousness** (Paladin_Core): spell Shield of Righteousness (Cooldown Progress (Spell)); spell Shield of Righteousness (Spell Known) — duration < 2 -> glow; expirationTime < 1 -> glow
- **Crusader_Stike** (Paladin_Core): spell Crusader Strike (Cooldown Progress (Spell)) — charges == 0 -> grey; expirationTime < 1 -> glow
- **Hammer_of_the_Righteous** (Paladin_Core): spell Hammer of the Righteous (Cooldown Progress (Spell)) — charges == 0 -> grey; expirationTime < 1 -> glow
- **Divine_Storm** (Paladin_Core): spell Divine Storm (Cooldown Progress (Spell)) — charges == 0 -> grey; expirationTime < 1 -> glow
- **Holy Shield** (Paladin_Core): aura 20925 on player; spell Holy Shield (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Consecration[Retribution]** (Paladin_Core): spell 26573 (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Holy_Shock** (Paladin_Core): spell Holy Shock (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Infusion_of_Light** (Paladin_Core): aura 54149 on player; spell 200025 (Spell Known)
- **Sacred_Shield_Holy** (Paladin_Core): aura 53601 on group — expirationTime < 3 -> glow; matchCountPerUnit >= 1 -> grey
- **Avenging_Wrath** (Paladin_Utility): aura 31884 on player; spell Avenging Wrath (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown < 1 -> glow
- **Divine Favor** (Paladin_Utility): aura 20216 on player; spell Divine Favor (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Divine Plea** (Paladin_Utility): aura 54428 on player; spell Divine Plea (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Divine Illumination** (Paladin_Utility): aura 31842 on player; spell Divine Illumination (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Holy Wrath** (Paladin_Utility): spell Holy Wrath (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Hummer_of_Justice** (Paladin_Utility): spell Hammer of Justice (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Repentance** (Paladin_Utility): spell Repentance (Cooldown Progress (Spell))
- **Divine_Protection** (Paladin_Utility): aura 498 on player; spell Divine Protection (Cooldown Progress (Spell))
- **Divine_Shiled** (Paladin_Utility): aura 642 on player; spell Divine Shield (Cooldown Progress (Spell))
- **Aura_Mastery** (Paladin_Utility): aura 31821 on player; spell Aura Mastery (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Divine Sacrifice** (Paladin_Utility): aura 64205 on player; spell Divine Sacrifice (Cooldown Progress (Spell))
- **Blessing of Freedom** (Paladin_Utility): aura 1044 on group; spell Hand of Freedom (Cooldown Progress (Spell))
- **Blessing of Protection** (Paladin_Utility): aura 1022 on group; spell Hand of Protection (Cooldown Progress (Spell))
- **Blessing of Sacrifice** (Paladin_Utility): aura 6940 on group; spell Hand of Sacrifice (Cooldown Progress (Spell))
- **Hand of Salvation** (Paladin_Utility): aura 1038 on group; spell Hand of Salvation (Cooldown Progress (Spell))
- **Forbearance** (Paladin_Utility): aura 25771 on player
- **Lay_of_Hand** (Paladin_Utility): spell Lay on Hands (Cooldown Progress (Spell))
- **Divine Intervention** (Paladin_Utility): spell Divine Intervention (Cooldown Progress (Spell))
- **Racial_Traits_Paladin** (Paladin_Utility): spell Gift of the Naaru (Cooldown Progress (Spell)); spell Stoneform (Cooldown Progress (Spell)); spell Escape Artist (Cooldown Progress (Spell)); spell Arcane Torrent (Cooldown Progress (Spell))
- **BloodLust_Paladin** (Paladin_Utility): aura 32182,2825 on player
- **Saves_Paladin** (Paladin_Utility): aura 33206,6940,1022,15286,47788,31821 on player
- **Auras** (Paladin_Utility): aura 465,32223,7294,19746,19876,19888,19891 on player

## mod-playerbots : déclencheur → actions (priorité)

### DpsPaladinStrategy
- par défaut : hammer of wrath, judgement of wisdom, crusader strike, divine storm, consecration, melee
- `art of war` → exorcism (ACTION_DEFAULT+0.2f)
- `seal` → seal of corruption (ACTION_HIGH)
- `low mana` → seal of wisdom (ACTION_HIGH+5)
- `avenging wrath` → avenging wrath (ACTION_HIGH+2)
- `medium aoe` → divine storm (ACTION_HIGH+4), consecration (ACTION_HIGH+3)
- `enemy out of melee` → reach melee (ACTION_HIGH+1)

### GenericPaladinNonCombatStrategy
- `party member dead` → redemption (ACTION_CRITICAL_HEAL+10)
- `party member almost full health` → flash of light on party (ACTION_MEDIUM_HEAL+5.0f)
- `party member medium health` → flash of light on party (ACTION_MEDIUM_HEAL+6.0f)
- `party member low health` → holy light on party (ACTION_MEDIUM_HEAL+7.0f)
- `party member critical health` → holy light on party (ACTION_MEDIUM_HEAL+8.0f)
- `not sensing undead` → sense undead (ACTION_IDLE+1.0f)
- `often` → apply oil (ACTION_IDLE+1.0f)
- `often` → apply stone (ACTION_IDLE+1.0f)
- `greater blessing needed` → cast greater blessing assignment (ACTION_NORMAL)

### GenericPaladinStrategy
- `hammer of justice interrupt` → hammer of justice (ACTION_INTERRUPT)
- `hammer of justice on enemy healer` → hammer of justice on enemy healer (ACTION_INTERRUPT)
- `hammer of justice on snare target` → hammer of justice on snare target (ACTION_INTERRUPT)
- `critical health` → divine shield (ACTION_EMERGENCY)
- `critical health` → lay on hands (ACTION_EMERGENCY+1)
- `party member critical health` → lay on hands on party (ACTION_EMERGENCY+2)
- `divine shield low health` → flash of light (ACTION_EMERGENCY+3), holy light (ACTION_EMERGENCY+2)
- `protect party member` → blessing of protection on party (ACTION_EMERGENCY+3)
- `high mana` → divine plea (ACTION_HIGH)
- `hand of freedom on party` → hand of freedom on party (ACTION_HIGH+4)
- `cleanse cure disease` → cleanse disease (ACTION_DISPEL+2)
- `cleanse party member cure disease` → cleanse disease on party (ACTION_DISPEL+1)
- `cleanse cure poison` → cleanse poison (ACTION_DISPEL+2)
- `cleanse party member cure poison` → cleanse poison on party (ACTION_DISPEL+1)
- `cleanse cure magic` → cleanse magic (ACTION_DISPEL+2)
- `cleanse party member cure magic` → cleanse magic on party (ACTION_DISPEL+1)
- `turn undead` → turn undead (ACTION_HIGH+1)
- `healer should attack` → hammer of wrath (ACTION_DEFAULT+0.6f), holy shock (ACTION_DEFAULT+0.5f), shield of righteousness (ACTION_DEFAULT+0.4f), judgement of light (ACTION_DEFAULT+0.3f), consecration (ACTION_DEFAULT+0.2f), exorcism (ACTION_DEFAULT+0.1f)

### HealPaladinStrategy
- par défaut : judgement of light
- `seal` → seal of wisdom (ACTION_HIGH)
- `medium mana` → divine illumination (ACTION_HIGH+2)
- `low mana` → divine favor (ACTION_HIGH+1)
- `party member to heal out of spell range` → reach party member to heal (ACTION_EMERGENCY+3)
- `medium group heal setting` → divine sacrifice (ACTION_CRITICAL_HEAL+5), avenging wrath (ACTION_HIGH+4)
- `party member critical health` → holy shock on party (ACTION_CRITICAL_HEAL+6), divine sacrifice (ACTION_CRITICAL_HEAL+5), holy light on party (ACTION_CRITICAL_HEAL+4)
- `party member low health` → holy light on party (ACTION_MEDIUM_HEAL+5)
- `party member medium health` → holy light on party (ACTION_LIGHT_HEAL+9), flash of light on party (ACTION_LIGHT_HEAL+8)
- `party member almost full health` → flash of light on party (ACTION_LIGHT_HEAL+3)
- `beacon of light on main tank` → beacon of light on main tank (ACTION_CRITICAL_HEAL+7)
- `sacred shield on main tank` → sacred shield on main tank (ACTION_CRITICAL_HEAL+6)

### OffhealRetPaladinStrategy
- par défaut : hammer of wrath, judgement of wisdom, crusader strike, divine storm, melee
- `seal` → seal of corruption (ACTION_HIGH)
- `low mana` → seal of wisdom (ACTION_HIGH+5), divine plea (ACTION_HIGH+4)
- `art of war` → exorcism (ACTION_HIGH+1)
- `avenging wrath` → avenging wrath (ACTION_HIGH+2)
- `medium aoe` → divine storm (ACTION_HIGH+4), consecration (ACTION_HIGH+3)
- `enemy out of melee` → reach melee (ACTION_HIGH+1)
- `retribution aura` → retribution aura (ACTION_NORMAL)
- `blessing of might` → blessing of might (ACTION_NORMAL+1)
- `low health` → holy light (ACTION_CRITICAL_HEAL+2)
- `party member critical health` → holy shock on party (ACTION_CRITICAL_HEAL+6), holy light on party (ACTION_CRITICAL_HEAL+4)
- `party member low health` → holy light on party (ACTION_MEDIUM_HEAL+5)
- `party member medium health` → flash of light on party (ACTION_LIGHT_HEAL+8)
- `party member almost full health` → flash of light on party (ACTION_LIGHT_HEAL+3)
- `party member to heal out of spell range` → reach party member to heal (ACTION_EMERGENCY+3)
- `beacon of light on main tank` → beacon of light on main tank (ACTION_CRITICAL_HEAL+7)

### PaladinBuffStrategies
- `shadow resistance aura` → shadow resistance aura (ACTION_NORMAL)
- `frost resistance aura` → frost resistance aura (ACTION_NORMAL)
- `fire resistance aura` → fire resistance aura (ACTION_NORMAL)
- `devotion aura` → devotion aura (ACTION_NORMAL)
- `retribution aura` → retribution aura (ACTION_NORMAL)
- `concentration aura` → concentration aura (ACTION_NORMAL)
- `crusader aura` → crusader aura (ACTION_NORMAL)
- `righteous fury` → righteous fury (ACTION_HIGH+8)

### TankPaladinStrategy
- par défaut : shield of righteousness, hammer of the righteous, judgement of wisdom, melee
- `seal` → seal of corruption (ACTION_HIGH)
- `low mana` → seal of wisdom (ACTION_HIGH+9)
- `light aoe` → avenger's shield (ACTION_HIGH+5)
- `medium aoe` → consecration (ACTION_HIGH+7), avenger's shield (ACTION_HIGH+6)
- `lose aggro` → hand of reckoning (ACTION_HIGH+7)
- `medium health` → holy shield (ACTION_HIGH+4)
- `avenging wrath` → avenging wrath (ACTION_HIGH+2)
- `target critical health` → hammer of wrath (ACTION_CRITICAL_HEAL)
- `righteous fury` → righteous fury (ACTION_HIGH+8)
- `medium group heal setting` → divine sacrifice (ACTION_HIGH+5)
- `enough mana` → consecration (ACTION_HIGH+4)
- `not facing target` → set facing (ACTION_NORMAL+7)
- `enemy out of melee` → reach melee (ACTION_HIGH+1)
