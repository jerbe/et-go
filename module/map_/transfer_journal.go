package map_

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	transferJournalCollection = "map_transfer_transactions"

	TransferTransactionPending        = "pending"
	TransferTransactionCommitted      = "committed"
	TransferTransactionSourceDisposed = "source_disposed"
	TransferTransactionFailed         = "failed"
)

// TransferRecoveryTargetState 是跨进程协调器对目标端最终状态的确认。
//
// Pending/Unknown/NotFound 都不能安全地触发 source cleanup；只有
// Committed 携带有效 recovery token 时，source 才允许进入清理流程。
type TransferRecoveryTargetState string

const (
	TransferRecoveryUnknown   TransferRecoveryTargetState = "unknown"
	TransferRecoveryPending   TransferRecoveryTargetState = "pending"
	TransferRecoveryNotFound  TransferRecoveryTargetState = "not_found"
	TransferRecoveryCommitted TransferRecoveryTargetState = "committed"
	TransferRecoveryFailed    TransferRecoveryTargetState = "failed"
)

// TransferRecoveryResult 是协调器对单笔 source journal 的判断。
type TransferRecoveryResult struct {
	State         TransferRecoveryTargetState
	RecoveryToken string
	Cause         error
}

// TransferRecoveryCoordinator 定义跨进程恢复所需的显式外部协议。
//
// Resolve 必须通过目标 ledger、Location ownership 和请求 digest 得出状态；
// CleanupSource 必须使用 recovery token 做幂等校验后再删除 source Unit。
// Go 核心不提供默认实现，也不会在未注入协调器时猜测目标已成功。
type TransferRecoveryCoordinator interface {
	Resolve(context.Context, TransferTransactionRecord) (TransferRecoveryResult, error)
	CleanupSource(context.Context, TransferTransactionRecord, string) error
}

type transferJournalStore interface {
	Insert(ctx context.Context, entity any, collection string) error
	Query(ctx context.Context, filter bson.M, collection string, results any) error
	Save(ctx context.Context, id int64, entity any, collection string) error
	Remove(ctx context.Context, id int64, collection string) (int64, error)
}

// TransferTransactionRecord 是 Map transfer 的持久化状态。
//
// Pending 表示源端已锁定并准备发送；Committed 表示目标端已完成
// ownership 切换；SourceDisposed 表示源端已经删除旧 Unit。响应丢失时
// 必须保留 Pending/Committed，不能把未知状态伪造成失败。
type TransferTransactionRecord struct {
	Id          int64         `bson:"_id"`
	RpcID       uint32        `bson:"rpc_id"`
	SourceActor actor.ActorID `bson:"source_actor"`
	TargetActor actor.ActorID `bson:"target_actor"`
	TargetMap   string        `bson:"target_map"`
	OldActorID  actor.ActorID `bson:"old_actor_id"`
	UnitID      int64         `bson:"unit_id"`
	Unit        []byte        `bson:"unit"`
	Entitys     [][]byte      `bson:"entitys"`
	Digest      []byte        `bson:"digest"`
	State       string        `bson:"state"`
	LastError   string        `bson:"last_error,omitempty"`
	CreatedAt   time.Time     `bson:"created_at"`
	UpdatedAt   time.Time     `bson:"updated_at"`
}

func (r *TransferTransactionRecord) GetID() int64 {
	if r == nil {
		return 0
	}
	return r.Id
}

func (r *TransferTransactionRecord) CollectionName() string {
	return transferJournalCollection
}

// TransferJournalComponent 持久化 source-side transfer 状态。
//
// Store 可以由测试注入；生产 Map Scene 通过 DBManagerComponent 懒加载
// 当前 Zone 的 DBComponent。没有数据库时返回错误，不回退到内存事务。
type TransferJournalComponent struct {
	ecs.BaseComponent

	Collection   string
	MaxRetries   int
	RetryBackoff time.Duration
	Store        transferJournalStore

	mu     sync.Mutex
	closed bool
}

func (c *TransferJournalComponent) Type() string { return "TransferJournalComponent" }

func (c *TransferJournalComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Collection == "" {
		c.Collection = transferJournalCollection
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultDumpMaxRetries
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = defaultDumpRetryBackoff
	}
}

func (c *TransferJournalComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	c.Store = nil
	c.mu.Unlock()
}

func (c *TransferJournalComponent) Begin(
	ctx context.Context,
	scene *ecs.Scene,
	request *M2MUnitTransferRequest,
	targetActor actor.ActorID,
	targetMap string,
) (*TransferTransactionRecord, error) {
	if c == nil || request == nil || scene == nil || !targetActor.IsValid() || targetMap == "" {
		return nil, ErrTransferRequestInvalid
	}
	if request.RpcID == 0 || !request.OldActorID.IsValid() {
		return nil, ErrTransferRequestInvalid
	}
	sourceActor := actor.SceneActorID(scene)
	if !sourceActor.IsValid() {
		return nil, ErrTransferRequestInvalid
	}
	digest, err := transferRequestDigest(request)
	if err != nil {
		return nil, err
	}
	unitID := transferUnitID(request)
	if unitID <= 0 {
		return nil, ErrTransferRequestInvalid
	}
	store, collection, attempts, backoff, err := c.storeForScene(scene)
	if err != nil {
		return nil, err
	}
	transactionID, err := newTransferTransactionID()
	if err != nil {
		return nil, fmt.Errorf("map_: create transfer transaction id: %w", err)
	}
	now := time.Now()
	record := &TransferTransactionRecord{
		Id:          transactionID,
		RpcID:       request.RpcID,
		SourceActor: sourceActor,
		TargetActor: targetActor,
		TargetMap:   targetMap,
		OldActorID:  request.OldActorID,
		UnitID:      unitID,
		Unit:        append([]byte(nil), request.Unit...),
		Entitys:     cloneByteMatrix(request.Entitys),
		Digest:      append([]byte(nil), digest[:]...),
		State:       TransferTransactionPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := saveWithRetry(ctx, func(callCtx context.Context) error {
		return store.Insert(callCtx, record, collection)
	}, attempts, backoff); err != nil {
		return nil, fmt.Errorf("map_: begin transfer transaction: %w", err)
	}
	return record, nil
}

func (c *TransferJournalComponent) MarkState(
	ctx context.Context,
	scene *ecs.Scene,
	record *TransferTransactionRecord,
	state string,
	lastErr error,
) error {
	if c == nil || record == nil || scene == nil || record.Id <= 0 {
		return ErrTransferRequestInvalid
	}
	switch state {
	case TransferTransactionPending, TransferTransactionCommitted,
		TransferTransactionSourceDisposed, TransferTransactionFailed:
	default:
		return ErrTransferRequestInvalid
	}
	if !validTransferJournalTransition(record.State, state) {
		return fmt.Errorf("%w: %s -> %s", ErrTransferJournalStateInvalid, record.State, state)
	}
	store, collection, attempts, backoff, err := c.storeForScene(scene)
	if err != nil {
		return err
	}
	next := *record
	next.Unit = append([]byte(nil), record.Unit...)
	next.Entitys = cloneByteMatrix(record.Entitys)
	next.Digest = append([]byte(nil), record.Digest...)
	next.State = state
	next.UpdatedAt = time.Now()
	if lastErr != nil {
		next.LastError = lastErr.Error()
	} else {
		next.LastError = ""
	}
	if err := saveWithRetry(ctx, func(callCtx context.Context) error {
		return store.Save(callCtx, next.Id, &next, collection)
	}, attempts, backoff); err != nil {
		return fmt.Errorf("map_: update transfer transaction %d: %w", record.Id, err)
	}
	*record = next
	return nil
}

func validTransferJournalTransition(from, to string) bool {
	switch from {
	case TransferTransactionPending:
		return to == TransferTransactionPending ||
			to == TransferTransactionCommitted ||
			to == TransferTransactionFailed
	case TransferTransactionCommitted:
		return to == TransferTransactionCommitted ||
			to == TransferTransactionSourceDisposed
	case TransferTransactionSourceDisposed:
		return to == TransferTransactionSourceDisposed
	case TransferTransactionFailed:
		return to == TransferTransactionFailed
	default:
		return false
	}
}

// QueryRecoverable 返回仍可能需要恢复的事务。
//
// Recover 已提供严格的 scanner 编排，但目标状态查询、recovery token 生成、
// Location ownership 证明和 source cleanup 仍由调用方的协调器负责；组件不
// 在未知协议下自动猜测。
func (c *TransferJournalComponent) QueryRecoverable(
	ctx context.Context,
	scene *ecs.Scene,
) ([]TransferTransactionRecord, error) {
	if c == nil || scene == nil {
		return nil, ErrTransferRequestInvalid
	}
	store, collection, _, _, err := c.storeForScene(scene)
	if err != nil {
		return nil, err
	}
	var records []TransferTransactionRecord
	if err := store.Query(ctx, bson.M{
		"state": bson.M{"$in": []string{
			TransferTransactionPending,
			TransferTransactionCommitted,
		}},
	}, collection, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// Recover 使用显式协调器收敛可恢复 source journal。
//
// 返回值是本轮处理后仍需人工或下一轮 scanner 继续处理的记录。目标未找到、
// 仍处理中或协调器无法确认时保留原状态；目标已提交时必须先完成带 token
// 的幂等 source cleanup，再将 journal 标记为 SourceDisposed。任一步失败，
// 都保留记录并返回错误，禁止把未知状态变成成功。
func (c *TransferJournalComponent) Recover(
	ctx context.Context,
	scene *ecs.Scene,
	coordinator TransferRecoveryCoordinator,
) ([]TransferTransactionRecord, error) {
	if c == nil || scene == nil {
		return nil, ErrTransferRequestInvalid
	}
	if ctx == nil {
		return nil, ErrTransferRequestInvalid
	}
	if coordinator == nil {
		return nil, ErrTransferRecoveryCoordinatorMissing
	}
	records, err := c.QueryRecoverable(ctx, scene)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Id < records[j].Id })
	unresolved := make([]TransferTransactionRecord, 0, len(records))
	var recoveryErr error
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			recoveryErr = err
			unresolved = append(unresolved, record)
			continue
		}
		if record.Id <= 0 || record.RpcID == 0 || !record.SourceActor.IsValid() ||
			!record.TargetActor.IsValid() || record.UnitID <= 0 ||
			len(record.Digest) != sha256.Size {
			if recoveryErr == nil {
				recoveryErr = ErrTransferRecoveryStateInvalid
			}
			unresolved = append(unresolved, record)
			continue
		}
		result, err := coordinator.Resolve(ctx, record)
		if err != nil {
			if recoveryErr == nil {
				recoveryErr = err
			}
			unresolved = append(unresolved, record)
			continue
		}
		switch result.State {
		case TransferRecoveryUnknown, TransferRecoveryPending, TransferRecoveryNotFound:
			unresolved = append(unresolved, record)
		case TransferRecoveryCommitted:
			if result.RecoveryToken == "" {
				if recoveryErr == nil {
					recoveryErr = ErrTransferRecoveryTokenMissing
				}
				unresolved = append(unresolved, record)
				continue
			}
			if err := coordinator.CleanupSource(ctx, record, result.RecoveryToken); err != nil {
				if recoveryErr == nil {
					recoveryErr = err
				}
				unresolved = append(unresolved, record)
				continue
			}
			next := record
			if err := c.MarkState(ctx, scene, &next, TransferTransactionSourceDisposed, nil); err != nil {
				if recoveryErr == nil {
					recoveryErr = err
				}
				unresolved = append(unresolved, record)
			}
		case TransferRecoveryFailed:
			cause := result.Cause
			if cause == nil {
				cause = ErrTransferRecoveryTargetFailed
			}
			next := record
			if err := c.MarkState(ctx, scene, &next, TransferTransactionFailed, cause); err != nil {
				if recoveryErr == nil {
					recoveryErr = err
				}
				unresolved = append(unresolved, record)
			}
		default:
			if recoveryErr == nil {
				recoveryErr = fmt.Errorf("%w: %q", ErrTransferRecoveryStateInvalid, result.State)
			}
			unresolved = append(unresolved, record)
		}
	}
	return unresolved, recoveryErr
}

func transferJournalForScene(scene *ecs.Scene) (*TransferJournalComponent, error) {
	if scene == nil {
		return nil, ErrTransferRequestInvalid
	}
	component, ok := scene.GetComponent("TransferJournalComponent")
	if !ok || component == nil {
		return nil, nil
	}
	journal, ok := component.(*TransferJournalComponent)
	if !ok || journal == nil {
		return nil, ErrTransferJournalStoreMissing
	}
	return journal, nil
}

func (c *TransferJournalComponent) storeForScene(
	scene *ecs.Scene,
) (transferJournalStore, string, int, time.Duration, error) {
	if c == nil || scene == nil {
		return nil, "", 0, 0, ErrTransferRequestInvalid
	}
	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, "", 0, 0, ErrTransferLedgerClosed
	}
	store := c.Store
	collection := c.Collection
	attempts := c.MaxRetries + 1
	backoff := c.RetryBackoff
	c.mu.Unlock()
	if store != nil {
		return store, collection, attempts, backoff, nil
	}
	component, ok := scene.GetComponent("DBManagerComponent")
	if !ok || component == nil {
		return nil, "", 0, 0, ErrTransferJournalStoreMissing
	}
	manager, ok := component.(*db.DBManagerComponent)
	if !ok || manager == nil {
		return nil, "", 0, 0, ErrTransferJournalStoreMissing
	}
	zoneDB, err := manager.GetZoneDB(scene.Zone())
	if err != nil {
		return nil, "", 0, 0, err
	}
	return zoneDB, collection, attempts, backoff, nil
}

func transferUnitID(request *M2MUnitTransferRequest) int64 {
	if request == nil {
		return 0
	}
	var snapshot unitSnapshot
	if err := bson.Unmarshal(request.Unit, &snapshot); err != nil {
		return 0
	}
	return snapshot.ID
}

func cloneByteMatrix(values [][]byte) [][]byte {
	if values == nil {
		return nil
	}
	result := make([][]byte, len(values))
	for index, value := range values {
		result[index] = append([]byte(nil), value...)
	}
	return result
}

func newTransferTransactionID() (int64, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	id := int64(binary.BigEndian.Uint64(raw[:]) & uint64(^uint64(0)>>1))
	if id <= 0 {
		return 0, fmt.Errorf("map_: generated transfer transaction id is zero")
	}
	return id, nil
}
