
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
Æ
combat.protowowagent.combat.v1api/module/v1/session.protogoogle/protobuf/empty.proto"Ô
KillRequest
entry (Rentry
count (Rcount

item_entry (R	itemEntry

item_count (R	itemCount
radius (Rradius
max_s (RmaxS
wait_s (RwaitS
no_loot (RnoLoot"$
RestRequest
wait_s (RwaitS"&
StatusRequest
wait_s (RwaitS"á
CombatStatus
state (	Rstate
reason (	Rreason
kills (Rkills
items (Ritems
hp_pct (RhpPct
mana_pct (RmanaPct
	elapsed_s (RelapsedS
doing (	Rdoing
coin	 (Rcoin2´
CombatI
Kill.wowagent.combat.v1.KillRequest .wowagent.combat.v1.CombatStatusI
Rest.wowagent.combat.v1.RestRequest .wowagent.combat.v1.CombatStatusM
Status!.wowagent.combat.v1.StatusRequest .wowagent.combat.v1.CombatStatus@
Stop.google.protobuf.Empty .wowagent.combat.v1.CombatStatusC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.Empty>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyB Zwowagent/gen/combatv1;combatv1bproto3