
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
ç
state.protowowagent.state.v1api/module/v1/session.protogoogle/protobuf/empty.proto"¨
Aura
slot (Rslot
spell (Rspell
stacks (Rstacks
left_ms (RleftMs
max_ms (RmaxMs
mine (Rmine
positive (Rpositive"Ö
SelfInfo
guid (Rguid
map (Rmap
x (Rx
y (Ry
z (Rz
o (Ro
level (Rlevel
hp (Rhp
max_hp	 (RmaxHp
power
 (Rpower
	max_power (RmaxPower

power_type (R	powerType
xp (Rxp
next_xp (RnextXp
money (Rmoney
target (Rtarget
dead (Rdead
	run_speed (RrunSpeed

unit_flags (R	unitFlags
ghost (Rghost
class_id (RclassId
race (Rrace#
talent_points (RtalentPoints

aura_state (R	auraState-
auras (2.wowagent.state.v1.AuraRauras"ì
Item
slot (Rslot
entry (Rentry
count (Rcount
guid (Rguid
	container (R	container
bag (Rbag
bag_slot (RbagSlot

durability (R
durability%
max_durability	 (RmaxDurability"E
Quest
id (Rid
state (Rstate
counts (Rcounts"ž
SnapshotResponse/
self (2.wowagent.state.v1.SelfInfoRself-
items (2.wowagent.state.v1.ItemRitems0
quests (2.wowagent.state.v1.QuestRquests
objects (Robjects
packets (Rpackets

free_slots (R	freeSlots%
durability_pct (RdurabilityPct"¸
BriefResponse/
self (2.wowagent.state.v1.SelfInfoRself0
quests (2.wowagent.state.v1.QuestRquests

free_slots (R	freeSlots%
durability_pct (RdurabilityPct"á
NearbyRequest
kind (	Rkind
entry (Rentry
radius (Rradius
max (Rmax

alive_only (R	aliveOnly
	npc_flags (RnpcFlags!
hostile_only (RhostileOnly

with_auras (R	withAuras"³
Obj
guid (Rguid
entry (Rentry
kind (	Rkind
x (Rx
y (Ry
z (Rz
dist (Rdist
level (Rlevel
hp	 (Rhp
max_hp
 (RmaxHp
faction (Rfaction

unit_flags (R	unitFlags
	npc_flags (RnpcFlags
	dyn_flags (RdynFlags
target (Rtarget
reaction (	Rreaction

aura_state (R	auraState-
auras (2.wowagent.state.v1.AuraRauras"R
NearbyResponse*
objs (2.wowagent.state.v1.ObjRobjs
total (Rtotal"Y
SetPosRequest
x (Rx
y (Ry
z (Rz
o (Ro
map (Rmap"(
SpellsResponse
spells (Rspells"#
GetObjRequest
guid (Rguid2´
StateG
Snapshot.google.protobuf.Empty#.wowagent.state.v1.SnapshotResponseA
Brief.google.protobuf.Empty .wowagent.state.v1.BriefResponseM
Nearby .wowagent.state.v1.NearbyRequest!.wowagent.state.v1.NearbyResponseB
GetObj .wowagent.state.v1.GetObjRequest.wowagent.state.v1.ObjB
SetPos .wowagent.state.v1.SetPosRequest.google.protobuf.EmptyC
Spells.google.protobuf.Empty!.wowagent.state.v1.SpellsResponseC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.Empty>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyBZwowagent/gen/statev1;statev1bproto3