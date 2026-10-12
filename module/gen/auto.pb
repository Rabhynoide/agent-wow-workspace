
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
…

auto.protowowagent.auto.v1api/module/v1/session.protogoogle/protobuf/empty.proto"Ý

RunRequest
no_progress (R
noProgress
quests (RquestsC
rewards (2).wowagent.auto.v1.RunRequest.RewardsEntryRrewards
wait_s (RwaitS:
RewardsEntry
key (Rkey
value (Rvalue:8"&
StatusRequest
wait_s (RwaitS"u
GrindRequest
entries (Rentries
until_level (R
untilLevel
wait_s (RwaitS
max_s (RmaxS"ï

AutoStatus
state (	Rstate
reason (	Rreason
quest (Rquest
step (	Rstep
done (Rdone
level (Rlevel
xp (Rxp
	elapsed_s (RelapsedS
progress	 (	Rprogress
deaths
 (Rdeaths"f
GotoRequest
x (Rx
y (Ry
z (Rz
radius (Rradius
wait_s (RwaitS2«
AutoH
Recover.wowagent.auto.v1.StatusRequest.wowagent.auto.v1.AutoStatusC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.EmptyA
Run.wowagent.auto.v1.RunRequest.wowagent.auto.v1.AutoStatusE
Grind.wowagent.auto.v1.GrindRequest.wowagent.auto.v1.AutoStatusC
Goto.wowagent.auto.v1.GotoRequest.wowagent.auto.v1.AutoStatusG
Status.wowagent.auto.v1.StatusRequest.wowagent.auto.v1.AutoStatus<
Stop.google.protobuf.Empty.wowagent.auto.v1.AutoStatus>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyBZwowagent/gen/autov1;autov1bproto3