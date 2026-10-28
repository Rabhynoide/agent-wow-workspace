
ä
google/protobuf/any.protogoogle.protobuf"6
Any
type_url (	RtypeUrl
value (RvalueBv
com.google.protobufBAnyProtoPZ,google.golang.org/protobuf/types/known/anypb¢GPBªGoogle.Protobuf.WellKnownTypesbproto3
¾
google/protobuf/empty.protogoogle.protobuf"
EmptyB}
com.google.protobufB
EmptyProtoPZ.google.golang.org/protobuf/types/known/emptypbø¢GPBªGoogle.Protobuf.WellKnownTypesbproto3
§
api/module/v1/session.protoagentwow.module.v1google/protobuf/any.protogoogle/protobuf/empty.proto"?
WorldPacket
opcode (Ropcode
payload (Rpayload"E
SendPacketRequest
opcode (Ropcode
payload (Rpayload"5
ClockResponse$
client_time_ms (RclientTimeMs"u
InvokeModuleRequest
module (	Rmodule
method (	Rmethod.
request (2.google.protobuf.AnyRrequest"D
InvokeModuleResponse,
result (2.google.protobuf.AnyRresult2€
SessionK

SendPacket%.agentwow.module.v1.SendPacketRequest.google.protobuf.EmptyE
GetClock.google.protobuf.Empty!.agentwow.module.v1.ClockResponsea
InvokeModule'.agentwow.module.v1.InvokeModuleRequest(.agentwow.module.v1.InvokeModuleResponseB5Z3github.com/agent-wow/agent-wow/pkg/modules/v1;modv1bproto3
ã

town.protowowagent.town.v1api/module/v1/session.protogoogle/protobuf/empty.proto"^
SellRequest
dry_run (RdryRun
min_free (RminFree
	show_kept (RshowKept"’
SellLine
entry (Rentry
name (	Rname
count (Rcount
copper (Rcopper
action (	Raction
reason (	Rreason"É

SellResult
ok (Rok
reason (	Rreason
sold (Rsold
copper (Rcopper

free_slots (R	freeSlots
vendor (	Rvendor0
lines (2.wowagent.town.v1.SellLineRlines"
RepairResult
ok (Rok
reason (	Rreason
copper (Rcopper%
durability_pct (RdurabilityPct
vendor (	Rvendor"‡
RestockRequest
food (Rfood
drink (Rdrink

max_copper (R	maxCopper
potions (Rpotions
item (Ritem"a
BuyLine
entry (Rentry
name (	Rname
count (Rcount
copper (Rcopper"š
RestockResult
ok (Rok
reason (	Rreason1
bought (2.wowagent.town.v1.BuyLineRbought
copper (Rcopper
vendor (	Rvendor"9
RoomRequest
entry (Rentry
count (Rcount"œ

RoomResult
ok (Rok
reason (	Rreason
	destroyed (R	destroyed)
destroyed_copper (RdestroyedCopper

free_slots (R	freeSlots2­
TownG
SellJunk.wowagent.town.v1.SellRequest.wowagent.town.v1.SellResult@
Repair.google.protobuf.Empty.wowagent.town.v1.RepairResultL
Restock .wowagent.town.v1.RestockRequest.wowagent.town.v1.RestockResultG
MakeRoom.wowagent.town.v1.RoomRequest.wowagent.town.v1.RoomResultC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.Empty>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyBZwowagent/gen/townv1;townv1bproto3