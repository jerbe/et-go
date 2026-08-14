package lockstep

import "errors"

var (
	// ErrRoomSceneMissing 表示 RoomManager 没有关联有效 Map Scene。
	ErrRoomSceneMissing = errors.New("lockstep: room manager scene missing")
	// ErrRoomManagerMissing 表示 Map Scene 缺少 RoomManagerComponent。
	ErrRoomManagerMissing = errors.New("lockstep: room manager component missing")
	// ErrRoomFiberManagerMissing 表示没有注入 Fiber Manager。
	ErrRoomFiberManagerMissing = errors.New("lockstep: fiber manager missing")
	// ErrRoomManagerClosed 表示房间管理器已经关闭。
	ErrRoomManagerClosed = errors.New("lockstep: room manager closed")
	// ErrRoomFiberCreateFailed 表示动态 Room Fiber 创建失败。
	ErrRoomFiberCreateFailed = errors.New("lockstep: room fiber creation failed")
	// ErrRoomServerMissing 表示新建 Room Fiber 缺少 RoomServerComponent。
	ErrRoomServerMissing = errors.New("lockstep: room server component missing")
	// ErrRoomDefinitionMissing 表示 RoomServerComponent 没有注入 Room。
	ErrRoomDefinitionMissing = errors.New("lockstep: room definition missing")
	// ErrRoomIDInvalid 表示房间 ID 非法。
	ErrRoomIDInvalid = errors.New("lockstep: invalid room id")
	// ErrRoomActorMissing 表示房间创建成功但没有完整 Room ActorID。
	ErrRoomActorMissing = errors.New("lockstep: room actor missing")
	// ErrRoomPlayersInvalid 表示房间玩家列表为空、含非法 ID 或重复 ID。
	ErrRoomPlayersInvalid = errors.New("lockstep: invalid room players")
	// ErrSnapshotMissing 表示恢复所需的快照不存在。
	ErrSnapshotMissing = errors.New("lockstep: snapshot missing")
	// ErrSnapshotProviderMissing 表示没有注入真实世界状态序列化器。
	ErrSnapshotProviderMissing = errors.New("lockstep: snapshot provider missing")
	// ErrSnapshotEmpty 表示序列化器返回空快照。
	ErrSnapshotEmpty = errors.New("lockstep: snapshot is empty")
	// ErrSnapshotStateInvalid 表示快照状态结构不合法。
	ErrSnapshotStateInvalid = errors.New("lockstep: snapshot state invalid")
	// ErrReplayMissing 表示 Replay 实例为空。
	ErrReplayMissing = errors.New("lockstep: replay missing")
	// ErrMessageNil 表示协议编码器收到 nil 业务消息。
	ErrMessageNil = errors.New("lockstep: message is nil")
	// ErrMatchRequestInvalid 表示匹配请求缺少有效玩家 ID。
	ErrMatchRequestInvalid = errors.New("lockstep: invalid match request")
	// ErrMatchComponentMissing 表示 Match Scene 缺少 MatchComponent。
	ErrMatchComponentMissing = errors.New("lockstep: match component missing")
	// ErrMatchNotificationMissing 表示 Match 成功通知依赖未配置。
	ErrMatchNotificationMissing = errors.New("lockstep: match notification sender missing")
	// ErrRoomCancelMissing 表示房间回收依赖未配置或回收失败。
	ErrRoomCancelMissing = errors.New("lockstep: room cancel dependency missing")
	// ErrFrameBufferMissing 表示锁步更新器缺少 FrameBuffer。
	ErrFrameBufferMissing = errors.New("lockstep: frame buffer missing")
	// ErrFrameInvalid 表示帧号非法。
	ErrFrameInvalid = errors.New("lockstep: invalid frame")
	// ErrLockstepStateOutOfSync 表示 Room、Updater 和 LSWorld 的帧状态不一致。
	ErrLockstepStateOutOfSync = errors.New("lockstep: room and ls world frame out of sync")
	// ErrPlayerInvalid 表示玩家 ID 非法。
	ErrPlayerInvalid = errors.New("lockstep: invalid player")
	// ErrRoomMessageSenderMissing 表示 Room 消息发送依赖未配置。
	ErrRoomMessageSenderMissing = errors.New("lockstep: room message sender missing")
	// ErrRoomRouteMissing 表示已认证玩家没有有效 Room ActorID。
	ErrRoomRouteMissing = errors.New("lockstep: room route missing")
	// ErrGateMessageSenderMissing 表示 Gate 没有跨 Actor 消息发送器。
	ErrGateMessageSenderMissing = errors.New("lockstep: gate message sender missing")
	// ErrMapSceneAmbiguous 表示当前没有足够的地图选择策略。
	ErrMapSceneAmbiguous = errors.New("lockstep: map scene is ambiguous")
	// ErrMapSceneMissing 表示没有可用地图场景。
	ErrMapSceneMissing = errors.New("lockstep: map scene missing")
	// ErrMapSceneZoneRequired 表示按 Zone 选择地图时缺少有效 Zone。
	ErrMapSceneZoneRequired = errors.New("lockstep: map scene zone required")
	// ErrMapSceneNameRequired 表示按名称选择地图时缺少有效名称。
	ErrMapSceneNameRequired = errors.New("lockstep: map scene name required")
	// ErrMatchSceneAmbiguous 表示当前没有足够的匹配场景选择策略。
	ErrMatchSceneAmbiguous = errors.New("lockstep: match scene is ambiguous")
	// ErrMatchSceneMissing 表示没有可用匹配场景。
	ErrMatchSceneMissing = errors.New("lockstep: match scene missing")
	// ErrMatchSceneZoneRequired 表示按 Zone 选择匹配场景时缺少有效 Zone。
	ErrMatchSceneZoneRequired = errors.New("lockstep: match scene zone required")
	// ErrMatchSceneNameRequired 表示按名称选择匹配场景时缺少有效名称。
	ErrMatchSceneNameRequired = errors.New("lockstep: match scene name required")
)
