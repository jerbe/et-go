package lockstep

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

type RoomServerComponent struct {
	ecs.BaseComponent
	mu               sync.RWMutex
	players          []*RoomPlayer
	playerIndex      map[int64]*RoomPlayer
	room             *LockstepRoom
	initErr          error
	broadcasts       []*Room2CStart
	MapActorId       int64
	mapActor         actor.ActorID
	AllPlayerOffLine bool
	fiberManager     *fiber.Manager
	disposeOnce      sync.Once
	closed           bool
	lateRegistered   bool
}

func NewRoomServerComponent(room *LockstepRoom) *RoomServerComponent {
	return &RoomServerComponent{room: room}
}

func (c *RoomServerComponent) Type() string { return "RoomServerComponent" }

func (c *RoomServerComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	shouldRegister := !c.lateRegistered
	if c.room == nil {
		c.initErr = ErrRoomDefinitionMissing
		c.mu.Unlock()
	} else {
		c.initErr = nil
		if c.room.World == nil {
			c.initErr = ErrLSWorldMissing
		}
		if c.playerIndex == nil {
			c.playerIndex = make(map[int64]*RoomPlayer)
		}
		if c.initErr == nil && len(c.players) == 0 {
			for _, playerID := range c.room.PlayerIds {
				if err := c.addPlayerLocked(NewRoomPlayer(playerID)); err != nil {
					c.initErr = err
					break
				}
			}
		}
		c.mu.Unlock()
	}
	if shouldRegister {
		if entity := c.GetEntity(); entity != nil && entity.Scene() != nil {
			if registrar, ok := entity.Scene().Fiber().(interface {
				RegisterLateUpdate(ecs.LateUpdateSystem)
			}); ok && registrar != nil {
				c.mu.Lock()
				if !c.closed && !c.lateRegistered {
					c.lateRegistered = true
					registrar.RegisterLateUpdate(c)
				}
				c.mu.Unlock()
			}
		}
	}
}

// InitializationError 返回 Room 是否已经显式注入。
func (c *RoomServerComponent) InitializationError() error {
	if c == nil {
		return ErrRoomDefinitionMissing
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initErr
}

// SetRoom 注入动态 Room Fiber 的真实房间定义。
func (c *RoomServerComponent) SetRoom(room *LockstepRoom) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if room == nil {
		c.room = nil
		c.initErr = ErrRoomDefinitionMissing
		return
	}
	if room.World == nil {
		c.room = room
		c.initErr = ErrLSWorldMissing
		return
	}
	if len(room.PlayerIds) > 0 {
		if err := room.SyncWorldPlayers(room.PlayerIds); err != nil {
			c.room = room
			c.initErr = err
			return
		}
	}
	c.room = room
	c.initErr = nil
	if c.playerIndex == nil {
		c.playerIndex = make(map[int64]*RoomPlayer)
	}
	if len(c.players) == 0 {
		for _, playerID := range room.PlayerIds {
			if err := c.addPlayerLocked(NewRoomPlayer(playerID)); err != nil {
				c.initErr = err
				break
			}
		}
	}
}

// SetFiberManager 注入 Room Fiber 的生命周期管理器。
func (c *RoomServerComponent) SetFiberManager(manager *fiber.Manager) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.fiberManager = manager
}

func (c *RoomServerComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	wasRegistered := c.lateRegistered
	c.lateRegistered = false
	players := append([]*RoomPlayer(nil), c.players...)
	c.players = nil
	c.playerIndex = nil
	c.room = nil
	c.initErr = nil
	c.broadcasts = nil
	c.MapActorId = 0
	c.mapActor = actor.ActorID{}
	c.AllPlayerOffLine = false
	c.mu.Unlock()

	if wasRegistered {
		if entity := c.GetEntity(); entity != nil && entity.Scene() != nil {
			if registrar, ok := entity.Scene().Fiber().(interface {
				UnregisterLateUpdate(ecs.LateUpdateSystem)
			}); ok && registrar != nil {
				registrar.UnregisterLateUpdate(c)
			}
		}
	}
	for _, player := range players {
		if player != nil && !player.IsDisposed() {
			player.Dispose()
		}
	}
}

func (c *RoomServerComponent) AddPlayer(player *RoomPlayer) {
	if c == nil || player == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if err := c.addPlayerLocked(player); err != nil {
		c.initErr = err
	}
}

func (c *RoomServerComponent) addPlayerLocked(player *RoomPlayer) error {
	if player == nil {
		return ErrPlayerInvalid
	}
	if c.playerIndex == nil {
		c.playerIndex = make(map[int64]*RoomPlayer)
	}
	if _, ok := c.playerIndex[player.ID()]; ok {
		return nil
	}
	if c.room != nil {
		playerIDs := append([]int64(nil), c.room.PlayerIds...)
		playerIDs = appendUnique(playerIDs, player.ID())
		if err := c.room.SyncWorldPlayers(playerIDs); err != nil {
			return err
		}
	}
	c.players = append(c.players, player)
	c.playerIndex[player.ID()] = player
	if c.room != nil {
		c.room.PlayerIds = appendUnique(c.room.PlayerIds, player.ID())
	}
	return nil
}

// SetPlayerIDs 设置房间初始玩家列表。
func (c *RoomServerComponent) SetPlayerIDs(playerIDs []int64) {
	if c == nil {
		return
	}
	if err := validateRoomPlayers(playerIDs); err != nil {
		c.mu.Lock()
		if !c.closed {
			c.initErr = err
		}
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if c.room == nil {
		c.initErr = ErrRoomDefinitionMissing
		c.mu.Unlock()
		return
	}
	if err := c.room.SyncWorldPlayers(playerIDs); err != nil {
		c.initErr = err
		c.mu.Unlock()
		return
	}

	previous := c.playerIndex
	if previous == nil {
		previous = make(map[int64]*RoomPlayer)
	}
	nextIndex := make(map[int64]*RoomPlayer, len(playerIDs))
	nextPlayers := make([]*RoomPlayer, 0, len(playerIDs))
	removed := make([]*RoomPlayer, 0)
	for _, playerID := range playerIDs {
		player := previous[playerID]
		if player == nil || player.IsDisposed() {
			player = NewRoomPlayer(playerID)
		}
		nextPlayers = append(nextPlayers, player)
		nextIndex[playerID] = player
	}
	for playerID, player := range previous {
		if _, exists := nextIndex[playerID]; !exists && player != nil {
			removed = append(removed, player)
		}
	}
	c.players = nextPlayers
	c.playerIndex = nextIndex
	c.room.PlayerIds = append([]int64(nil), playerIDs...)
	c.initErr = nil
	c.mu.Unlock()

	for _, player := range removed {
		if !player.IsDisposed() {
			player.Dispose()
		}
	}
}

func (c *RoomServerComponent) Player(playerID int64) *RoomPlayer {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if player, ok := c.playerIndex[playerID]; ok {
		return player
	}
	for _, player := range c.players {
		if player != nil && player.ID() == playerID {
			return player
		}
	}
	return nil
}

func (c *RoomServerComponent) PlayerIDs() []int64 {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	playerIDs := make([]int64, 0, len(c.players))
	for _, player := range c.players {
		if player != nil {
			playerIDs = append(playerIDs, player.ID())
		}
	}
	return playerIDs
}

func (c *RoomServerComponent) IsAllPlayerProgress100() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, player := range c.players {
		if player == nil || player.Progress < 100 {
			return false
		}
	}
	return len(c.players) > 0
}

func (c *RoomServerComponent) SetPlayerOnline(playerID int64, online bool) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	player, ok := c.playerIndex[playerID]
	if !ok || player == nil {
		return false
	}
	player.IsOnline = online
	return true
}

func (c *RoomServerComponent) SetPlayerProgress(playerID int64, progress int) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	player, ok := c.playerIndex[playerID]
	if !ok || player == nil {
		return false
	}
	player.Progress = progress
	return true
}

// RestorePlayerState 恢复快照中的玩家连接、进度和 Gate Actor 状态。
func (c *RoomServerComponent) RestorePlayerState(
	playerID int64,
	online bool,
	progress int,
	gateActorID actor.ActorID,
) bool {
	if c == nil || playerID <= 0 || progress < 0 || progress > 100 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	player, ok := c.playerIndex[playerID]
	if !ok || player == nil {
		return false
	}
	player.IsOnline = online
	player.Progress = progress
	player.GateActorID = gateActorID
	return true
}

func (c *RoomServerComponent) restoreSnapshotPlayers(
	room *LockstepRoom,
	playerIDs []int64,
	states []lockstepSnapshotPlayer,
) error {
	if c == nil || room == nil {
		return ErrSnapshotStateInvalid
	}
	if err := validateRoomPlayers(playerIDs); err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshotStateInvalid, err)
	}

	stateByID := make(map[int64]lockstepSnapshotPlayer, len(states))
	for _, state := range states {
		stateByID[state.PlayerID] = state
	}
	if len(stateByID) != len(playerIDs) {
		return fmt.Errorf("%w: snapshot player state count mismatch", ErrSnapshotStateInvalid)
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("%w: room server closed", ErrSnapshotStateInvalid)
	}
	previous := c.playerIndex
	if previous == nil {
		previous = make(map[int64]*RoomPlayer)
	}
	nextIndex := make(map[int64]*RoomPlayer, len(playerIDs))
	nextPlayers := make([]*RoomPlayer, 0, len(playerIDs))
	removed := make([]*RoomPlayer, 0)
	for _, playerID := range playerIDs {
		state, ok := stateByID[playerID]
		if !ok || state.PlayerID <= 0 || state.Progress < 0 || state.Progress > 100 {
			c.mu.Unlock()
			return fmt.Errorf("%w: player %d state invalid", ErrSnapshotStateInvalid, playerID)
		}
		player := previous[playerID]
		if player == nil || player.IsDisposed() {
			player = NewRoomPlayer(playerID)
		}
		player.IsOnline = state.Online
		player.Progress = state.Progress
		player.GateActorID = state.GateActorID
		nextPlayers = append(nextPlayers, player)
		nextIndex[playerID] = player
	}
	for playerID, player := range previous {
		if _, exists := nextIndex[playerID]; !exists && player != nil {
			removed = append(removed, player)
		}
	}

	c.room = room
	c.players = nextPlayers
	c.playerIndex = nextIndex
	c.room.PlayerIds = append([]int64(nil), playerIDs...)
	c.initErr = nil
	c.mu.Unlock()

	for _, player := range removed {
		if !player.IsDisposed() {
			player.Dispose()
		}
	}
	return nil
}

func (c *RoomServerComponent) PlayerOnline(playerID int64) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	player, ok := c.playerIndex[playerID]
	return ok && player != nil && player.IsOnline
}

func (c *RoomServerComponent) StartGame(rpcID uint32) *Room2CStart {
	if c == nil {
		return nil
	}
	msg, err := c.startGame(rpcID)
	if err != nil {
		slog.Error("lockstep start game failed", "err", err)
		return nil
	}
	return msg
}

func (c *RoomServerComponent) startGame(rpcID uint32) (*Room2CStart, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrRoomDefinitionMissing
	}
	if c.room == nil || c.initErr != nil {
		if c.room == nil {
			c.mu.Unlock()
			return nil, ErrRoomDefinitionMissing
		}
		initErr := c.initErr
		c.mu.Unlock()
		return nil, initErr
	}
	playerIDs := append([]int64(nil), c.room.PlayerIds...)
	unitInfos, err := buildUnitInfosFromWorld(c.room.World, playerIDs)
	if err != nil {
		c.initErr = err
		c.mu.Unlock()
		return nil, err
	}
	c.room.StartTime = time.Now().UnixMilli()
	room := c.room
	startTime := c.room.StartTime
	msg := &Room2CStart{
		RpcId:     rpcID,
		StartTime: c.room.StartTime,
		UnitInfos: unitInfos,
	}
	entity := c.GetEntity()
	c.mu.Unlock()

	// 动态 Room Fiber 启动时先保存第 0 帧快照，再通知客户端开始推进。
	// 没有 updater 的独立测试或嵌入场景保留原有行为。
	if entity != nil {
		if component, ok := entity.GetComponent("LSServerUpdater"); ok {
			updater, valid := component.(*LSServerUpdater)
			if !valid || updater == nil {
				return nil, ErrRoomFiberCreateFailed
			}
			if err := updater.CaptureSnapshot(); err != nil {
				c.mu.Lock()
				if c.room == room && c.room.StartTime == startTime {
					c.room.StartTime = 0
				}
				c.mu.Unlock()
				return nil, err
			}
		}
	}

	c.mu.Lock()
	if c.closed || c.room != room || c.room.StartTime != startTime {
		c.mu.Unlock()
		return nil, ErrRoomDefinitionMissing
	}
	c.broadcasts = append(c.broadcasts, msg)
	c.mu.Unlock()

	if entity != nil && entity.Scene() != nil {
		broadcastRoomMessage(entity.Scene(), playerIDs, msg)
	}
	return msg, nil
}

func (c *RoomServerComponent) LastBroadcast() *Room2CStart {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.broadcasts) == 0 {
		return nil
	}
	return c.broadcasts[len(c.broadcasts)-1]
}

func (c *RoomServerComponent) LateUpdate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if len(c.players) == 0 {
		c.mu.Unlock()
		return
	}
	allOffline := true
	for _, player := range c.players {
		if player != nil && player.IsOnline {
			allOffline = false
			break
		}
	}
	shouldDispose := allOffline && !c.AllPlayerOffLine
	c.AllPlayerOffLine = allOffline
	c.mu.Unlock()
	if shouldDispose {
		c.disposeRoom()
	}
}

func (c *RoomServerComponent) SetMapActor(actorID actor.ActorID, mapActorID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.mapActor = actorID
	c.MapActorId = mapActorID
}

func (c *RoomServerComponent) disposeRoom() {
	if c == nil {
		return
	}
	c.disposeOnce.Do(func() {
		c.mu.RLock()
		entity := c.GetEntity()
		mapActor := c.mapActor
		manager := c.fiberManager
		var playerIDs []int64
		if c.room != nil {
			playerIDs = append([]int64(nil), c.room.PlayerIds...)
		}
		c.mu.RUnlock()
		if entity == nil || entity.Scene() == nil {
			return
		}
		scene := entity.Scene()
		if mapActor.IsValid() {
			if !sendRoomMessageToMap(scene, mapActor, &Room2MNotifyRoomDispose{
				RoomActorId: scene.InstanceID(),
				RoomActor:   sceneActorID(scene),
				PlayerIds:   playerIDs,
				DisposeAt:   time.Now().UnixMilli(),
			}) {
				slog.Error("lockstep notify map room dispose failed", "room_actor", scene.InstanceID(), "map_actor", mapActor)
			}
		}

		fiberRef, ok := scene.Fiber().(interface {
			ID() int64
			RequestStop()
		})
		if !ok || fiberRef == nil {
			slog.Error("lockstep room fiber lifecycle missing", "room_actor", scene.InstanceID())
			return
		}
		if manager == nil {
			if owner, ok := scene.Fiber().(interface{ Manager() *fiber.Manager }); ok {
				manager = owner.Manager()
			}
		}
		if manager != nil {
			go manager.Remove(fiberRef.ID())
			return
		}
		fiberRef.RequestStop()
	})
}

func appendUnique(list []int64, value int64) []int64 {
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}
