package map_

import "errors"

var (
	// ErrMapTargetNotFound 表示目标地图不存在。
	ErrMapTargetNotFound = errors.New("map_: target map not found")
	// ErrMapTargetAmbiguous 表示切图请求没有足够信息选择多个目标中的哪一个。
	ErrMapTargetAmbiguous = errors.New("map_: target map is ambiguous")
	// ErrMapTargetInvalid 表示目标地图名称或 ActorID 无效。
	ErrMapTargetInvalid = errors.New("map_: target map is invalid")
	// ErrLocationProxyMissing 表示缺少位置服务组件。
	ErrLocationProxyMissing = errors.New("map_: location proxy component missing")
	// ErrMessageSenderMissing 表示缺少消息发送组件。
	ErrMessageSenderMissing = errors.New("map_: message sender component missing")
	// ErrTransferUnitMissing 表示待转移单位为空。
	ErrTransferUnitMissing = errors.New("map_: transfer unit missing")
	// ErrTransferUnitInvalid 表示单位迁移快照中的身份或类型非法。
	ErrTransferUnitInvalid = errors.New("map_: transfer unit invalid")
	// ErrTransferRequestInvalid 表示地图间转移请求缺少有效关联信息。
	ErrTransferRequestInvalid = errors.New("map_: transfer request invalid")
	// ErrTransferComponentUnsupported 表示迁移载荷包含未注册组件。
	ErrTransferComponentUnsupported = errors.New("map_: transfer component unsupported")
	// ErrTransferComponentInvalid 表示迁移组件 factory 或解码能力无效。
	ErrTransferComponentInvalid = errors.New("map_: transfer component invalid")
	// ErrTransferComponentDuplicate 表示迁移载荷重复包含同一组件类型。
	ErrTransferComponentDuplicate = errors.New("map_: duplicate transfer component")
	// ErrMapManagerMissing 表示地图没有有效的地图元数据组件。
	ErrMapManagerMissing = errors.New("map_: map manager component missing")
	// ErrAOIManagerMissing 表示地图没有有效的 AOI 管理器。
	ErrAOIManagerMissing = errors.New("map_: aoi manager component missing")
	// ErrTransferNotificationMissing 表示切图通知依赖未注入。
	ErrTransferNotificationMissing = errors.New("map_: transfer notification dependency missing")
	// ErrTransferUnitAlreadyExists 表示目标地图已有同 ID 单位。
	ErrTransferUnitAlreadyExists = errors.New("map_: transfer unit already exists")
	// ErrTransferLedgerMissing 表示目标地图没有转移幂等账本。
	ErrTransferLedgerMissing = errors.New("map_: transfer ledger missing")
	// ErrTransferLedgerClosed 表示转移幂等账本已经关闭。
	ErrTransferLedgerClosed = errors.New("map_: transfer ledger closed")
	// ErrTransferLedgerStoreMissing 表示 durable transfer ledger 没有数据库实现。
	ErrTransferLedgerStoreMissing = errors.New("map_: transfer ledger store missing")
	// ErrTransferLedgerRecoveryRequired 表示发现未完成的 durable transfer 记录。
	ErrTransferLedgerRecoveryRequired = errors.New("map_: transfer ledger recovery required")
	// ErrTransferLedgerRecoveryCoordinatorMissing 表示没有注入目标状态协调器。
	ErrTransferLedgerRecoveryCoordinatorMissing = errors.New("map_: transfer ledger recovery coordinator missing")
	// ErrTransferLedgerRecoveryStateInvalid 表示目标协调器返回了非法终态。
	ErrTransferLedgerRecoveryStateInvalid = errors.New("map_: transfer ledger recovery state invalid")
	// ErrTransferLedgerPersistence 表示 durable transfer 终态无法持久化。
	ErrTransferLedgerPersistence = errors.New("map_: transfer ledger persistence failed")
	// ErrTransferJournalStoreMissing 表示持久化 transfer journal 没有数据库实现。
	ErrTransferJournalStoreMissing = errors.New("map_: transfer journal store missing")
	// ErrTransferJournalStateInvalid 表示 source journal 状态迁移不合法。
	ErrTransferJournalStateInvalid = errors.New("map_: transfer journal state invalid")
	// ErrTransferRecoveryCoordinatorMissing 表示没有注入跨进程恢复协调器。
	ErrTransferRecoveryCoordinatorMissing = errors.New("map_: transfer recovery coordinator missing")
	// ErrTransferRecoveryStateInvalid 表示恢复协调器返回了未知状态。
	ErrTransferRecoveryStateInvalid = errors.New("map_: transfer recovery state invalid")
	// ErrTransferRecoveryTokenMissing 表示目标已提交但没有可验证的恢复 token。
	ErrTransferRecoveryTokenMissing = errors.New("map_: transfer recovery token missing")
	// ErrTransferRecoveryTargetFailed 表示目标协调器确认了终态失败。
	ErrTransferRecoveryTargetFailed = errors.New("map_: transfer target recovery failed")
	// ErrTransferCorrelationConflict 表示同一来源和 RpcID 携带了不同载荷。
	ErrTransferCorrelationConflict = errors.New("map_: transfer correlation conflict")
	// ErrMessageNil 表示协议编码器收到 nil 业务消息。
	ErrMessageNil = errors.New("map_: message is nil")
)
