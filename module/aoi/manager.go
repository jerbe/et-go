package aoi

import (
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/event"
	etmath "github.com/jerbe/et-go/engine/math"
)

const eventChangePosition event.EventID = "unit.ChangePosition"

type positionChangeEvent interface {
	ChangedEntity() *ecs.Entity
	ChangedPosition() etmath.Vector3
}

// AOIManagerComponent 管理场景中的 AOI 关系。
type AOIManagerComponent struct {
	ecs.BaseComponent
	cells            map[int64]*Cell
	cancelPositionCB func()
	closed           bool
}

// Type 返回组件名称。
func (m *AOIManagerComponent) Type() string { return "AOIManagerComponent" }

// Awake 初始化管理器并订阅位置变化事件。
func (m *AOIManagerComponent) Awake() {
	if m == nil || m.closed {
		return
	}
	if m.cells == nil {
		m.cells = make(map[int64]*Cell)
	}
	if m.cancelPositionCB != nil {
		return
	}
	scene := m.scene()
	if scene == nil || scene.EventBus() == nil {
		return
	}
	m.cancelPositionCB = scene.EventBus().Subscribe(eventChangePosition, func(args any) {
		change, ok := args.(positionChangeEvent)
		if !ok {
			return
		}
		entity := change.ChangedEntity()
		if entity == nil {
			return
		}
		component, ok := entity.GetComponent("AOIEntity")
		if !ok {
			return
		}
		aoiEntity, ok := component.(*AOIEntity)
		if !ok || aoiEntity == nil {
			return
		}
		pos := change.ChangedPosition()
		aoiEntity.Pos = pos
		if aoiEntity.Cell == nil {
			m.Enter(aoiEntity, pos.X, pos.Z)
			return
		}
		m.Move(aoiEntity, pos.X, pos.Z)
	})
}

// OnDestroy 清理订阅和内部状态。
func (m *AOIManagerComponent) OnDestroy() {
	if m == nil || m.closed {
		return
	}
	m.closed = true
	if m.cancelPositionCB != nil {
		m.cancelPositionCB()
		m.cancelPositionCB = nil
	}
	entities := make(map[int64]*AOIEntity)
	for _, cell := range m.cells {
		if cell == nil {
			continue
		}
		for id, entity := range cell.AOIUnits {
			if entity != nil {
				entities[id] = entity
			}
		}
		for id, entity := range cell.SubsEnterEntities {
			if entity != nil {
				entities[id] = entity
			}
		}
		for id, entity := range cell.SubsLeaveEntities {
			if entity != nil {
				entities[id] = entity
			}
		}
	}
	m.cells = nil
	for _, entity := range entities {
		entity.Cell = nil
		entity.resetVisibility()
	}
}

// Enter 将实体加入 AOI。
func (m *AOIManagerComponent) Enter(entity *AOIEntity, x, z float32) {
	if m == nil || m.closed || entity == nil {
		return
	}
	entity.Awake()
	entity.Pos = etmath.Vector3{X: x, Y: entity.Pos.Y, Z: z}

	id := cellID(x, z)
	if entity.Cell != nil {
		if entity.Cell.ID == id {
			return
		}
		m.Move(entity, x, z)
		return
	}
	cell := m.getOrCreateCell(id)
	cell.AOIUnits[entity.ID] = entity
	entity.Cell = cell

	oldEnter := map[int64]struct{}{}
	oldLeave := map[int64]struct{}{}
	newEnter := rangeCells(cell.X, cell.Z, enterRadius(entity.ViewDistance))
	newLeave := rangeCells(cell.X, cell.Z, leaveRadius(entity.ViewDistance, entity.IsPlayer()))

	m.updateEntitySubscriptions(entity, oldEnter, newEnter, oldLeave, newLeave)

	for watcherID, watcher := range cell.SubsEnterEntities {
		if watcherID == entity.ID || watcher == nil {
			continue
		}
		m.onEntityEnter(watcher, entity)
		m.onEntityEnter(entity, watcher)
	}
}

// Leave 将实体移出 AOI。
func (m *AOIManagerComponent) Leave(entity *AOIEntity) {
	if m == nil || m.closed || entity == nil || entity.Cell == nil {
		return
	}

	cell := entity.Cell
	cx, cz := cellXZ(cell.ID)
	enterCells := rangeCells(cx, cz, enterRadius(entity.ViewDistance))
	leaveCells := rangeCells(cx, cz, leaveRadius(entity.ViewDistance, entity.IsPlayer()))

	for watcherID, watcher := range cell.SubsLeaveEntities {
		if watcherID == entity.ID || watcher == nil {
			continue
		}
		m.onEntityLeave(watcher, entity)
		m.onEntityLeave(entity, watcher)
	}

	for id := range enterCells {
		if target, ok := m.cells[id]; ok {
			delete(target.SubsEnterEntities, entity.ID)
			m.recycleCell(id)
		}
	}
	for id := range leaveCells {
		if target, ok := m.cells[id]; ok {
			delete(target.SubsLeaveEntities, entity.ID)
			m.recycleCell(id)
		}
	}

	delete(cell.AOIUnits, entity.ID)
	entity.Cell = nil
	entity.resetVisibility()
	m.recycleCell(cell.ID)
}

// Move 更新实体在 AOI 中的位置。
func (m *AOIManagerComponent) Move(entity *AOIEntity, newX, newZ float32) {
	if m == nil || m.closed || entity == nil {
		return
	}
	if entity.Cell == nil {
		m.Enter(entity, newX, newZ)
		return
	}

	oldCell := entity.Cell
	oldCID := oldCell.ID
	newCID := cellID(newX, newZ)
	entity.Pos = etmath.Vector3{X: newX, Y: entity.Pos.Y, Z: newZ}
	if oldCID == newCID {
		return
	}

	oldCX, oldCZ := cellXZ(oldCID)
	newCX, newCZ := cellXZ(newCID)
	oldEnter := rangeCells(oldCX, oldCZ, enterRadius(entity.ViewDistance))
	newEnter := rangeCells(newCX, newCZ, enterRadius(entity.ViewDistance))
	oldLeave := rangeCells(oldCX, oldCZ, leaveRadius(entity.ViewDistance, entity.IsPlayer()))
	newLeave := rangeCells(newCX, newCZ, leaveRadius(entity.ViewDistance, entity.IsPlayer()))

	newCell := m.getOrCreateCell(newCID)
	oldEnterWatchers := snapshotEntities(oldCell.SubsEnterEntities)
	oldLeaveWatchers := snapshotEntities(oldCell.SubsLeaveEntities)
	newEnterWatchers := snapshotEntities(newCell.SubsEnterEntities)
	newLeaveWatchers := snapshotEntities(newCell.SubsLeaveEntities)

	delete(oldCell.AOIUnits, entity.ID)
	newCell.AOIUnits[entity.ID] = entity
	entity.Cell = newCell

	m.updateEntitySubscriptions(entity, oldEnter, newEnter, oldLeave, newLeave)

	for id, occupant := range oldLeaveDifferenceOccupants(m.cells, oldLeave, newLeave) {
		if id == entity.ID || occupant == nil {
			continue
		}
		m.onEntityLeave(entity, occupant)
	}
	for id, occupant := range newEnterDifferenceOccupants(m.cells, oldEnter, newEnter) {
		if id == entity.ID || occupant == nil {
			continue
		}
		m.onEntityEnter(entity, occupant)
	}

	for watcherID, watcher := range oldLeaveWatchers {
		if watcherID == entity.ID || watcher == nil {
			continue
		}
		if _, ok := newLeaveWatchers[watcherID]; !ok {
			m.onEntityLeave(watcher, entity)
		}
	}
	for watcherID, watcher := range newEnterWatchers {
		if watcherID == entity.ID || watcher == nil {
			continue
		}
		if _, ok := oldEnterWatchers[watcherID]; !ok {
			m.onEntityEnter(watcher, entity)
		}
	}

	m.recycleCell(oldCID)
}

func (m *AOIManagerComponent) getOrCreateCell(id int64) *Cell {
	if cell, ok := m.cells[id]; ok {
		return cell
	}
	x, z := cellXZ(id)
	cell := NewCell(id, x, z)
	m.cells[id] = cell
	return cell
}

func (m *AOIManagerComponent) updateEntitySubscriptions(entity *AOIEntity, oldEnter, newEnter, oldLeave, newLeave map[int64]struct{}) {
	for id := range oldEnter {
		if _, ok := newEnter[id]; ok {
			continue
		}
		if cell, ok := m.cells[id]; ok {
			delete(cell.SubsEnterEntities, entity.ID)
			m.recycleCell(id)
		}
	}
	for id := range newEnter {
		if _, ok := oldEnter[id]; ok {
			continue
		}
		m.getOrCreateCell(id).SubsEnterEntities[entity.ID] = entity
	}
	for id := range oldLeave {
		if _, ok := newLeave[id]; ok {
			continue
		}
		if cell, ok := m.cells[id]; ok {
			delete(cell.SubsLeaveEntities, entity.ID)
			m.recycleCell(id)
		}
	}
	for id := range newLeave {
		if _, ok := oldLeave[id]; ok {
			continue
		}
		m.getOrCreateCell(id).SubsLeaveEntities[entity.ID] = entity
	}
}

func (m *AOIManagerComponent) onEntityEnter(a, b *AOIEntity) {
	if a == nil || b == nil || a.ID == b.ID {
		return
	}
	a.Awake()
	b.Awake()
	if _, ok := a.SeeUnits[b.ID]; ok {
		return
	}

	a.SeeUnits[b.ID] = b
	b.BeSeeUnits[a.ID] = a
	if b.IsPlayer() {
		a.SeePlayers[b.ID] = b
	}
	if a.IsPlayer() {
		b.BeSeePlayers[a.ID] = a
	}
	m.publishEnterEvent(a, b)
}

func (m *AOIManagerComponent) onEntityLeave(a, b *AOIEntity) {
	if a == nil || b == nil || a.ID == b.ID {
		return
	}
	if _, ok := a.SeeUnits[b.ID]; !ok {
		return
	}

	delete(a.SeeUnits, b.ID)
	delete(b.BeSeeUnits, a.ID)
	if b.IsPlayer() {
		delete(a.SeePlayers, b.ID)
	}
	if a.IsPlayer() {
		delete(b.BeSeePlayers, a.ID)
	}
	m.publishLeaveEvent(a, b)
}

func (m *AOIManagerComponent) publishEnterEvent(a, b *AOIEntity) {
	scene := m.scene()
	if scene == nil || scene.EventBus() == nil {
		return
	}
	scene.EventBus().Publish(EventUnitEnterSightRange, &UnitEnterSightRange{A: a, B: b})
}

func (m *AOIManagerComponent) publishLeaveEvent(a, b *AOIEntity) {
	scene := m.scene()
	if scene == nil || scene.EventBus() == nil {
		return
	}
	scene.EventBus().Publish(EventUnitLeaveSightRange, &UnitLeaveSightRange{A: a, B: b})
}

func (m *AOIManagerComponent) recycleCell(id int64) {
	cell, ok := m.cells[id]
	if !ok || cell == nil || !cell.Empty() {
		return
	}
	delete(m.cells, id)
}

func (m *AOIManagerComponent) scene() *ecs.Scene {
	entity := m.GetEntity()
	if entity == nil {
		return nil
	}
	return entity.Scene()
}

func enterRadius(viewDistance int) int {
	if viewDistance <= 0 {
		return 0
	}
	return (viewDistance-1)/CellSize + 1
}

func leaveRadius(viewDistance int, isPlayer bool) int {
	radius := enterRadius(viewDistance)
	if isPlayer {
		radius++
	}
	return radius
}

func rangeCells(cx, cz int32, radius int) map[int64]struct{} {
	result := make(map[int64]struct{})
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			result[makeCellID(cx+int32(dx), cz+int32(dz))] = struct{}{}
		}
	}
	return result
}

func snapshotEntities(source map[int64]*AOIEntity) map[int64]*AOIEntity {
	result := make(map[int64]*AOIEntity, len(source))
	for id, entity := range source {
		result[id] = entity
	}
	return result
}

func oldLeaveDifferenceOccupants(cells map[int64]*Cell, oldLeave, newLeave map[int64]struct{}) map[int64]*AOIEntity {
	result := make(map[int64]*AOIEntity)
	for id := range oldLeave {
		if _, ok := newLeave[id]; ok {
			continue
		}
		if cell, ok := cells[id]; ok {
			for entityID, entity := range cell.AOIUnits {
				result[entityID] = entity
			}
		}
	}
	return result
}

func newEnterDifferenceOccupants(cells map[int64]*Cell, oldEnter, newEnter map[int64]struct{}) map[int64]*AOIEntity {
	result := make(map[int64]*AOIEntity)
	for id := range newEnter {
		if _, ok := oldEnter[id]; ok {
			continue
		}
		if cell, ok := cells[id]; ok {
			for entityID, entity := range cell.AOIUnits {
				result[entityID] = entity
			}
		}
	}
	return result
}
