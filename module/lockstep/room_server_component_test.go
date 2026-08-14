package lockstep

import (
	"context"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etlog "github.com/jerbe/et-go/internal/log"
)

func TestRoomServerLateUpdate(t *testing.T) {
	room := NewLockstepRoom([]int64{1, 2})
	component := NewRoomServerComponent(room)
	component.Awake()
	if len(component.players) != 2 {
		t.Fatalf("players = %d", len(component.players))
	}
	component.players[0].IsOnline = false
	component.players[1].IsOnline = false
	component.LateUpdate()
	if !component.AllPlayerOffLine {
		t.Fatal("expected all players offline")
	}
}

func TestRoomServerDestroyDisposesPlayersAndDoesNotReopen(t *testing.T) {
	component := NewRoomServerComponent(NewLockstepRoom([]int64{1, 2}))
	component.Awake()
	players := append([]*RoomPlayer(nil), component.players...)

	component.OnDestroy()
	for _, player := range players {
		if player == nil || !player.IsDisposed() {
			t.Fatal("room players should be disposed with room server")
		}
	}

	component.Awake()
	if len(component.players) != 0 {
		t.Fatal("destroyed room server should not reopen players")
	}
}

func TestRoomServerSetPlayerIDsKeepsPlayerStateConsistent(t *testing.T) {
	component := NewRoomServerComponent(NewLockstepRoom([]int64{1, 2}))
	component.Awake()
	if !component.SetPlayerOnline(1, false) {
		t.Fatal("failed to mark player offline")
	}
	component.SetPlayerIDs([]int64{1, 3})

	if component.Player(2) != nil {
		t.Fatal("removed player should not remain indexed")
	}
	if component.Player(3) == nil {
		t.Fatal("new player should be indexed")
	}
	if component.PlayerOnline(1) {
		t.Fatal("existing player state should be preserved")
	}
	if got := component.PlayerIDs(); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("player ids = %v, want [1 3]", got)
	}
	if err := component.InitializationError(); err != nil {
		t.Fatalf("SetPlayerIDs initialization error = %v", err)
	}
}

func TestRoomServerSetPlayerIDsRejectsInvalidPlayers(t *testing.T) {
	component := NewRoomServerComponent(NewLockstepRoom([]int64{1}))
	component.Awake()
	component.SetPlayerIDs([]int64{1, 1})
	if err := component.InitializationError(); err != ErrRoomPlayersInvalid {
		t.Fatalf("invalid player list error = %v, want %v", err, ErrRoomPlayersInvalid)
	}
}

func TestRoomServerOfflineRoomRemovesOwnedFiber(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()

	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer manager.StopAll()

	mapFiber := fiber.New(context.Background(), ecs.SceneTypeMap, 1, 1)
	defer mapFiber.Stop()
	mapScene := mapFiber.Root()
	innerSender := actor.NewProcessInnerSender(1, manager, actor.NewRpcManager())
	mapScene.AddComponent(innerSender)
	mapScene.AddComponent(actor.NewMessageSender(1, innerSender, nil))
	roomManager := &RoomManagerComponent{}
	roomManager.SetFiberManager(manager)
	mapScene.AddComponent(roomManager)

	_, _, err := roomManager.CreateRoom([]int64{1001})
	if err != nil {
		t.Fatalf("CreateRoom error = %v", err)
	}
	var room *Room
	for _, candidate := range roomManager.rooms {
		room = candidate
		break
	}
	if room == nil || !room.RoomActor.IsValid() {
		t.Fatal("room actor not found")
	}
	roomFiber, ok := manager.Get(room.RoomActor.FiberID)
	if !ok || roomFiber == nil {
		t.Fatalf("room fiber not found for actor id %d", room.RoomActor.FiberID)
	}

	componentRaw, ok := roomFiber.Root().GetComponent("RoomServerComponent")
	if !ok || componentRaw == nil {
		t.Fatal("room server component missing")
	}
	component, ok := componentRaw.(*RoomServerComponent)
	if !ok || component == nil {
		t.Fatal("room server component type invalid")
	}
	if !component.SetPlayerOnline(1001, false) {
		t.Fatal("failed to mark player offline")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, exists := manager.Get(roomFiber.ID()); !exists {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("offline room fiber should be removed from manager")
}
