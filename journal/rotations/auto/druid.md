# druid — résumé automatique des sources (scripts/rotation_sources.py)

## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille
- **Travel_Forms** (Druid_Dynamic): aura 783,33943,40120 on player
- **Tree_Of_Life** (Druid_Dynamic): aura 33891 on player (missing); aura 783,33943,40120,768,9634,5487 on player (missing)
- **Moonkin** (Druid_Dynamic): aura 24858 on player (missing); aura 783,33943,40120,768,9634,5487 on player (missing)
- **Faerie Fire** (Druid_Dynamic): aura 770,16857 on target (missing)
- **Faerie_Fire(Feral)** (Druid_Dynamic): spell Faerie Fire (Feral) (Cooldown Progress (Spell))
- **Mangle (Cat)** (Druid_Dynamic): aura 33876 on target (missing); aura 768 on player
- **Omen of Clarity** (Druid_Dynamic): aura 16870 on player
- **Tricks_Druid** (Druid_Dynamic): aura 57934 on player
- **Power_Infusion_Druid** (Druid_Dynamic): aura 10060 on player
- **Nature's Grace** (Druid_Dynamic): aura 16886 on player
- **Savage Defense** (Druid_Dynamic): aura 62606 on player
- **Infected Wound** (Druid_Dynamic): aura 58179 on target
- **Predator's Swiftness** (Druid_Dynamic): aura 69369 on player
- **Owlkin Frenzy** (Druid_Dynamic): aura 48391 on player
- **Natural Perfection** (Druid_Dynamic): aura 45281 on player
- **Elune's Wrath** (Druid_Dynamic): aura 64823 on player
- **Wrath of Elune** (Druid_Dynamic): aura 46833 on player
- **Maul_Charge** (Druid_Dynamic): spell Heroic Throw (Queued Action)
- **Druid_Mana_Boomie** (Druid_Afenar[WotLK]): aura 24858 on player
- **Druid_Mana** (Druid_Afenar[WotLK]): aura 24858 on player
- **Druid_CP_1** (Druid_CP): aura 768 on player
- **Druid_CP_2** (Druid_CP): aura 768 on player
- **Druid_CP_3** (Druid_CP): aura 768 on player
- **Druid_CP_4** (Druid_CP): aura 768 on player
- **Druid_CP_5** (Druid_CP): aura 768 on player
- **Druid_Positions** (Druid_Afenar[WotLK]): aura 768 on player
- **Eclipse_Lunar** (Druid_Core): aura 48518 on player; aura 48517 on player; aura 768,9634,5487 on player (missing)
- **Moonfire** (Druid_Core): aura 8921 on target; aura 768,9634,5487 on player (missing) — expirationTime <= 2 -> glow
- **Starfall** (Druid_Core): aura 48505 on player; spell Starfall (Cooldown Progress (Spell))
- **Insect_Swarm** (Druid_Core): aura 5570 on target; aura 768,9634,5487 on player (missing) — expirationTime <= 2 -> glow
- **Eclipse_Solar** (Druid_Core): aura 48517 on player; aura 48518 on player; aura 768,9634,5487 on player (missing)
- **Rejuvination ** (Druid_Core): aura 774 on multi; aura 5487,9634,768,24858 on player (missing) — matchCountPerUnit >= 1 -> grey
- **Regrowth ** (Druid_Core): aura 26980 on multi; aura 5487,9634,768,24858 on player (missing) — matchCountPerUnit >= 1 -> grey
- **Lifebloom** (Druid_Core): aura 33763 on multi; aura 5487,9634,768,24858 on player (missing) — matchCountPerUnit >= 1 -> grey
- **Wild_Growth** (Druid_Core): spell Wild Growth (Cooldown Progress (Spell)); aura 5487,9634,768,24858 on player (missing) — expirationTime < 1 -> glow
- **Swiftmend** (Druid_Core): spell Swiftmend (Cooldown Progress (Spell)); aura 5487,9634,768,24858 on player (missing) — onCooldown < 1 -> grey
- **Nature's_Swiftness** (Druid_Core): aura 17116 on player; spell Nature's Swiftness (Cooldown Progress (Spell))
- **Enrage - Druid** (Druid_Core): aura 5229 on player; spell Enrage (Cooldown Progress (Spell)); aura 5487,9634 on player
- **Maul** (Druid_Core): spell Maul (Cooldown Progress (Spell)); aura 5487,9634 on player
- **Demoralizing Roar** (Druid_Core): aura 48560 on target; aura 5487,9634 on player — buffed < 0 -> grey; buffed > 1 -> grey
- **Tiger's_Fury** (Druid_Core): aura 5217 on player; spell Tiger's Fury (Cooldown Progress (Spell)); aura 768 on player — expirationTime < 1 -> glow
- **Rake** (Druid_Core): aura 27008 on target; aura 768 on player — expirationTime < 4 -> glow
- **Rip** (Druid_Core): aura 27003 on target; spell Rake (Cooldown Progress (Spell)); aura 768 on player — expirationTime < 2 -> glow
- **Mangle** (Druid_Core): spell Mangle (Bear) (Cooldown Progress (Spell)); aura 9634,5487 on player — expirationTime < 0.5 -> glow
- **Lacerate** (Druid_Core): aura 33745 on target; aura 5487,9634 on player
- **Savage_Roar** (Druid_Core): aura 52610 on player; aura 768 on player
- **Berserk ** (Druid_Core): aura 50334 on player; spell Berserk (Cooldown Progress (Spell))
- **Force of Nature** (Druid_Utility): spell Force of Nature (Cooldown Progress (Spell))
- **Typhoon** (Druid_Utility): spell Typhoon (Cooldown Progress (Spell))
- **Mighty_Bash** (Druid_Utility): spell Bash (Cooldown Progress (Spell))
- **Maim** (Druid_Utility): spell Maim (Cooldown Progress (Spell))
- **Barkskin** (Druid_Utility): aura 22812 on player; spell Barkskin (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Survival_Instincts** (Druid_Utility): aura 61336 on player; spell Survival Instincts (Cooldown Progress (Spell))
- **Frenzied_Regeneration** (Druid_Utility): spell Frenzied Regeneration (Cooldown Progress (Spell)) — onCooldown < 1 -> grey
- **Dash** (Druid_Utility): aura 1850,252216 on player; spell Dash (Cooldown Progress (Spell))
- **innervate** (Druid_Utility): aura 29166 on player; spell Innervate (Cooldown Progress (Spell))
- **Nature's Grasp** (Druid_Utility): aura 16689 on player; spell Nature's Grasp (Cooldown Progress (Spell))
- **Feral_Charge_Bear** (Druid_Utility): spell Feral Charge - Bear (Cooldown Progress (Spell))
- **Feral_Charge_Cat** (Druid_Utility): spell Feral Charge - Cat (Cooldown Progress (Spell))
- **Tranquility** (Druid_Utility): spell Tranquility (Cooldown Progress (Spell))
- **Rebirth** (Druid_Utility): spell Rebirth (Cooldown Progress (Spell))
- **Challenging_Roar** (Druid_Utility): aura 5209 on target; spell Challenging Roar (Cooldown Progress (Spell))
- **Prowl** (Druid_Utility): aura 5215 on player; spell Prowl (Cooldown Progress (Spell))
- **Cower - Druid** (Druid_Utility): spell Cower (Cooldown Progress (Spell))
- **Growl** (Druid_Utility): spell Growl (Cooldown Progress (Spell))
- **Mark of the Wild** (Druid_Utility): aura 1126,21849 on player (missing)
- **Thorns** (Druid_Utility): aura 467 on player
- **Racial_Traits_Druid** (Druid_Utility): spell Gift of the Naaru (Cooldown Progress (Spell)); spell Stoneform (Cooldown Progress (Spell)); spell Escape Artist (Cooldown Progress (Spell)); spell Arcane Torrent (Cooldown Progress (Spell))
- **BloodLust_Druid** (Druid_Utility): aura 32182,2825 on player
- **Saves_Druid** (Druid_Utility): aura 33206,6940,1022,15286,47788,31821 on player
- **Druid_Mana_Nonhealer** (Druid_Afenar[WotLK]): aura 768 on player (missing); aura 5487,9634 on player (missing)
- **Feral_Energy[Bear]** (Druid_Afenar[WotLK]): aura 9634,5487 on player
- **Druid_Cast** (Druid_Afenar[WotLK]): aura 24858 on player (missing)
- **Druid_Cast_Boomie** (Druid_Afenar[WotLK]): aura 24858 on player

## mod-playerbots : déclencheur → actions (priorité)

### BalanceDruidStrategy

### BearDruidStrategy

### CatDruidStrategy
- par défaut : melee

### FeralDruidStrategy

### GenericDruidNonCombatStrategy
- `party member dead` → revive (ACTION_CRITICAL_HEAL+10)
- `party member critical health` → wild growth on party (ACTION_MEDIUM_HEAL+7), regrowth on party (ACTION_MEDIUM_HEAL+6), rejuvenation on party (ACTION_MEDIUM_HEAL+5)
- `party member low health` → wild growth on party (ACTION_MEDIUM_HEAL+5), regrowth on party (ACTION_MEDIUM_HEAL+4), rejuvenation on party (ACTION_MEDIUM_HEAL+3)
- `party member medium health` → wild growth on party (ACTION_MEDIUM_HEAL+3), regrowth on party (ACTION_MEDIUM_HEAL+2), rejuvenation on party (ACTION_MEDIUM_HEAL+1)
- `party member almost full health` → wild growth on party (ACTION_LIGHT_HEAL+3), rejuvenation on party (ACTION_LIGHT_HEAL+2)
- `party member remove curse` → remove curse on party (ACTION_DISPEL+7)
- `party member critical health` → wild growth on party (ACTION_MEDIUM_HEAL+7), regrowth on party (ACTION_MEDIUM_HEAL+6), rejuvenation on party (ACTION_MEDIUM_HEAL+5)
- `party member low health` → wild growth on party (ACTION_MEDIUM_HEAL+5), regrowth on party (ACTION_MEDIUM_HEAL+4), rejuvenation on party (ACTION_MEDIUM_HEAL+3)
- `party member medium health` → wild growth on party (ACTION_MEDIUM_HEAL+3), regrowth on party (ACTION_MEDIUM_HEAL+2), rejuvenation on party (ACTION_MEDIUM_HEAL+1)
- `party member almost full health` → wild growth on party (ACTION_LIGHT_HEAL+3), rejuvenation on party (ACTION_LIGHT_HEAL+2)
- `party member remove curse` → remove curse on party (ACTION_DISPEL+7)

### GenericDruidStrategy

### RestoDruidStrategy
