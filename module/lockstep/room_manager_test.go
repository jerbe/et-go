package lockstep

import (
	"context"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etlog "github.com/jerbe/et-go/internal/log"
	"github.com/jerbe/et-go/module/login"
	_ "github.com/jerbe/et-go/module/statesync"
	"github.com/jerbe/et-go/module/unit"
	etproto "github.com/jerbe/et-go/proto"
	gproto "google.golang.org/protobuf/proto"
)

func TestRoomManagerBasic(t *testing.T) {
	manager := &RoomManagerComponent{}
	if room := manager.AddRoom(0); room != nil {
		t.Fatal("zero room id should be rejected")
	}
	room := manager.AddRoom(42)
	if room == nil || room.ID != 42 {
		t.Fatalf("AddRoom failed: %+v", room)
	}
	if got, ok := manager.GetRoom(42); !ok || got != room {
		t.Fatalf("GetRoom mismatch: %+v", got)
	}
}

func TestRoomManagerRejectsInvalidPlayers(t *testing.T) {
	manager := &RoomManagerComponent{}
	if _, _, err := manager.CreateRoom(nil); err != ErrRoomPlayersInvalid {
		t.Fatalf("CreateRoom empty players error = %v, want %v", err, ErrRoomPlayersInvalid)
	}
	if _, _, err := manager.CreateRoom([]int64{1, 1}); err != ErrRoomPlayersInvalid {
		t.Fatalf("CreateRoom duplicate players error = %v, want %v", err, ErrRoomPlayersInvalid)
	}
}

func TestRoomManagerHandleUnitDisconnect(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()

	fiberManager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer fiberManager.StopAll()

	sceneFiber := fiber.New(context.Background(), ecs.SceneTypeMap, 1, 1)
	t.Cleanup(sceneFiber.Stop)
	scene := sceneFiber.Root()
	innerSender := actor.NewProcessInnerSender(1, nil, actor.NewRpcManager())
	scene.AddComponent(innerSender)
	scene.AddComponent(actor.NewMessageSender(1, innerSender, nil))
	scene.AddComponent(&unit.UnitComponent{})
	manager := &RoomManagerComponent{}
	manager.SetFiberManager(fiberManager)
	scene.AddComponent(manager)

	player, err := unit.CreatePlayer(scene, 1001)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	if _, _, err := manager.CreateRoom([]int64{1001}); err != nil {
		t.Fatalf("CreateRoom error = %v", err)
	}

	var room *Room
	for _, candidate := range manager.rooms {
		room = candidate
		break
	}
	if room == nil || !room.RoomActor.IsValid() {
		t.Fatalf("room actor missing: %+v", room)
	}

	component, ok := player.GetComponent("MailBox")
	if !ok {
		t.Fatal("mailbox missing")
	}
	mailbox := component.(*actor.MailBox)
	payload, err := gproto.Marshal(&etproto.G2M_SessionDisconnect{RpcId: 1})
	if err != nil {
		t.Fatalf("marshal disconnect err = %v", err)
	}
	if _, err := mailbox.Dispatch(login.MsgG2MSessionDisconnect, payload); err != nil {
		t.Fatalf("dispatch disconnect err = %v", err)
	}

	roomFiber, ok := fiberManager.Get(room.RoomActor.FiberID)
	if !ok || roomFiber == nil {
		t.Fatal("room fiber missing")
	}
	roomComponent, ok := roomFiber.Root().GetComponent("RoomServerComponent")
	if !ok {
		t.Fatal("room server component missing")
	}
	roomServer := roomComponent.(*RoomServerComponent)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !roomServer.PlayerOnline(1001) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("player should be marked offline in room")
}
