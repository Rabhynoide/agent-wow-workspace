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

// Profile weighs item stats (ItemModType) for a class.
type Profile struct {
	Name    string
	Weights map[uint32]float32
	Armor   float32 // per armor point
	// Weapon DPS: main/off hand, two-hander, wand, other ranged weapons.
	MeleeDPS, TwoHandDPS, WandDPS, RangedDPS float32
}

// Stat ids: 3 agility, 4 strength, 5 intellect, 6 spirit, 7 stamina, 31 hit, 32 crit, 36 haste,
// 37 expertise, 38 attack power, 44 armor penetration, 45 spell power, 43 mp5; 16-21/28-30 are the
// older melee/ranged/spell variants of hit, crit and haste.
var (
	// Caster (priest, mage, warlock) leveling: spell power > spirit > haste > crit > intellect > stamina > hit.
	Caster = &Profile{Name: "caster", Armor: 0.1, MeleeDPS: 0.15, TwoHandDPS: 0.15, WandDPS: 2.0, RangedDPS: 0,
		Weights: map[uint32]float32{
			45: 1.2, 42: 1.2, 6: 1.0, 30: 0.7, 36: 0.7, 21: 0.6, 32: 0.6, 5: 0.55, 43: 0.5, 7: 0.4, 18: 0.3, 31: 0.3,
			41: 0.2, 0: 0.05, 1: 0.05, 3: 0.05, 4: 0.05,
		}}
	// Strength melee (warrior, paladin, death knight) leveling: weapon damage first (two-handers),
	// then strength, crit, hit, agility, stamina; no caster stats.
	Strength = &Profile{Name: "strength melee", Armor: 0.1, MeleeDPS: 3.0, TwoHandDPS: 3.5, WandDPS: 0, RangedDPS: 0.3,
		Weights: map[uint32]float32{
			4: 1.0, 38: 0.5, 32: 0.8, 19: 0.8, 31: 0.7, 16: 0.7, 37: 0.6, 44: 0.5, 3: 0.6, 7: 0.5, 36: 0.4, 28: 0.4,
			1: 0.05,
		}}
	// Agility melee/ranged (rogue, hunter, feral druid, enhancement shaman).
	Agility = &Profile{Name: "agility", Armor: 0.1, MeleeDPS: 2.5, TwoHandDPS: 2.5, WandDPS: 0, RangedDPS: 2.0,
		Weights: map[uint32]float32{
			3: 1.0, 38: 0.5, 32: 0.8, 19: 0.8, 20: 0.8, 31: 0.7, 16: 0.7, 17: 0.7, 4: 0.5, 44: 0.5, 7: 0.5, 36: 0.4,
			28: 0.4, 5: 0.1, 1: 0.05,
		}}
)

// ProfileFor returns the stat profile of a class id.
func ProfileFor(class uint32) *Profile {
	switch class {
	case 1, 2, 6: // warrior, paladin, death knight
		return Strength
	case 3, 4, 7, 11: // hunter, rogue, shaman, druid
		return Agility
	}
	return Caster // priest, mage, warlock (and unknown)
}

// Score values an item for the profile.
func Score(it *Item, p *Profile) float32 {
	if it == nil {
		return 0
	}
	s := float32(it.Armor) * p.Armor
	for _, st := range it.Stats {
		s += p.Weights[st.Type] * float32(st.Value)
	}
	if it.Class == 2 {
		switch {
		case it.Subclass == 19: // wand
			s += it.Dps * p.WandDPS
		case it.Subclass == 2 || it.Subclass == 3 || it.Subclass == 18 || it.Subclass == 16: // bow, gun, crossbow, thrown
			s += it.Dps * p.RangedDPS
		case it.InventoryType == invTwoHand:
			s += it.Dps * p.TwoHandDPS
		default:
			s += it.Dps * p.MeleeDPS
		}
	}
	return s
}

// Gain returns the best slot for the item and the score gained by equipping it there
// (a two-hander replaces main and off hand).
func Gain(it *Item, equipped map[int]*Item, p *Profile) (int, float32) {
	best, bestGain := -1, float32(0)
	for _, slot := range Slots(it.InventoryType) {
		cur := Score(equipped[slot], p)
		if it.InventoryType == invTwoHand {
			cur += Score(equipped[SlotOffHand], p)
		}
		if slot == SlotOffHand {
			if mh := equipped[SlotMainHand]; mh != nil && mh.InventoryType == invTwoHand {
				cur += Score(mh, p) // would have to drop the two-hander
			}
		}
		if g := Score(it, p) - cur; best < 0 || g > bestGain {
			best, bestGain = slot, g
		}
	}
	return best, bestGain
}
