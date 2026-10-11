// Package itemuse builds CMSG_USE_ITEM, shared by the modules that use bag items (quest items, food).
package itemuse

import (
	statepb "wowagent/gen/statev1"
	"wowagent/internal/wire"
)

// Target flags of SpellCastTargets.
const (
	targetUnit       = 0x2
	targetDest       = 0x40
	targetGameObject = 0x800
)

// Packet is the CMSG_USE_ITEM payload: the item's position and GUID, the cast count (to match
// SMSG_CAST_FAILED), the item's on-use spell (the server checks it) and the target, if any (a unit
// or game object, with its position for spells aimed at a place).
func Packet(item *statepb.Item, spell uint32, castCount uint8, target *statepb.Obj) []byte {
	w := &wire.Writer{}
	w.U8(uint8(item.Bag))
	w.U8(uint8(item.BagSlot))
	w.U8(castCount)
	w.U32(spell)
	w.U64(item.Guid)
	w.U32(0) // glyph index
	w.U8(0)  // cast flags
	if target == nil {
		w.U32(0)
		return w.B
	}
	flag := uint32(targetUnit)
	if target.Kind == "gameobject" {
		flag = targetGameObject
	}
	w.U32(flag | targetDest)
	w.PackedGUID(target.Guid)
	w.PackedGUID(0) // destination transport
	w.F32(target.X)
	w.F32(target.Y)
	w.F32(target.Z)
	return w.B
}
