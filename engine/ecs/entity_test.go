package ecs

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

type lifecycleTestComponent struct {
	BaseComponent
	typeName       string
	events         *[]string
	panicOnDestroy bool
	destroyed      bool
}

func (c *lifecycleTestComponent) Type() string {
	return c.typeName
}

func (c *lifecycleTestComponent) Awake() {
	if c.events != nil {
		*c.events = append(*c.events, c.typeName+":awake")
	}
}

func (c *lifecycleTestComponent) OnDestroy() {
	c.destroyed = true
	if c.events != nil {
		*c.events = append(*c.events, c.typeName+":destroy")
	}
	if c.panicOnDestroy {
		panic("destroy panic")
	}
}

type transferTestComponent struct {
	BaseComponent
	typeName string
	payload  []byte
}

func (c *transferTestComponent) Type() string {
	return c.typeName
}

func (c *transferTestComponent) Transfer() ([]byte, error) {
	if c.payload == nil {
		return nil, errors.New("empty payload")
	}
	return append([]byte(nil), c.payload...), nil
}

func (c *transferTestComponent) OnTransferIn(data []byte) error {
	c.payload = append([]byte(nil), data...)
	return nil
}

func TestEntityStatusFlags(t *testing.T) {
	if StatusNone != 0 {
		t.Fatalf("StatusNone = %d, want 0", StatusNone)
	}
	if StatusIsFromPool != 1 {
		t.Fatalf("StatusIsFromPool = %d, want 1", StatusIsFromPool)
	}
	if StatusIsRegister != 1<<1 {
		t.Fatalf("StatusIsRegister = %d, want %d", StatusIsRegister, 1<<1)
	}
	if StatusIsComponent != 1<<2 {
		t.Fatalf("StatusIsComponent = %d, want %d", StatusIsComponent, 1<<2)
	}
	if StatusIsNew != 1<<3 {
		t.Fatalf("StatusIsNew = %d, want %d", StatusIsNew, 1<<3)
	}
	if StatusIsSerializeWithParent != 1<<4 {
		t.Fatalf("StatusIsSerializeWithParent = %d, want %d", StatusIsSerializeWithParent, 1<<4)
	}

	status := StatusNone.Set(StatusIsNew).Set(StatusIsRegister)
	if !status.Has(StatusIsNew) || !status.Has(StatusIsRegister) {
		t.Fatal("status should contain StatusIsNew and StatusIsRegister")
	}
	if got := status.Clear(StatusIsRegister); got.Has(StatusIsRegister) {
		t.Fatal("status should clear StatusIsRegister")
	}
	if got := status.String(); got != "StatusIsRegister|StatusIsNew" {
		t.Fatalf("String() = %q, want %q", got, "StatusIsRegister|StatusIsNew")
	}
}

func TestNewEntityAssignsUniqueInstanceIDAndStatus(t *testing.T) {
	first := NewEntity()
	second := NewEntity()

	if first.InstanceID() == second.InstanceID() {
		t.Fatal("instance id should be unique")
	}
	if !first.HasStatus(StatusIsNew) {
		t.Fatal("new entity should have StatusIsNew")
	}
}

func TestEntityComponentLifecycle(t *testing.T) {
	entity := NewEntity()
	events := make([]string, 0, 2)
	component := &lifecycleTestComponent{
		typeName: "test",
		events:   &events,
	}

	entity.AddComponent(component)

	if got := component.GetEntity(); got != entity {
		t.Fatal("component entity should be set")
	}
	if len(events) != 1 || events[0] != "test:awake" {
		t.Fatalf("awake events = %v, want [test:awake]", events)
	}

	entity.RemoveComponent("test")

	if !component.destroyed {
		t.Fatal("component should be destroyed on remove")
	}
	if component.GetEntity() != nil {
		t.Fatal("component entity should be cleared on remove")
	}
}

func TestReplacingComponentClearsPreviousEntityReference(t *testing.T) {
	entity := NewEntity()
	previous := &lifecycleTestComponent{typeName: "replace"}
	replacement := &lifecycleTestComponent{typeName: "replace"}

	entity.AddComponent(previous)
	entity.AddComponent(replacement)

	if !previous.destroyed {
		t.Fatal("replaced component should be destroyed")
	}
	if previous.GetEntity() != nil {
		t.Fatal("replaced component entity reference should be cleared")
	}
	if replacement.GetEntity() != entity {
		t.Fatal("replacement component should be attached to entity")
	}
}

func TestAddComponentWithID(t *testing.T) {
	entity := NewEntity()
	component := &lifecycleTestComponent{typeName: "with-id"}

	entity.AddComponentWithID(42, component)

	if got := component.ID(); got != 42 {
		t.Fatalf("component ID = %d, want 42", got)
	}
	if _, ok := entity.GetComponent("with-id"); !ok {
		t.Fatal("component should be stored on entity")
	}
}

func TestAddChildWithIDAndSceneRegistration(t *testing.T) {
	scene := NewScene(SceneTypeMain, 1, "main")
	parent := NewEntity()
	scene.AddChild(parent)

	if _, ok := scene.GetEntity(parent.InstanceID()); !ok {
		t.Fatal("parent should be registered in scene")
	}

	child := NewEntity()
	parent.AddChildWithID(10086, child)

	if child.ID() != 10086 {
		t.Fatalf("child ID = %d, want 10086", child.ID())
	}
	if child.Parent() != parent {
		t.Fatal("child parent should be set")
	}
	if got, ok := scene.GetEntity(child.InstanceID()); !ok || got != child {
		t.Fatal("child should be registered in scene")
	}

	parent.RemoveChild(child.InstanceID())

	if _, ok := scene.GetEntity(child.InstanceID()); ok {
		t.Fatal("child should be unregistered after remove")
	}
	if child.Parent() != nil {
		t.Fatal("child parent should be cleared after remove")
	}
	if child.Scene() != nil {
		t.Fatal("child scene should be cleared after remove")
	}
}

func TestDisposeOrderAndGuards(t *testing.T) {
	scene := NewScene(SceneTypeMain, 1, "main")
	parent := NewEntity()
	scene.AddChild(parent)
	child := NewEntity()
	parent.AddChild(child)

	events := make([]string, 0, 4)
	childComponent := &lifecycleTestComponent{typeName: "child", events: &events}
	panicComponent := &lifecycleTestComponent{typeName: "panic", events: &events, panicOnDestroy: true}
	parentComponent := &lifecycleTestComponent{typeName: "parent", events: &events}

	child.AddComponent(childComponent)
	parent.AddComponent(panicComponent)
	parent.AddComponent(parentComponent)

	parent.Dispose()
	parent.Dispose()

	indexOf := func(target string) int {
		for index, event := range events {
			if event == target {
				return index
			}
		}
		return -1
	}

	childIndex := indexOf("child:destroy")
	panicIndex := indexOf("panic:destroy")
	parentIndex := indexOf("parent:destroy")
	if childIndex == -1 || panicIndex == -1 || parentIndex == -1 {
		t.Fatalf("destroy events missing: %v", events)
	}
	if childIndex > panicIndex || childIndex > parentIndex {
		t.Fatalf("child should destroy before parent components: %v", events)
	}
	if !parent.IsDisposed() || !child.IsDisposed() {
		t.Fatal("dispose should mark entity disposed")
	}
	if _, ok := scene.GetEntity(parent.InstanceID()); ok {
		t.Fatal("disposed parent should be removed from scene")
	}
	if _, ok := scene.GetEntity(child.InstanceID()); ok {
		t.Fatal("disposed child should be removed from scene")
	}

	parent.AddComponent(&lifecycleTestComponent{typeName: "ignored"})
	parent.AddChild(NewEntity())
	if len(parent.components) != 0 {
		t.Fatal("disposed entity should ignore AddComponent")
	}
	if len(parent.children) != 0 {
		t.Fatal("disposed entity should ignore AddChild")
	}
}

func TestGetTransferComponents(t *testing.T) {
	entity := NewEntity()
	transferComponent := &transferTestComponent{
		typeName: "transfer",
		payload:  []byte("payload"),
	}
	plainComponent := &lifecycleTestComponent{typeName: "plain"}

	entity.AddComponent(transferComponent)
	entity.AddComponent(plainComponent)

	components := entity.GetTransferComponents()
	if len(components) != 1 {
		t.Fatalf("transfer component count = %d, want 1", len(components))
	}
	if components[0] != transferComponent {
		t.Fatal("unexpected transfer component")
	}

	data, err := transferComponent.Transfer()
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}

	clone := &transferTestComponent{typeName: "clone"}
	if err := clone.OnTransferIn(data); err != nil {
		t.Fatalf("OnTransferIn() error = %v", err)
	}
	if string(clone.payload) != "payload" {
		t.Fatalf("clone payload = %q, want %q", string(clone.payload), "payload")
	}
}

func TestSceneTypeValuesAndSceneAccessors(t *testing.T) {
	cases := []struct {
		sceneType SceneType
		value     int
		name      string
	}{
		{SceneTypeMain, 1001, "Main"},
		{SceneTypeLaunch, 1002, "Launch"},
		{SceneTypeNetInner, 1003, "NetInner"},
		{SceneTypeNetClient, 1004, "NetClient"},
		{SceneTypeLocation, 3001, "Location"},
		{SceneTypeRouter, 9001, "Router"},
		{SceneTypeRouterNode, 9002, "RouterNode"},
		{SceneTypeRealm, 9003, "Realm"},
		{SceneTypeGate, 9004, "Gate"},
		{SceneTypeLockStep, 11001, "LockStep"},
		{SceneTypeMatch, 11002, "Match"},
		{SceneTypeRoom, 11003, "Room"},
		{SceneTypeHTTP, 16001, "HTTP"},
		{SceneTypeMap, 18001, "Map"},
		{SceneTypeCentral, 20001, "Central"},
	}

	for _, tc := range cases {
		if got := int(tc.sceneType); got != tc.value {
			t.Fatalf("%s value = %d, want %d", tc.name, got, tc.value)
		}
		if got := tc.sceneType.String(); got != tc.name {
			t.Fatalf("%s String() = %q, want %q", tc.name, got, tc.name)
		}
	}

	scene := NewScene(SceneTypeMap, 8, "map-8")
	token := struct{ Name string }{Name: "fiber"}
	scene.SetFiber(token)

	if scene.SceneType() != SceneTypeMap {
		t.Fatalf("scene type = %v, want %v", scene.SceneType(), SceneTypeMap)
	}
	if scene.Zone() != 8 {
		t.Fatalf("scene zone = %d, want 8", scene.Zone())
	}
	if scene.Name() != "map-8" {
		t.Fatalf("scene name = %q, want %q", scene.Name(), "map-8")
	}
	if scene.Fiber() != token {
		t.Fatal("scene fiber should be stored")
	}
	if got, ok := scene.GetEntity(scene.InstanceID()); !ok || got != &scene.Entity {
		t.Fatal("scene root entity should be registered")
	}
}

func TestWorldSingletonLifecycleAndConcurrency(t *testing.T) {
	world := NewWorld()
	world.RegisterSingleton("answer", 42)

	value, ok := world.GetSingleton("answer")
	if !ok || value.(int) != 42 {
		t.Fatal("singleton should be readable after register")
	}

	world.RemoveSingleton("answer")
	if _, ok := world.GetSingleton("answer"); ok {
		t.Fatal("singleton should be removed")
	}

	const goroutines = 32
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", index)
			world.RegisterSingleton(key, index)
			if value, ok := world.GetSingleton(key); ok && value.(int) != index {
				t.Errorf("GetSingleton(%q) = %v, want %d", key, value, index)
			}
			world.RemoveSingleton(key)
		}(i)
	}
	wg.Wait()

	world.Shutdown()
	if _, ok := world.GetSingleton("missing"); ok {
		t.Fatal("shutdown world should not return values")
	}

	world.RegisterSingleton("reinit", 7)
	value, ok = world.GetSingleton("reinit")
	if !ok || value.(int) != 7 {
		t.Fatal("world should allow safe register after shutdown")
	}
}
