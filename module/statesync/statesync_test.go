package statesync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/numeric"
	"github.com/jerbe/et-go/module/unit"
)

type testLocationSender struct {
	ecs.BaseComponent
	mu       sync.Mutex
	messages []sentMessage
}

type sentMessage struct {
	playerID int64
	msgID    uint16
	payload  []byte
}

func (c *testLocationSender) Type() string { return "MessageLocationSenderComponent" }
func (c *testLocationSender) SendToPlayer(playerID int64, msgID uint16, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, sentMessage{playerID: playerID, msgID: msgID, payload: append([]byte(nil), payload...)})
	return nil
}

func (c *testLocationSender) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

func (c *testLocationSender) Messages() []sentMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := make([]sentMessage, len(c.messages))
	copy(copied, c.messages)
	return copied
}

func (c *testLocationSender) Last() (sentMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.messages) == 0 {
		return sentMessage{}, false
	}
	return c.messages[len(c.messages)-1], true
}

type testFinder struct {
	points []etmath.Vector3
}

func (f *testFinder) FindPath(start, target, extents etmath.Vector3, maxPolys int) ([]etmath.Vector3, error) {
	_ = start
	_ = target
	_ = extents
	_ = maxPolys
	return append([]etmath.Vector3(nil), f.points...), nil
}

func TestBroadcastAndAOIHandlers(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	sender := &testLocationSender{}
	scene.AddComponent(sender)
	RegisterAOIHandlers(scene.EventBus())

	player, err := unit.CreatePlayer(scene, 1)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	npc, err := unit.CreateNPC(scene, 2, 2001)
	if err != nil {
		t.Fatalf("CreateNPC error = %v", err)
	}
	npc.AddComponent(aoi.NewAOIEntity(npc.ID(), int(npc.UnitType), 9000))

	playerAOI := mustAOI(t, player)
	npcAOI := mustAOI(t, npc)
	playerAOI.BeSeePlayers[player.ID()] = playerAOI
	playerAOI.BeSeePlayers[npc.ID()] = npcAOI

	BroadcastIncludeSelf(player, &Stop{Id: player.ID()})
	if sender.Count() == 0 {
		t.Fatal("broadcast should send messages")
	}
	first := sender.Messages()[0]
	if first.msgID != MsgStop {
		t.Fatalf("expected MsgStop, got %d", first.msgID)
	}

	scene.EventBus().Publish(aoi.EventUnitEnterSightRange, &aoi.UnitEnterSightRange{A: playerAOI, B: npcAOI})
	scene.EventBus().Publish(aoi.EventUnitLeaveSightRange, &aoi.UnitLeaveSightRange{A: playerAOI, B: npcAOI})
	if sender.Count() < 3 {
		t.Fatal("AOI handlers should send enter and leave messages")
	}
	foundCreate := false
	foundRemove := false
	for _, msg := range sender.Messages() {
		switch msg.msgID {
		case MsgCreateUnits:
			create, err := unmarshalCreateUnits(msg.payload)
			if err != nil {
				t.Fatalf("unmarshal CreateUnits err = %v", err)
			}
			if len(create.Units) != 1 || create.Units[0].UnitId != npc.ID() {
				t.Fatalf("unexpected CreateUnits payload %+v", create)
			}
			foundCreate = true
		case MsgRemoveUnits:
			remove, err := unmarshalRemoveUnits(msg.payload)
			if err != nil {
				t.Fatalf("unmarshal RemoveUnits err = %v", err)
			}
			if len(remove.Units) != 1 || remove.Units[0] != npc.ID() {
				t.Fatalf("unexpected RemoveUnits payload %+v", remove)
			}
			foundRemove = true
		}
	}
	if !foundCreate || !foundRemove {
		t.Fatal("AOI handlers should broadcast CreateUnits and RemoveUnits")
	}
}

func TestHandlePathfindingAndStop(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	sender := &testLocationSender{}
	scene.AddComponent(sender)

	player, err := unit.CreatePlayer(scene, 10)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	moveComponent := mustMove(t, player)
	moveComponent.Bind(player)
	var now atomic.Int64
	moveComponent.SetNowFunc(func() time.Time { return time.UnixMilli(now.Load()) })

	pathfinding := &move.PathfindingComponent{}
	pathfinding.SetFinder(&testFinder{points: []etmath.Vector3{
		player.Position(),
		player.Position().Add(etmath.Vector3{X: 0, Y: 0, Z: 6}),
	}})
	player.AddComponent(pathfinding)

	HandlePathfindingResult(scene, player, &PathfindingResultReq{
		Position: player.Position().Add(etmath.Vector3{X: 0, Y: 0, Z: 6}),
	})
	if sender.Count() == 0 {
		t.Fatal("pathfinding should broadcast result")
	}
	first := sender.Messages()[0]
	if first.msgID != MsgPathfindingResult {
		t.Fatalf("expected MsgPathfindingResult, got %d", first.msgID)
	}
	pathMsg, err := unmarshalPathfindingResult(first.payload)
	if err != nil {
		t.Fatalf("invalid pathfinding payload: %v", err)
	}
	if pathMsg.Id != player.ID() || len(pathMsg.Points) < 2 {
		t.Fatalf("unexpected pathfinding message %+v", pathMsg)
	}
	waitForCondition(t, time.Second, func() bool {
		return !moveComponent.IsArrived()
	})
	now.Store(1000)
	moveComponent.Update()
	waitForCondition(t, time.Second, func() bool {
		return sender.Count() >= 2
	})
	if sender.Count() < 2 {
		t.Fatal("movement completion should broadcast stop")
	}
	if last, ok := sender.Last(); ok {
		if last.msgID != MsgStop {
			t.Fatalf("expected stop message after arrival, got %d", last.msgID)
		}
		stopMsg, err := unmarshalStop(last.payload)
		if err != nil {
			t.Fatalf("invalid stop payload: %v", err)
		}
		if stopMsg.Error != 0 {
			t.Fatalf("expected zero error, got %d", stopMsg.Error)
		}
	}

	HandleStop(scene, player, &StopReq{})
	if sender.Count() < 3 {
		t.Fatal("explicit stop should broadcast stop")
	}
	last, ok := sender.Last()
	if !ok || last.msgID != MsgStop {
		t.Fatal("explicit stop should use MsgStop")
	}

	numericComponent := mustNumeric(t, player)
	numericComponent.SetFloat(numeric.Speed, 0)
	HandlePathfindingResult(scene, player, &PathfindingResultReq{
		Position: etmath.Vector3{X: 1, Y: 0, Z: 1},
	})
	if sender.Count() < 4 {
		t.Fatal("zero speed should send stop error")
	}
	last, ok = sender.Last()
	if !ok || last.msgID != MsgStop {
		t.Fatal("zero speed should send MsgStop")
	}
	stopMsg, err := unmarshalStop(last.payload)
	if err != nil {
		t.Fatalf("invalid stop payload: %v", err)
	}
	if stopMsg.Error != 2 {
		t.Fatalf("expected error=2 for zero speed, got %d", stopMsg.Error)
	}
}

func TestHandleEnterMap(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	sender := &testLocationSender{}
	scene.AddComponent(sender)

	player, err := unit.CreatePlayer(scene, 100)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	other, err := unit.CreateNPC(scene, 101, 2001)
	if err != nil {
		t.Fatalf("CreateNPC error = %v", err)
	}
	otherAOI := aoi.NewAOIEntity(other.ID(), int(other.UnitType), 9000)
	other.AddComponent(otherAOI)
	playerAOI := mustAOI(t, player)
	playerAOI.SeeUnits[other.ID()] = otherAOI

	resp := HandleEnterMap(scene, player, &EnterMap{RpcID: 1})
	if resp.Error != 0 {
		t.Fatalf("HandleEnterMap error = %+v", resp)
	}
	if sender.Count() < 2 {
		t.Fatal("enter map should send self info and visible units")
	}
	msgs := sender.Messages()
	if msgs[0].msgID != MsgCreateMyUnit {
		t.Fatalf("expected MsgCreateMyUnit, got %d", msgs[0].msgID)
	}
	myUnit, err := unmarshalCreateMyUnit(msgs[0].payload)
	if err != nil {
		t.Fatalf("invalid create my unit payload: %v", err)
	}
	if myUnit.Unit == nil || myUnit.Unit.UnitId != player.ID() {
		t.Fatalf("unexpected my unit payload %+v", myUnit)
	}
	if msgs[1].msgID != MsgCreateUnits {
		t.Fatalf("expected MsgCreateUnits, got %d", msgs[1].msgID)
	}
	create, err := unmarshalCreateUnits(msgs[1].payload)
	if err != nil {
		t.Fatalf("invalid create units payload: %v", err)
	}
	if len(create.Units) != 1 || create.Units[0].UnitId != other.ID() {
		t.Fatalf("unexpected create units payload %+v", create)
	}
}

func mustAOI(t *testing.T, u *unit.Unit) *aoi.AOIEntity {
	t.Helper()
	component, ok := u.GetComponent("AOIEntity")
	if !ok {
		t.Fatal("AOIEntity missing")
	}
	return component.(*aoi.AOIEntity)
}

func mustMove(t *testing.T, u *unit.Unit) *move.MoveComponent {
	t.Helper()
	component, ok := u.GetComponent("MoveComponent")
	if !ok {
		t.Fatal("MoveComponent missing")
	}
	return component.(*move.MoveComponent)
}

func mustNumeric(t *testing.T, u *unit.Unit) *numeric.NumericComponent {
	t.Helper()
	component, ok := u.GetComponent("NumericComponent")
	if !ok {
		t.Fatal("NumericComponent missing")
	}
	return component.(*numeric.NumericComponent)
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("wait condition timeout")
}
