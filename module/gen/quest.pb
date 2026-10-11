
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
ò
quest.protowowagent.quest.v1api/module/v1/session.protogoogle/protobuf/empty.proto"K
QuestResult
ok (Rok
reason (	Rreason
items (Ritems"G
AcceptRequest
	npc_entry (RnpcEntry
quest_id (RquestId"_
TurnInRequest
	npc_entry (RnpcEntry
quest_id (RquestId
reward (Rreward"6

UseRequest
entry (Rentry
loot (Rloot"*
ExploreRequest
trigger (Rtrigger"‰
UseItemRequest

item_entry (R	itemEntry!
target_entry (RtargetEntry
target_kind (	R
targetKind
spell (Rspell2†
QuestJ
Accept .wowagent.quest.v1.AcceptRequest.wowagent.quest.v1.QuestResultJ
TurnIn .wowagent.quest.v1.TurnInRequest.wowagent.quest.v1.QuestResultD
Use.wowagent.quest.v1.UseRequest.wowagent.quest.v1.QuestResultL
Explore!.wowagent.quest.v1.ExploreRequest.wowagent.quest.v1.QuestResultL
UseItem!.wowagent.quest.v1.UseItemRequest.wowagent.quest.v1.QuestResultC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.Empty>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyBZwowagent/gen/questv1;questv1bproto3