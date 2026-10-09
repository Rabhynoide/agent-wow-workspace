package state

import "github.com/agent-wow/agent-wow/pkg/opcode"

const (
	opUpdateObject     = uint32(opcode.SMSGUpdateObject)
	opCompressedUpdate = uint32(opcode.SMSGCompressedUpdateObject)
	opDestroyObject    = uint32(opcode.SMSGDestroyObject)
	opLoginVerifyWorld = uint32(opcode.SMSGLoginVerifyWorld)
	opNewWorld         = uint32(opcode.SMSGNewWorld)
	opMonsterMove      = uint32(opcode.SMSGMonsterMove)
	opInitialSpells    = uint32(opcode.SMSGInitialSpells)
	opLearnedSpell     = uint32(opcode.SMSGLearnedSpell)
	opSupercededSpell  = uint32(opcode.SMSGSupercededSpell)
	opRemovedSpell     = uint32(opcode.SMSGRemovedSpell)
)
