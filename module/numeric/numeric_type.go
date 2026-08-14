package numeric

// NumericTypeMax 子属性阈值。类型值 >= NumericTypeMax 时表示子属性。
const NumericTypeMax = 10000

// FloatMultiplier 浮点值存储倍率。
const FloatMultiplier = 10000

// 子属性偏移量。
const (
	SubBase     = 1
	SubAdd      = 2
	SubPct      = 3
	SubFinalAdd = 4
	SubFinalPct = 5
)

// 速度属性。
const (
	Speed         = 1000
	SpeedBase     = Speed*10 + SubBase
	SpeedAdd      = Speed*10 + SubAdd
	SpeedPct      = Speed*10 + SubPct
	SpeedFinalAdd = Speed*10 + SubFinalAdd
	SpeedFinalPct = Speed*10 + SubFinalPct
)

// 生命值属性。
const (
	Hp         = 1001
	HpBase     = Hp*10 + SubBase
	HpAdd      = Hp*10 + SubAdd
	HpPct      = Hp*10 + SubPct
	HpFinalAdd = Hp*10 + SubFinalAdd
	HpFinalPct = Hp*10 + SubFinalPct
)

// 最大生命值属性。
const (
	MaxHp         = 1002
	MaxHpBase     = MaxHp*10 + SubBase
	MaxHpAdd      = MaxHp*10 + SubAdd
	MaxHpPct      = MaxHp*10 + SubPct
	MaxHpFinalAdd = MaxHp*10 + SubFinalAdd
	MaxHpFinalPct = MaxHp*10 + SubFinalPct
)

// AOI 视距属性。
const (
	AOI         = 1003
	AOIBase     = AOI*10 + SubBase
	AOIAdd      = AOI*10 + SubAdd
	AOIPct      = AOI*10 + SubPct
	AOIFinalAdd = AOI*10 + SubFinalAdd
	AOIFinalPct = AOI*10 + SubFinalPct
)
