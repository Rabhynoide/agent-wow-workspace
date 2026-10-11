# warlock — résumé automatique des sources (scripts/rotation_sources.py)

## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille
- **Power_Infusion_Warlock** (Warlock_Dynamic): aura 10060 on player
- **Tricks_Warlock** (Warlock_Dynamic): aura 57934 on player
- **Glyph of Corruption** (Warlock_Dynamic): aura 17941 on player
- **Glyph of Life Tap** (Warlock_Dynamic): aura 63321 on player
- **Empowered Imp** (Warlock_Dynamic): aura 47283 on player
- **Eradication** (Warlock_Dynamic): aura 64368 on player
- **Backdraft** (Warlock_Dynamic): aura 54277 on player
- **Pyroclasm** (Warlock_Dynamic): aura 18093 on player
- **Backlash** (Warlock_Dynamic): aura 34936 on player
- **Decimation** (Warlock_Dynamic): aura 63167 on player
- **Molten Core** (Warlock_Dynamic): aura 71165 on player
- **Shadow Embrace** (Warlock_Dynamic): aura 32391 on target
- **Seed of Corruption** (Warlock_Dynamic): aura 27243 on target
- **Nether Protection** (Warlock_Dynamic): aura 54374 on player
- **Demonic Pact** (Warlock_Dynamic): aura 48090 on player
- **Demonic Soul** (Warlock_Dynamic): aura 61595 on player
- **Devious Minds** (Warlock_Dynamic): aura 70840 on player
- **Shadow Cleave** (Warlock_Dynamic): spell Shadow Cleave (Cooldown Progress (Spell)); aura 47241 on player
- **Demon Charge** (Warlock_Dynamic): spell Demon Charge (Cooldown Progress (Spell)); aura 47241 on player
- **Immolation Aura** (Warlock_Dynamic): aura 50589 on player; spell Immolation Aura (Cooldown Progress (Spell)); aura 47241 on player
- **Challenging Howl** (Warlock_Dynamic): spell Challenging Howl (Cooldown Progress (Spell)); aura 47241 on player
- **Demonic_Empowerment** (Warlock_Core): aura 47193 on pet; spell Demonic Empowerment (Cooldown Progress (Spell))
- **Curses** (Warlock_Core): aura 980,603,1714,702,1490,18223 on target — expirationTime < 2 -> glow
- **Conflagrate** (Warlock_Core): spell Conflagrate (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Corruption** (Warlock_Core): aura 27216 on target — expirationTime < 2 -> glow
- **Unstable_Affliction** (Warlock_Core): aura 30108 on target — expirationTime < 3 -> glow
- **Haunt** (Warlock_Core): aura 48181 on target; spell Haunt (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; expirationTime < 3 -> glow
- **Immolate** (Warlock_Core): aura 348 on target; spell Immolate (Cooldown Progress (Spell)) — expirationTime < 2 -> glow
- **Chaos_Nova** (Warlock_Core): spell Chaos Bolt (Cooldown Progress (Spell)) — charges == 0 -> grey; expirationTime < 1 -> glow
- **Metamorphosis ** (Warlock_Core): aura 47241 on player; spell Metamorphosis (Cooldown Progress (Spell))
- **Shadowburn** (Warlock_Core): spell Shadowburn (Cooldown Progress (Spell)) — charges == 0 -> grey; expirationTime < 1 -> glow
- **Nightfall ** (Warlock_Afenar[WotLK]): aura 17941 on player
- **Shadowflame** (Warlock_Utility): aura 47897 on target; spell Shadowflame (Cooldown Progress (Spell))
- **Howl_of_Terror** (Warlock_Utility): spell Howl of Terror (Cooldown Progress (Spell))
- **Shadow_Fury** (Warlock_Utility): spell Shadowfury (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Death_Coil** (Warlock_Utility): spell Death Coil (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **No Pet - Warlock** (Warlock_Utility): aura 18790,18791,18791,18789 on player (missing); aura 19443 on player (missing)
- **Fire Shield** (Warlock_Utility): aura 11771 on player; spell 11771 (Spell Known)
- **Seduction** (Warlock_Utility): aura 6358 on multi; spell 6358 (Spell Known) — matchCount >= 1 -> grey; unitCount == 0 -> grey
- **Spell Lock** (Warlock_Utility): aura 19244 on multi; spell Spell Lock (Cooldown Progress (Spell)); spell 19244 (Spell Known)
- **Suffering** (Warlock_Utility): aura 19441 on player; spell Suffering (Cooldown Progress (Spell)); spell Sacrifice (Cooldown Progress (Spell)) — duration > 2 -> grey
- **Intercept - Warlock** (Warlock_Utility): aura 30151 on target; spell Intercept (Cooldown Progress (Spell)); spell 30151 (Spell Known)
- **Fel Domination** (Warlock_Utility): aura 18708 on player; spell Fel Domination (Cooldown Progress (Spell))
- **Shadow Ward** (Warlock_Utility): aura 6229 on player; spell Shadow Ward (Cooldown Progress (Spell))
- **Demonic Circle: Teleport** (Warlock_Utility): spell Demonic Circle: Teleport (Cooldown Progress (Spell)); aura 48018 on player
- **Soulshatter** (Warlock_Utility): spell Soulshatter (Cooldown Progress (Spell))
- **Ritual of Souls** (Warlock_Utility): spell Ritual of Souls (Cooldown Progress (Spell))
- **Ritual of Summoning** (Warlock_Utility): spell Ritual of Summoning (Cooldown Progress (Spell))
- **Healthstone** (Warlock_Utility): spell 6201 (Spell Known) — value == 0 -> grey
- **Summon_Infernal** (Warlock_Utility): spell 1122 (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Racial_Traits_Warlock** (Warlock_Utility): spell Gift of the Naaru (Cooldown Progress (Spell)); spell Stoneform (Cooldown Progress (Spell)); spell Escape Artist (Cooldown Progress (Spell)); spell Arcane Torrent (Cooldown Progress (Spell))
- **BloodLust_Warlock** (Warlock_Utility): aura 264667,90355,80353,32182,2825,292686,230935,256740,178207 on player
- **Saves_Warlock** (Warlock_Utility): aura 33206,97462,6940,1022,204018,15286,47788,31821,102342,116849,77764 on player
- **Soulstone** (Warlock_Utility): aura 20765 on group — matchCount < 1 -> grey; show >= 1 -> grey; value >= 1 -> grey
- **Soul Link** (Warlock_Utility): aura 25228 on player (missing)
- **Fel Armor** (Warlock_Utility): aura 28176,706,696 on player (missing)
- **Curses_raid** (Warlock_Utility): aura 1714,702,1490 on target

## mod-playerbots : déclencheur → actions (priorité)

### AfflictionWarlockStrategy

### DemonologyWarlockStrategy

### DestructionWarlockStrategy

### GenericWarlockNonCombatStrategy

### GenericWarlockStrategy
