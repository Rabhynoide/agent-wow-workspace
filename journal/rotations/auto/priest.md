# priest — résumé automatique des sources (scripts/rotation_sources.py)

## Afenar WotLK (WeakAuras) : ce qui est suivi, et quand ça brille
- **Tricks_Priest** (Priest_Dynamic): aura 57934 on player
- **Power_Infusion_Priest** (Priest_Dynamic): aura 10060 on player
- **Shadow Weaving** (Priest_Dynamic): aura 15258 on player
- **Blessed Recovery** (Priest_Dynamic): aura 27813 on player
- **Serendipity** (Priest_Dynamic): aura 63734 on player
- **Holy Concentration** (Priest_Dynamic): aura 34754 on player
- **Blessed Resilience** (Priest_Dynamic): aura 33143 on player
- **Focused Will** (Priest_Dynamic): aura 45242 on player
- **Borrowed Time** (Priest_Dynamic): aura 59891 on player
- **Spirit Tap** (Priest_Dynamic): aura 15271 on player
- **Improved Spirit Tap** (Priest_Dynamic): aura 59000 on player
- **Disciplined Power** (Priest_Dynamic): aura 64911 on player
- **Devious Mind** (Priest_Dynamic): aura 64907 on player
- **Shadow Word: Pain - Discipline & Holy** (Priest_Dynamic): aura 589 on target
- **Devouring Plague - Discipline & Holy** (Priest_Dynamic): aura 2944 on target
- **Shadow Word: Death - Discipline & Holy** (Priest_Dynamic): spell Shadow Word: Death (Cooldown Progress (Spell))
- **Prayer of Mending - Shadow** (Priest_Dynamic): spell Prayer of Mending (Cooldown Progress (Spell)); spell 34914 (Spell Known)
- **Holy Fire** (Priest_Dynamic): spell Holy Fire (Cooldown Progress (Spell))
- **ShadowFrom** (Priest_Dynamic): aura 15473 on player (missing)
- **innervate_Buff_Priest** (Priest_Dynamic): aura 29166 on player
- **Inner_Focus** (Priest_Core): aura 14751 on player; spell Inner Focus (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Prayer_of_Mending** (Priest_Core): spell Prayer of Mending (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Circle_of_Healing** (Priest_Core): spell Circle of Healing (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Power_Word:Shield_Holy** (Priest_Core): aura 17 on player; aura 6788 on player; spell Power Word: Shield (Cooldown Progress (Spell))
- **Penance 2** (Priest_Core): spell Penance (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Shadow_Word:Death** (Priest_Core): spell Shadow Word: Death (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Mind_Blast** (Priest_Core): spell Mind Blast (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; onCooldown == 1 -> grey
- **Devouring_Plague** (Priest_Core): aura 25467 on target — expirationTime <= 2 -> glow
- **Vampiric_Touch** (Priest_Core): aura 34914 on target; spell Vampiric Touch (Cooldown Progress (Spell)) — expirationTime <= 3 -> glow
- **Power_Infusion** (Priest_Core): aura 10060 on player; spell Power Infusion (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Pain_Suppresion** (Priest_Core): spell Pain Suppression (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Guardian_Spirit** (Priest_Core): spell Guardian Spirit (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Shadow_Word:Pain** (Priest_Core): aura 589 on target — expirationTime <= 3 -> glow
- **Divine_Hymn** (Priest_Utility): spell Divine Hymn (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Hymn_Of_Hope** (Priest_Utility): spell Hymn of Hope (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Shadow_Fiend** (Priest_Utility): spell Shadowfiend (Cooldown Progress (Spell))
- **Innier_Focus_Shadow** (Priest_Utility): aura 14751 on player; spell Inner Focus (Cooldown Progress (Spell)) — expirationTime < 1 -> glow; show < 1 -> glow
- **Silence ** (Priest_Utility): spell Silence (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Psychic_Scream** (Priest_Utility): spell Psychic Scream (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Desperate_Prayer** (Priest_Utility): aura 19236 on player; spell Desperate Prayer (Cooldown Progress (Spell))
- **Dispersion** (Priest_Utility): aura 47585 on player; spell Dispersion (Cooldown Progress (Spell))
- **Fade** (Priest_Utility): aura 586,213602 on player; spell Fade (Cooldown Progress (Spell))
- **Fear_Ward** (Priest_Utility): spell Fear Ward (Cooldown Progress (Spell)) — expirationTime < 1 -> glow
- **Power_Word:Shield(Shadow&Disc)** (Priest_Utility): aura 17 on player; aura 6788 on player; spell Power Word: Shield (Cooldown Progress (Spell))
- **Racials_Priest** (Priest_Utility): spell Gift of the Naaru (Cooldown Progress (Spell)); spell Stoneform (Cooldown Progress (Spell)); spell Escape Artist (Cooldown Progress (Spell)); spell Arcane Torrent (Cooldown Progress (Spell))
- **Vampiric_Embrace** (Priest_Utility): aura 15286 on player (missing)
- **Devine_Spirit** (Priest_Utility): aura 25312,32999 on player (missing)
- **Power Word: Fortitude** (Priest_Utility): aura Power Word: Fortitude,25392 on player (missing)
- **Inner Fire** (Priest_Utility): aura Inner Fire on player (missing)
- **Saves_Priest** (Priest_Utility): aura 33206,6940,1022,15286,47788,31821 on player
- **BloodLust_Priest** (Priest_Utility): aura 32182,2825 on player

## mod-playerbots : déclencheur → actions (priorité)

### DiscPriestStrategy
- par défaut : shoot
- `group heal setting` → prayer of mending on party (ACTION_MEDIUM_HEAL+8), power word: shield on not full (ACTION_MEDIUM_HEAL+7)
- `medium group heal setting` → divine hymn (ACTION_CRITICAL_HEAL+7), prayer of mending on party (ACTION_CRITICAL_HEAL+6), power word: shield on not full (ACTION_CRITICAL_HEAL+5), prayer of healing on party (ACTION_CRITICAL_HEAL+4)
- `party member critical health` → power word: shield on party (ACTION_CRITICAL_HEAL+5), penance on party (ACTION_CRITICAL_HEAL+4), prayer of mending on party (ACTION_CRITICAL_HEAL+3), flash heal on party (ACTION_CRITICAL_HEAL+2)
- `party member low health` → power word: shield on party (ACTION_MEDIUM_HEAL+4), prayer of mending on party (ACTION_MEDIUM_HEAL+3), penance on party (ACTION_MEDIUM_HEAL+2), flash heal on party (ACTION_MEDIUM_HEAL+0)
- `party member medium health` → power word: shield on party (ACTION_LIGHT_HEAL+9), prayer of mending on party (ACTION_LIGHT_HEAL+7), penance on party (ACTION_LIGHT_HEAL+6), flash heal on party (ACTION_LIGHT_HEAL+5)
- `party member almost full health` → power word: shield on party (ACTION_LIGHT_HEAL+3), prayer of mending on party (ACTION_LIGHT_HEAL+2), renew on party (ACTION_LIGHT_HEAL+1)
- `party member to heal out of spell range` → reach party member to heal (ACTION_CRITICAL_HEAL+10)
- `critical health` → pain suppression (ACTION_EMERGENCY+1)
- `protect party member` → pain suppression on party (ACTION_EMERGENCY)

### GenericPriestStrategy
- `critical health` → desperate prayer (ACTION_HIGH+5)
- `critical health` → power word: shield (ACTION_NORMAL)
- `fear ward on main tank` → fear ward on main tank (ACTION_HIGH+3)
- `low health` → power word: shield (ACTION_HIGH)
- `medium mana` → shadowfiend (ACTION_HIGH+2), inner focus (ACTION_HIGH+1)
- `low mana` → hymn of hope (ACTION_HIGH)
- `enemy too close for spell` → flee (ACTION_MOVE+9)
- `being attacked` → power word: shield (ACTION_HIGH+1)
- `healer should attack` → shadow word: pain (ACTION_DEFAULT+0.5f), holy fire (ACTION_DEFAULT+0.4f), smite (ACTION_DEFAULT+0.3f), mind blast (ACTION_DEFAULT+0.2f), shoot (ACTION_DEFAULT)
- `medium aoe and healer should attack` → mind sear (ACTION_DEFAULT+0.5f)

### HolyPriestStrategy
- par défaut : smite, mana burn, starshards
- `holy fire` → holy fire (ACTION_NORMAL+9)
- `shadowfiend` → shadowfiend (ACTION_HIGH)
- `medium mana` → shadowfiend (ACTION_HIGH)
- `low mana` → mana burn (ACTION_HIGH)
- `party member low health` → power word: shield on party (ACTION_MEDIUM_HEAL+3), flash heal on party (ACTION_MEDIUM_HEAL+2)
- `party member medium health` → renew on party (ACTION_LIGHT_HEAL+2)
- `party member to heal out of spell range` → reach party member to heal (ACTION_CRITICAL_HEAL+10)
- `group heal setting` → prayer of mending on party (ACTION_MEDIUM_HEAL+9), circle of healing on party (ACTION_MEDIUM_HEAL+8)
- `medium group heal setting` → divine hymn (ACTION_CRITICAL_HEAL+7), prayer of mending on party (ACTION_CRITICAL_HEAL+6), circle of healing on party (ACTION_CRITICAL_HEAL+5), prayer of healing on party (ACTION_CRITICAL_HEAL+4)
- `party member critical health` → guardian spirit on party (ACTION_CRITICAL_HEAL+6), power word: shield on party (ACTION_CRITICAL_HEAL+5), prayer of mending on party (ACTION_CRITICAL_HEAL+3), greater heal on party (ACTION_MEDIUM_HEAL+2), flash heal on party (ACTION_CRITICAL_HEAL+1)
- `party member low health` → circle of healing on party (ACTION_MEDIUM_HEAL+4), prayer of mending on party (ACTION_MEDIUM_HEAL+3), greater heal on party (ACTION_MEDIUM_HEAL+2), flash heal on party (ACTION_MEDIUM_HEAL+1)
- `party member medium health` → circle of healing on party (ACTION_LIGHT_HEAL+7), prayer of mending on party (ACTION_LIGHT_HEAL+6), greater heal on party (ACTION_MEDIUM_HEAL+5), flash heal on party (ACTION_LIGHT_HEAL+4)
- `party member almost full health` → renew on party (ACTION_LIGHT_HEAL+2), prayer of mending on party (ACTION_LIGHT_HEAL+1)
- `party member to heal out of spell range` → reach party member to heal (ACTION_CRITICAL_HEAL+10)

### PriestNonCombatStrategy
- `party member dead` → remove shadowform (ACTION_CRITICAL_HEAL+11), resurrection (ACTION_CRITICAL_HEAL+10)
- `party member critical health` → renew on party (ACTION_CRITICAL_HEAL+3), penance on party (ACTION_CRITICAL_HEAL+2), greater heal on party (ACTION_CRITICAL_HEAL+1)
- `party member low health` → renew on party (ACTION_MEDIUM_HEAL+3), penance on party (ACTION_MEDIUM_HEAL+2), greater heal on party (ACTION_MEDIUM_HEAL+1)
- `party member medium health` → renew on party (ACTION_LIGHT_HEAL+9), penance on party (ACTION_LIGHT_HEAL+8)
- `party member almost full health` → renew on party (ACTION_LIGHT_HEAL+3)

### ShadowPriestStrategy
- par défaut : mind blast, mind flay, shadow word: death, shoot
- `shadowform` → shadowform (ACTION_HIGH)
- `low mana` → dispersion (ACTION_HIGH+5)
- `critical health` → dispersion (ACTION_HIGH+5)
- `silence` → silence (ACTION_INTERRUPT+1)
- `silence on enemy healer` → silence on enemy healer (ACTION_INTERRUPT)
- `shadow word: pain on attacker` → shadow word: pain on attacker (ACTION_NORMAL+5)
- `vampiric touch on attacker` → vampiric touch on attacker (ACTION_NORMAL+4)
- `mind sear channel check` → cancel channel (ACTION_HIGH+5)
- `medium aoe` → mind sear (ACTION_HIGH+4)
- `vampiric touch` → vampiric touch (ACTION_HIGH+3)
- `devouring plague` → devouring plague (ACTION_HIGH+2)
- `shadow word: pain` → shadow word: pain (ACTION_HIGH+1)
