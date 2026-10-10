// Package sellrule decides which bag items to sell to a vendor and which to keep.
package sellrule

import (
	"sort"

	"google.golang.org/protobuf/proto"

	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/gear"
)

// Item classes.
const (
	classConsumable = 0
	classContainer  = 1
	classWeapon     = 2
	classGem        = 3
	classArmor      = 4
	classReagent    = 5
	classProjectile = 6
	classTrade      = 7
	classRecipe     = 9
	classQuiver     = 11
	classQuest      = 12
	classKey        = 13
	classMisc       = 15

	subFoodDrink = 5
	catFood      = 11
	catDrink     = 59

	// An armor/weapon needing at most this many levels more than ours stays when it would be an upgrade.
	futureLevels = 5
	// Equal-score items are not upgrades.
	upgradeEps = 0.05
	// Consumables whose level is this far below ours are sold (a level-1 snack at level 11...).
	obsoleteGap = 10
)

// Stack is one bag stack.
type Stack struct {
	Entry, Count uint32
	Info         *worldpb.ItemInfo
}

// Ctx is the character the rules are applied for.
type Ctx struct {
	Class, Race, Level uint32
	Known              map[uint32]bool
	Equipped           map[int]*worldpb.ItemInfo
	// QuestItems are item entries required by quests in the log.
	QuestItems map[uint32]bool
}

// Decision is the verdict for one stack.
type Decision struct {
	Stack
	Sell   bool
	Reason string
	Copper uint32
	// rank orders the sales: lower first (gray, then unusable gear, ...).
	rank int
}

// Plan decides every stack; sales are sorted first, in the order they should be made.
func Plan(stacks []Stack, c Ctx) []Decision {
	out := make([]Decision, len(stacks))
	for i, s := range stacks {
		out[i] = decide(s, c)
	}
	markExtraFood(out, c)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Sell != b.Sell {
			return a.Sell
		}
		if a.Sell && a.rank != b.rank {
			return a.rank < b.rank
		}
		return false
	})
	return out
}

func keep(s Stack, reason string) Decision { return Decision{Stack: s, Reason: reason} }

func sell(s Stack, rank int, reason string) Decision {
	return Decision{Stack: s, Sell: true, Reason: reason, rank: rank, Copper: s.Info.SellPrice * s.Count}
}

func decide(s Stack, c Ctx) Decision {
	it := s.Info
	switch {
	case it == nil:
		return keep(s, "unknown item")
	case it.SellPrice == 0:
		return keep(s, "cannot be sold")
	case c.QuestItems[s.Entry] || it.Class == classQuest || it.Bonding >= 4:
		return keep(s, "quest item")
	case it.Quality == 0:
		return sell(s, 0, "gray")
	case it.Quality >= 3:
		return keep(s, "rare or better")
	}
	switch it.Class {
	case classContainer:
		return keep(s, "bag (equipping bags is not automated yet)")
	case classKey:
		return keep(s, "key")
	case classConsumable:
		// Food and drink are trimmed to the best entry by markExtraFood.
		if p := power(it); c.Level >= p+obsoleteGap {
			return sell(s, 4, "obsolete consumable (level "+itoa(p)+")")
		}
		return keep(s, "consumable")
	case classWeapon, classArmor:
		return decideGear(s, c)
	case classReagent:
		return keep(s, "reagent (some spells need them)")
	case classTrade, classRecipe, classGem, classProjectile, classQuiver:
		return sell(s, 3, "trade good / unusable class of item")
	case classMisc:
		if it.Subclass == 0 {
			return sell(s, 3, "junk")
		}
		return keep(s, "miscellaneous (pet, mount...)")
	}
	return keep(s, "unrecognised kind of item")
}

func decideGear(s Stack, c Ctx) Decision {
	it := s.Info
	if len(gear.Slots(it.InventoryType)) == 0 {
		return keep(s, "not equipment")
	}
	if !gear.Usable(it, c.Class, c.Race, c.Level, c.Known) {
		// Too high a level only: keep it when it will be an upgrade soon.
		soon := proto.Clone(it).(*worldpb.ItemInfo)
		soon.RequiredLevel = 0
		if it.RequiredLevel <= c.Level+futureLevels && gear.Usable(soon, c.Class, c.Race, c.Level, c.Known) {
			if _, g := gear.Gain(soon, c.Equipped, gear.ProfileFor(c.Class)); g > upgradeEps {
				return keep(s, "upgrade at level "+itoa(it.RequiredLevel))
			}
		}
		return sell(s, 1, "cannot be used by this class")
	}
	if _, g := gear.Gain(it, c.Equipped, gear.ProfileFor(c.Class)); g > upgradeEps {
		return keep(s, "upgrade (not equipped yet)")
	}
	return sell(s, 2, "no upgrade")
}

// markExtraFood keeps the best food and the best drink (all stacks of that entry) and sells the other kinds.
func markExtraFood(ds []Decision, c Ctx) {
	for _, cat := range []uint32{catFood, catDrink} {
		var best *Decision
		for i := range ds {
			d := &ds[i]
			if !isFood(d, cat) {
				continue
			}
			if best == nil || better(d, best, c.Level) {
				best = d
			}
		}
		if best == nil {
			continue
		}
		for i := range ds {
			d := &ds[i]
			if isFood(d, cat) && d.Entry != best.Entry {
				*d = sell(d.Stack, 4, "extra food/drink")
			}
		}
	}
}

func isFood(d *Decision, cat uint32) bool {
	it := d.Info
	return !d.Sell && it != nil && it.Class == classConsumable && it.Subclass == subFoodDrink && it.SpellCategory == cat && it.SellPrice > 0
}

// better prefers usable food, then the higher required level, item level and stack size.
func better(a, b *Decision, level uint32) bool {
	au, bu := a.Info.RequiredLevel <= level, b.Info.RequiredLevel <= level
	switch {
	case au != bu:
		return au
	case a.Info.RequiredLevel != b.Info.RequiredLevel:
		return a.Info.RequiredLevel > b.Info.RequiredLevel
	case a.Info.ItemLevel != b.Info.ItemLevel:
		return a.Info.ItemLevel > b.Info.ItemLevel
	}
	return a.Count > b.Count
}

// power is the level a consumable is meant for.
func power(it *worldpb.ItemInfo) uint32 {
	if it.RequiredLevel > 0 {
		return it.RequiredLevel
	}
	return it.ItemLevel
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
