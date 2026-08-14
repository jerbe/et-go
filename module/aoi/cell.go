package aoi

// CellSize 网格单元大小。
const CellSize = 10000

// Cell 网格单元。
type Cell struct {
	ID                int64
	X                 int32
	Z                 int32
	AOIUnits          map[int64]*AOIEntity
	SubsEnterEntities map[int64]*AOIEntity
	SubsLeaveEntities map[int64]*AOIEntity
}

// NewCell 创建新的 Cell。
func NewCell(id int64, x, z int32) *Cell {
	return &Cell{
		ID:                id,
		X:                 x,
		Z:                 z,
		AOIUnits:          make(map[int64]*AOIEntity),
		SubsEnterEntities: make(map[int64]*AOIEntity),
		SubsLeaveEntities: make(map[int64]*AOIEntity),
	}
}

// Empty 返回 Cell 是否已无任何引用。
func (c *Cell) Empty() bool {
	if c == nil {
		return true
	}
	return len(c.AOIUnits) == 0 && len(c.SubsEnterEntities) == 0 && len(c.SubsLeaveEntities) == 0
}

func cellID(x, z float32) int64 {
	cx := int32(int(x*1000) / CellSize)
	cz := int32(int(z*1000) / CellSize)
	return makeCellID(cx, cz)
}

func cellXZ(id int64) (int32, int32) {
	return int32(id >> 32), int32(id)
}

func makeCellID(cx, cz int32) int64 {
	return int64(cx)<<32 | int64(uint32(cz))
}
