package lockstep

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

var roomIDGen atomic.Int64

// RoomManagerComponent 管理房间元数据。
type RoomManagerComponent struct {
	ecs.BaseComponent
	mu           sync.RWMutex
	rooms        map[int64]*Room
	fiberManager *fiber.Manager
	closed       bool
}

// Room 表示一个房间。
type Room struct {
	ID          int64
	Instance    map[int64]struct{}
	PlayerIds   []int64
	MapActorId  int64
	MapActor    actor.ActorID
	RoomActor   actor.ActorID
	RoomActorId int64
}

// Type 返回组件名称。
func (c *RoomManagerComponent) Type() string { return "RoomManagerComponent" }

// SetFiberManager 注入动态 Room Fiber 所属的 Manager。
func (c *RoomManagerComponent) SetFiberManager(manager *fiber.Manager) {
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

// Awake 初始化内部状态。
func (c *RoomManagerComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.ensureRoomsLocked()
}

func (c *RoomManagerComponent) ensureRoomsLocked() {
	if c.rooms == nil {
		c.rooms = make(map[int64]*Room)
	}
}

// AddRoom 注册房间。
func (c *RoomManagerComponent) AddRoom(id int64) *Room {
	if c == nil || id <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.ensureRoomsLocked()
	room := &Room{ID: id, Instance: make(map[int64]struct{})}
	c.rooms[id] = room
	return room
}

// GetRoom 返回房间。
func (c *RoomManagerComponent) GetRoom(id int64) (*Room, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	room, ok := c.rooms[id]
	return room, ok
}

// FindRoomByActorID 按 Room ActorID 查找房间。
func (c *RoomManagerComponent) FindRoomByActorID(roomActorID int64) (*Room, bool) {
	if c == nil || roomActorID <= 0 {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, room := range c.rooms {
		if room != nil && room.RoomActorId == roomActorID {
			return room, true
		}
	}
	return nil, false
}

// CreateRoom 创建并初始化 Room Fiber，返回地图/房间 ActorID。
func (c *RoomManagerComponent) CreateRoom(playerIds []int64) (int64, int64, error) {
	if err := validateRoomPlayers(playerIds); err != nil {
		return 0, 0, err
	}
	c.Awake()
	c.mu.RLock()
	closed := c.closed
	manager := c.fiberManager
	c.mu.RUnlock()
	if closed {
		return 0, 0, ErrRoomManagerClosed
	}
	id := roomIDGen.Add(1)
	entity := c.GetEntity()
	if entity == nil || entity.Scene() == nil {
		return 0, 0, ErrRoomSceneMissing
	}
	scene := entity.Scene()
	if !sceneActorID(scene).IsValid() {
		return 0, 0, ErrRoomSceneMissing
	}
	if manager == nil {
		return 0, 0, ErrRoomFiberManagerMissing
	}

	mapActorID := scene.InstanceID()
	mapActor := sceneActorID(scene)
	room := &Room{
		ID:          id,
		PlayerIds:   append([]int64(nil), playerIds...),
		Instance:    make(map[int64]struct{}),
		MapActorId:  mapActorID,
		MapActor:    mapActor,
		RoomActorId: 100000 + id,
	}

	processID, ok := processIDForScene(scene)
	if !ok {
		return 0, 0, ErrRoomSceneMissing
	}
	var setupErr error
	var roomServer *RoomServerComponent
	roomFiber := manager.CreateWithSetup(ecs.SceneTypeRoom, scene.Zone(), processID, func(f *fiber.Fiber) error {
		if f == nil || f.Root() == nil {
			setupErr = ErrRoomFiberCreateFailed
			return setupErr
		}
		component, ok := f.Root().GetComponent("RoomServerComponent")
		if !ok || component == nil {
			setupErr = ErrRoomServerMissing
			return setupErr
		}
		var valid bool
		roomServer, valid = component.(*RoomServerComponent)
		if !valid || roomServer == nil {
			setupErr = ErrRoomServerMissing
			return setupErr
		}
		lockstepRoom, err := NewLockstepRoomWithError(playerIds)
		if err != nil {
			setupErr = err
			return err
		}
		roomServer.SetRoom(lockstepRoom)
		roomServer.SetMapActor(mapActor, mapActorID)
		roomServer.SetFiberManager(manager)
		if updaterComponent, ok := f.Root().GetComponent("LSServerUpdater"); ok && updaterComponent != nil {
			updater, valid := updaterComponent.(*LSServerUpdater)
			if !valid || updater == nil {
				setupErr = ErrRoomFiberCreateFailed
				return setupErr
			}
			updater.BindRoom(roomServer.room)
		}
		return nil
	}, func(f *fiber.Fiber, msg fiber.Message) {
		payload, err := actor.DispatchFiberMessage(f.Root(), msg)
		if msg.Reply != nil {
			msg.Reply <- fiber.MessageResponse{
				Payload: payload,
				Err:     err,
			}
		}
	})
	if roomFiber == nil {
		if setupErr != nil {
			return 0, 0, setupErr
		}
		return 0, 0, ErrRoomFiberCreateFailed
	}

	roomScene := roomFiber.Root()
	room.RoomActor = sceneActorID(roomScene)
	if !room.RoomActor.IsValid() {
		manager.Remove(roomFiber.ID())
		return 0, 0, ErrRoomFiberCreateFailed
	}
	room.RoomActorId = roomScene.InstanceID()
	if err := roomServer.InitializationError(); err != nil {
		manager.Remove(roomFiber.ID())
		return 0, 0, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		manager.Remove(roomFiber.ID())
		return 0, 0, ErrRoomManagerClosed
	}
	c.ensureRoomsLocked()
	c.rooms[id] = room
	c.mu.Unlock()
	return room.MapActorId, room.RoomActorId, nil
}

// OnDestroy 停止该地图管理的所有动态 Room Fiber。
func (c *RoomManagerComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	rooms := make([]*Room, 0, len(c.rooms))
	for roomID, room := range c.rooms {
		rooms = append(rooms, room)
		delete(c.rooms, roomID)
	}
	manager := c.fiberManager
	c.mu.Unlock()

	if manager == nil {
		return
	}
	for _, room := range rooms {
		stopRoomFiber(manager, room)
	}
}

// RemoveRoom 删除房间记录。
func (c *RoomManagerComponent) RemoveRoom(roomID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	room := c.rooms[roomID]
	delete(c.rooms, roomID)
	manager := c.fiberManager
	c.mu.Unlock()
	stopRoomFiber(manager, room)
}

func (c *RoomManagerComponent) RemoveRoomByActorID(roomActorID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	var removed *Room
	for roomID, room := range c.rooms {
		if room != nil && room.RoomActorId == roomActorID {
			removed = room
			delete(c.rooms, roomID)
			break
		}
	}
	manager := c.fiberManager
	c.mu.Unlock()
	stopRoomFiber(manager, removed)
}

// RemoveRoomByActor 从地图中删除指定完整 ActorID 的房间。
func (c *RoomManagerComponent) RemoveRoomByActor(roomActor actor.ActorID) {
	_ = c.RemoveRoomByActorIfExists(roomActor)
}

// RemoveRoomByActorIfExists 删除房间并返回是否确实找到该 Room。
func (c *RoomManagerComponent) RemoveRoomByActorIfExists(roomActor actor.ActorID) bool {
	if c == nil || !roomActor.IsValid() {
		return false
	}
	c.mu.Lock()
	var removed *Room
	for roomID, room := range c.rooms {
		if room != nil && room.RoomActor == roomActor {
			removed = room
			delete(c.rooms, roomID)
			break
		}
	}
	manager := c.fiberManager
	c.mu.Unlock()
	stopRoomFiber(manager, removed)
	return removed != nil
}

func stopRoomFiber(manager *fiber.Manager, room *Room) {
	if manager == nil || room == nil || !room.RoomActor.IsValid() {
		return
	}
	go manager.Remove(room.RoomActor.FiberID)
}

func (c *RoomManagerComponent) HandleUnitDisconnect(unitID int64) {
	if c == nil || unitID == 0 {
		return
	}
	entity := c.GetEntity()
	if entity == nil || entity.Scene() == nil {
		return
	}
	scene := entity.Scene()
	if scene == nil {
		return
	}
	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return
	}
	sender, ok := component.(*actor.MessageSender)
	if !ok {
		return
	}
	c.mu.RLock()
	rooms := make([]*Room, 0, len(c.rooms))
	for _, room := range c.rooms {
		rooms = append(rooms, room)
	}
	c.mu.RUnlock()
	for _, room := range rooms {
		if room == nil || !room.RoomActor.IsValid() {
			continue
		}
		if !containsPlayer(room.PlayerIds, unitID) {
			continue
		}
		payload, err := marshalPlayerOffline(&M2RoomPlayerOffline{PlayerId: unitID})
		if err != nil {
			return
		}
		if err := sender.Send(room.RoomActor, MsgM2RoomPlayerOffline, payload); err != nil {
			slog.Error("lockstep notify room player offline failed", "room_actor", room.RoomActor, "player_id", unitID, "err", err)
		}
		return
	}
}

func processIDForScene(scene *ecs.Scene) (int, bool) {
	if scene == nil {
		return 0, false
	}
	fiberRef, ok := scene.Fiber().(interface{ ProcessID() int })
	if !ok || fiberRef.ProcessID() <= 0 {
		return 0, false
	}
	return fiberRef.ProcessID(), true
}

func containsPlayer(playerIDs []int64, playerID int64) bool {
	for _, id := range playerIDs {
		if id == playerID {
			return true
		}
	}
	return false
}

func validateRoomPlayers(playerIDs []int64) error {
	if len(playerIDs) == 0 {
		return ErrRoomPlayersInvalid
	}
	seen := make(map[int64]struct{}, len(playerIDs))
	for _, playerID := range playerIDs {
		if playerID <= 0 {
			return ErrRoomPlayersInvalid
		}
		if _, exists := seen[playerID]; exists {
			return ErrRoomPlayersInvalid
		}
		seen[playerID] = struct{}{}
	}
	return nil
}

func currentSceneActorID(component ecs.Component) int64 {
	if component == nil || component.GetEntity() == nil || component.GetEntity().Scene() == nil {
		return 0
	}
	return component.GetEntity().Scene().InstanceID()
}
