
¾
google/protobuf/empty.protogoogle.protobuf"
EmptyB}
com.google.protobufB
EmptyProtoPZ.google.golang.org/protobuf/types/known/emptypbø¢GPBªGoogle.Protobuf.WellKnownTypesbproto3
€4
world.protowowagent.world.v1google/protobuf/empty.proto"¨
FindNpcRequest
name (	Rname
entry (Rentry

map_filter (R	mapFilter
map (Rmap
x (Rx
y (Ry
radius (Rradius
max (Rmax
	npc_flags	 (RnpcFlags0
sells_spell_category
 (RsellsSpellCategory&
sells_max_level (RsellsMaxLevel"ž
Spawn
entry (Rentry
name (	Rname
	min_level (RminLevel
	max_level (RmaxLevel
	npc_flags (RnpcFlags
faction (Rfaction
map (Rmap
x (Rx
y	 (Ry
z
 (Rz
dist (Rdist
rank (Rrank

health_mod (R	healthMod

damage_mod (R	damageMod
	attack_ms (RattackMs#
movement_type (RmovementType
wander (Rwander"C
FindNpcResponse0
spawns (2.wowagent.world.v1.SpawnRspawns"8
QuestInfoRequest
id (Rid
title (	Rtitle"p
QuestNpc
entry (Rentry
name (	Rname
map (Rmap
x (Rx
y (Ry
z (Rz"â
QuestObjective
kind (	Rkind
entry (Rentry
name (	Rname
count (Rcount
counter (Rcounter
credit_from (R
creditFrom
map (Rmap
x (Rx
y	 (Ry
z
 (Rz
radius (Rradius
use_item (RuseItem
	use_spell (RuseSpell
use_on (RuseOn
use_on_kind (	R	useOnKind"4
ItemStat
type (Rtype
value (Rvalue"Ò
	QuestInfo
id (Rid
title (	Rtitle
level (Rlevel
	min_level (RminLevel 
zone_or_sort (R
zoneOrSort

prev_quest (R	prevQuest

next_quest (R	nextQuest 
reward_xp_id (R
rewardXpId!
reward_money	 (RrewardMoneyA

objectives
 (2!.wowagent.world.v1.QuestObjectiveR
objectives7
starters (2.wowagent.world.v1.QuestNpcRstarters3
enders (2.wowagent.world.v1.QuestNpcRenders'
objectives_text (	RobjectivesText%
reward_choices (RrewardChoices#
special_flags (RspecialFlags!
reward_items (RrewardItems"I
QuestInfoResponse4
quests (2.wowagent.world.v1.QuestInfoRquests"+
QuestsForNpcRequest
entry (Rentry"w
QuestRef
id (Rid
title (	Rtitle
level (Rlevel
	min_level (RminLevel
role (	Rrole"K
QuestsForNpcResponse3
quests (2.wowagent.world.v1.QuestRefRquests"£
SpawnsRequest
entry (Rentry
map (Rmap
x (Rx
y (Ry
radius (Rradius
max (Rmax
kind (	Rkind
all (Rall"B
SpawnsResponse0
spawns (2.wowagent.world.v1.SpawnRspawns";
ItemInfoRequest
entry (Rentry
name (	Rname"å
ItemInfo
entry (Rentry
name (	Rname
quality (Rquality

item_level (R	itemLevel%
required_level (RrequiredLevel
class (Rclass
subclass (Rsubclass%
inventory_type (RinventoryType

sell_price	 (R	sellPrice
	buy_price
 (RbuyPrice
	stackable (R	stackable
armor (Rarmor
dps (Rdps1
stats (2.wowagent.world.v1.ItemStatRstats'
allowable_class (RallowableClass%
allowable_race (RallowableRace%
spell_category (RspellCategory
bonding (Rbonding
	use_spell (RuseSpell

display_id (R	displayId
dmg_min (RdmgMin
dmg_max (RdmgMax
delay_ms (RdelayMs'
container_slots (RcontainerSlots"E
ItemInfoResponse1
items (2.wowagent.world.v1.ItemInfoRitems"+
CreatureInfoRequest
entry (Rentry"¸
CreatureInfo
entry (Rentry
name (	Rname
	min_level (RminLevel
	max_level (RmaxLevel
faction (Rfaction
	npc_flags (RnpcFlags
rank (Rrank
type (Rtype#
skinning_loot	 (RskinningLoot
loot_id
 (RlootId'
detection_range (RdetectionRange"*
VendorItemsRequest
entry (Rentry"_

VendorItem
entry (Rentry
price (Rprice%
required_level (RrequiredLevel"J
VendorItemsResponse3
items (2.wowagent.world.v1.VendorItemRitems"*
ItemSourcesRequest
entry (Rentry"œ

ItemSource
kind (	Rkind
entry (Rentry
name (	Rname
	min_level (RminLevel
	max_level (RmaxLevel
chance (Rchance"N
ItemSourcesResponse7
sources (2.wowagent.world.v1.ItemSourceRsources"q
ClassTrainersRequest
class_id (RclassId
map (Rmap
x (Rx
y (Ry
max (Rmax"v
TrainerSpell
spell (Rspell
cost (Rcost
	req_level (RreqLevel
req_ability (R
reqAbility"à
ClassTrainer
entry (Rentry
name (	Rname
map (Rmap
x (Rx
y (Ry
z (Rz
dist (Rdist

trainer_id (R	trainerId7
spells	 (2.wowagent.world.v1.TrainerSpellRspells"T
ClassTrainersResponse;
trainers (2.wowagent.world.v1.ClassTrainerRtrainers")
GameObjectRequest
entry (Rentry"º
GameObjectInfo
entry (Rentry
name (	Rname
type (Rtype
lock_id (RlockId
loot_id (RlootId
quest_id (RquestId

loot_items (R	lootItems"x
Teleport
id (Rid
name (	Rname
map (Rmap
x (Rx
y (Ry
z (Rz
o (Ro"N
TeleportsResponse9
	teleports (2.wowagent.world.v1.TeleportR	teleports"u
	Transport
entry (Rentry
name (	Rname
path (Rpath
speed (Rspeed
accel (Raccel"R
TransportsResponse<

transports (2.wowagent.world.v1.TransportR
transports2Ú
World\
ItemSources%.wowagent.world.v1.ItemSourcesRequest&.wowagent.world.v1.ItemSourcesResponse\
VendorItems%.wowagent.world.v1.VendorItemsRequest&.wowagent.world.v1.VendorItemsResponseP
FindNpc!.wowagent.world.v1.FindNpcRequest".wowagent.world.v1.FindNpcResponseV
	QuestInfo#.wowagent.world.v1.QuestInfoRequest$.wowagent.world.v1.QuestInfoResponse_
QuestsForNpc&.wowagent.world.v1.QuestsForNpcRequest'.wowagent.world.v1.QuestsForNpcResponseM
Spawns .wowagent.world.v1.SpawnsRequest!.wowagent.world.v1.SpawnsResponseS
ItemInfo".wowagent.world.v1.ItemInfoRequest#.wowagent.world.v1.ItemInfoResponseS
Creature&.wowagent.world.v1.CreatureInfoRequest.wowagent.world.v1.CreatureInfob
ClassTrainers'.wowagent.world.v1.ClassTrainersRequest(.wowagent.world.v1.ClassTrainersResponseU

GameObject$.wowagent.world.v1.GameObjectRequest!.wowagent.world.v1.GameObjectInfoI
	Teleports.google.protobuf.Empty$.wowagent.world.v1.TeleportsResponseK

Transports.google.protobuf.Empty%.wowagent.world.v1.TransportsResponse>
BeforeLogout.google.protobuf.Empty.google.protobuf.EmptyBZwowagent/gen/worldv1;worldv1bproto3