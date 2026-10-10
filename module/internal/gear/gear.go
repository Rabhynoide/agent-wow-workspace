// Package gear decides which items improve the character (class proficiency, level, stat weights).
package gear

import (
	pb "wowagent/gen/worldv1"
)

type Item = pb.ItemInfo

// Equipment slots.
const (
	SlotMainHand = 15
	SlotOffHand  = 16
	SlotRanged   = 17
)

const invTwoHand = 17

// Slots returns the equipment slots an inventory type can go to (none for bags, ammo, tabards...).
func Slots(invType uint32) []int {
	switch invType {
	case 1, 2, 3, 4:
		return []int{int(invType) - 1}
	case 5, 20:
		return []int{4}
	case 6, 7, 8, 9, 10:
		return []int{int(invType) - 1}
	case 11:
		return []int{10, 11}
	case 12:
		return []int{12, 13}
	case 16:
		return []int{14}
	case 13, 21, invTwoHand:
		return []int{SlotMainHand}
	case 14, 22, 23:
		return []int{SlotOffHand}
	case 15, 25, 26, 28:
		return []int{SlotRanged}
	}
	return nil
}

// Proficiency spells per item class (2 weapon, 4 armor) and subclass. Misc armor (rings, necks,
// trinkets, off-hand frills) needs none.
var Proficiency = map[uint32]map[uint32]uint32{
	2: {0: 196, 1: 197, 2: 264, 3: 266, 4: 198, 5: 199, 6: 200, 7: 201, 8: 202, 10: 227, 13: 15590, 15: 1180, 16: 2567, 18: 5011, 19: 5009},
	4: {1: 9078, 2: 9077, 3: 8737, 4: 750, 6: 9116},
}

// Usable reports whether the character can equip the item: level, class/race masks and the
// proficiency spell for its armor/weapon type (known = spell book).
func Usable(it *Item, class, race, level uint32, known map[uint32]bool) bool {
	if it == nil || len(Slots(it.InventoryType)) == 0 || it.RequiredLevel > level {
		return false
	}
	if it.AllowableClass != -1 && it.AllowableClass != 0 && it.AllowableClass&(1<<(class-1)) == 0 {
		return false
	}
	if it.AllowableRace != -1 && it.AllowableRace != 0 && it.AllowableRace&(1<<(race-1)) == 0 {
		return false
	}
	if it.Class == 4 && it.Subclass == 0 {
		return true
	}
	spell, ok := Proficiency[it.Class][it.Subclass]
	return ok && known[spell]
}

// Stat weights (ItemModType) for a leveling shadow priest. Order from the leveling guides:
// spell power > spirit > haste > crit > intellect > stamina > hit (hit only matters near level 80).
var weights = map[uint32]float32{
	45: 1.2, 42: 1.2, // spell power, spell damage (old items)
	6:  1.0,          // spirit (also feeds Twisted Faith and regeneration)
	30: 0.7, 36: 0.7, // spell haste, haste
	21: 0.6, 32: 0.6, // spell crit, crit
	5:  0.55,         // intellect
	43: 0.5,          // mp5
	7:  0.4,          // stamina
	18: 0.3, 31: 0.3, // spell hit, hit
	41: 0.2,                             // healing power (only heals ourselves)
	0:  0.05, 1: 0.05, 3: 0.05, 4: 0.05, // mana, health, agility, strength
}

const (
	armorWeight = 0.1
	// Weapon damage barely matters: the main hand only gives stats and priests shoot with a wand.
	meleeDPS = 0.15
	wandDPS  = 2.0
)

// Score values an item for the character.
func Score(it *Item) float32 {
	if it == nil {
		return 0
	}
	s := float32(it.Armor) * armorWeight
	for _, st := range it.Stats {
		s += weights[st.Type] * float32(st.Value)
	}
	if it.Class == 2 {
		if it.Subclass == 19 {
			s += it.Dps * wandDPS
		} else {
			s += it.Dps * meleeDPS
		}
	}
	return s
}

// Gain returns the best slot for the item and the score gained by equipping it there
// (a two-hander replaces main and off hand).
func Gain(it *Item, equipped map[int]*Item) (int, float32) {
	best, bestGain := -1, float32(0)
	for _, slot := range Slots(it.InventoryType) {
		cur := Score(equipped[slot])
		if it.InventoryType == invTwoHand {
			cur += Score(equipped[SlotOffHand])
		}
		if slot == SlotOffHand {
			if mh := equipped[SlotMainHand]; mh != nil && mh.InventoryType == invTwoHand {
				cur += Score(mh) // would have to drop the two-hander
			}
		}
		if g := Score(it) - cur; best < 0 || g > bestGain {
			best, bestGain = slot, g
		}
	}
	return best, bestGain
}
