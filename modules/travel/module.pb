
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
é
travel.protowowagent.travel.v1api/module/v1/session.protogoogle/protobuf/empty.proto"C
Point
map (Rmap
x (Rx
y (Ry
z (Rz"ó
Leg
kind (	Rkind-
from (2.wowagent.travel.v1.PointRfrom)
to (2.wowagent.travel.v1.PointRto
nodes (Rnodes
trigger (Rtrigger
	npc_entry (RnpcEntry
seconds (Rseconds
name (	Rname
	transport	 (R	transport-
dock
 (2.wowagent.travel.v1.PointRdock6
	land_dock (2.wowagent.travel.v1.PointRlandDock"N
PlanRequest)
to (2.wowagent.travel.v1.PointRto
level (Rlevel"´
PlanResponse
ok (Rok
reason (	Rreason+
legs (2.wowagent.travel.v1.LegRlegs
seconds (Rseconds!
walk_seconds (RwalkSeconds
busy (Rbusy""

FlyRequest
nodes (Rnodes"2
TriggerRequest
id (Rid
map (Rmap"{
TravelResult
ok (Rok
reason (	Rreason)
at (2.wowagent.travel.v1.PointRat
seconds (Rseconds"%
NearRequest
radius (Rradius"”
NearResponse
found (Rfound
node (Rnode
name (	Rname
	npc_entry (RnpcEntry)
at (2.wowagent.travel.v1.PointRat"§
BoatRequest
	transport (R	transport-
dock (2.wowagent.travel.v1.PointRdock-
pier (2.wowagent.travel.v1.PointRpier

max_wait_s (RmaxWaitS"Q

BoatStatus
state (	Rstate
reason (	Rreason
wait_s (RwaitS"Ÿ
KnownResponse
nodes (Rnodes-
bind (2.wowagent.travel.v1.PointRbind

bind_known (R	bindKnown*
hearth_cooldown_s (RhearthCooldownS2ì
TravelI
Plan.wowagent.travel.v1.PlanRequest .wowagent.travel.v1.PlanResponseD
Discover.google.protobuf.Empty .wowagent.travel.v1.TravelResultG
Fly.wowagent.travel.v1.FlyRequest .wowagent.travel.v1.TravelResultO
Trigger".wowagent.travel.v1.TriggerRequest .wowagent.travel.v1.TravelResult@
Bind.google.protobuf.Empty .wowagent.travel.v1.TravelResultB
Hearth.google.protobuf.Empty .wowagent.travel.v1.TravelResultP
NearUnknown.wowagent.travel.v1.NearRequest .wowagent.travel.v1.NearResponseH
Board.wowagent.travel.v1.BoatRequest.wowagent.travel.v1.BoatStatusL
	Disembark.wowagent.travel.v1.BoatRequest.wowagent.travel.v1.BoatStatusB
Known.google.protobuf.Empty!.wowagent.travel.v1.KnownResponseC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.Empty>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyB Zwowagent/gen/travelv1;travelv1bproto3