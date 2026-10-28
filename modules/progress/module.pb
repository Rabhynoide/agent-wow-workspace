
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
À
progress.protowowagent.progress.v1api/module/v1/session.protogoogle/protobuf/empty.proto"‹
TrainCheckResult
count (Rcount
cost (Rcost
spells (	Rspells
trainer (Rtrainer!
trainer_name (	RtrainerName
dist (Rdist"
unaffordable (Runaffordable
reason (	Rreason
x	 (Rx
y
 (Ry
z (Rz"%
TrainRequest
wait_s (RwaitS"&
StatusRequest
wait_s (RwaitS"ž
TrainStatus
state (	Rstate
reason (	Rreason
doing (	Rdoing
learned (	Rlearned
spent (Rspent
	elapsed_s (RelapsedS")
TalentsRequest
dry_run (RdryRun"r
TalentsResult
ok (Rok
reason (	Rreason
learned (	Rlearned
points_left (R
pointsLeft"&
GearRequest
dry_run (RdryRun"P

GearResult
ok (Rok
reason (	Rreason
equipped (	Requipped"`
PickRewardRequest
items (Ritems
prices (Rprices

max_copper (R	maxCopper"p
PickRewardResult
index (Rindex
gain (Rgain
replaced (Rreplaced
reason (	Rreason2Ä
ProgressL

TrainCheck.google.protobuf.Empty&.wowagent.progress.v1.TrainCheckResultN
Train".wowagent.progress.v1.TrainRequest!.wowagent.progress.v1.TrainStatusP
Status#.wowagent.progress.v1.StatusRequest!.wowagent.progress.v1.TrainStatusA
Stop.google.protobuf.Empty!.wowagent.progress.v1.TrainStatusT
Talents$.wowagent.progress.v1.TalentsRequest#.wowagent.progress.v1.TalentsResultK
Gear!.wowagent.progress.v1.GearRequest .wowagent.progress.v1.GearResult]

PickReward'.wowagent.progress.v1.PickRewardRequest&.wowagent.progress.v1.PickRewardResultC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.Empty>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyB$Z"wowagent/gen/progressv1;progressv1bproto3