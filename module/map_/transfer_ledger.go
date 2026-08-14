package map_

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
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
	defaultTransferLedgerTTL        = 10 * time.Minute
	defaultTransferLedgerMaxEntries = 4096
)

type transferLedgerKey struct {
	oldActorID actor.ActorID
	rpcID      uint32
}

type transferLedgerEntry struct {
	key         transferLedgerKey
	digest      [sha256.Size]byte
	recordID    int64
	response    M2MUnitTransferResponse
	createdAt   time.Time
	completedAt time.Time
	done        chan struct{}
	completed   bool
}

const (
	transferLedgerCollection = "map_transfer_ledger"
	transferLedgerProcessing = "processing"
	transferLedgerCompleted  = "completed"
	transferLedgerFailed     = "failed"
)

type transferLedgerRecord struct {
	Id          int64                   `bson:"_id"`
	RpcID       uint32                  `bson:"rpc_id"`
	OldActorID  actor.ActorID           `bson:"old_actor_id"`
	TargetActor actor.ActorID           `bson:"target_actor"`
	TargetMap   string                  `bson:"target_map"`
	UnitID      int64                   `bson:"unit_id"`
	Digest      []byte                  `bson:"digest"`
	State       string                  `bson:"state"`
	Response    M2MUnitTransferResponse `bson:"response"`
	CreatedAt   time.Time               `bson:"created_at"`
	UpdatedAt   time.Time               `bson:"updated_at"`
}

// TransferLedgerProcessingRecord 是供恢复协调器读取的 processing 记录。
type TransferLedgerProcessingRecord struct {
	ID          int64
	RpcID       uint32
	OldActorID  actor.ActorID
	TargetActor actor.ActorID
	TargetMap   string
	UnitID      int64
	Digest      []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r transferLedgerRecord) processingRecord() TransferLedgerProcessingRecord {
	return TransferLedgerProcessingRecord{
		ID:          r.Id,
		RpcID:       r.RpcID,
		OldActorID:  r.OldActorID,
		TargetActor: r.TargetActor,
		TargetMap:   r.TargetMap,
		UnitID:      r.UnitID,
		Digest:      append([]byte(nil), r.Digest...),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// TransferLedgerRecoveryState 是目标端对 processing 记录的终态判断。
type TransferLedgerRecoveryState string

const (
	TransferLedgerRecoveryUnknown   TransferLedgerRecoveryState = "unknown"
	TransferLedgerRecoveryPending   TransferLedgerRecoveryState = "pending"
	TransferLedgerRecoveryNotFound  TransferLedgerRecoveryState = "not_found"
	TransferLedgerRecoveryCommitted TransferLedgerRecoveryState = "committed"
	TransferLedgerRecoveryFailed    TransferLedgerRecoveryState = "failed"
)

// TransferLedgerRecoveryResult 是目标恢复协调器返回的终态。
type TransferLedgerRecoveryResult struct {
	State    TransferLedgerRecoveryState
	Response M2MUnitTransferResponse
	Cause    error
}

// TransferLedgerRecoveryCoordinator 只负责证明目标端副作用的最终状态。
//
// 它不能由 Go ledger 自动推断；Resolve 必须结合目标 Unit、Location ownership、
// request digest 和部署层幂等记录返回结果。RecoverProcessing 只持久化终态和
// 唤醒本进程等待者，不执行 Unit/AOI/Location 操作。
type TransferLedgerRecoveryCoordinator interface {
	Resolve(context.Context, TransferLedgerProcessingRecord) (TransferLedgerRecoveryResult, error)
}

func (r *transferLedgerRecord) GetID() int64 {
	if r == nil {
		return 0
	}
	return r.Id
}

// TransferLedgerComponent 为目标 Map 记录已经处理过的 transfer 请求。
//
// 进程内请求使用内存条目等待同一个结果；配置了 DBManager 或显式 Store
// 时，processing/completed/failed 状态还会写入 durable ledger。发现跨进程
// 未完成的 processing 记录时，当前实现拒绝重复创建 Unit；RecoverProcessing
// 已提供只收敛 durable terminal response 的严格编排，不执行 Unit/AOI/Location
// 副作用。
//
// TODO(distributed): 仍需部署层提供能证明目标副作用和 Location ownership
// 的 TransferLedgerRecoveryCoordinator，并与 source journal 的 token 协议
// 对接；没有 Store 时也不会伪造跨进程恢复能力。
type TransferLedgerComponent struct {
	ecs.BaseComponent

	TTL            time.Duration
	MaxEntries     int
	MaxRetries     int
	RetryBackoff   time.Duration
	Store          transferJournalStore
	RequireDurable bool

	mu      sync.Mutex
	entries map[transferLedgerKey]*transferLedgerEntry
	closed  bool
}

func (c *TransferLedgerComponent) Type() string { return "TransferLedgerComponent" }

func (c *TransferLedgerComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.TTL <= 0 {
		c.TTL = defaultTransferLedgerTTL
	}
	if c.MaxEntries <= 0 {
		c.MaxEntries = defaultTransferLedgerMaxEntries
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
	if c.entries == nil {
		c.entries = make(map[transferLedgerKey]*transferLedgerEntry)
	}
}

func (c *TransferLedgerComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	entries := make([]*transferLedgerEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry == nil || entry.completed {
			continue
		}
		entry.response = transferErrorResponse(
			&M2MUnitTransferRequest{
				RpcID:      entry.key.rpcID,
				OldActorID: entry.key.oldActorID,
			},
			ErrTransferLedgerClosed,
		)
		entry.completed = true
		entry.completedAt = time.Now()
		entries = append(entries, entry)
	}
	c.entries = nil
	c.mu.Unlock()

	for _, entry := range entries {
		if entry == nil {
			continue
		}
		close(entry.done)
	}
}

type transferLedgerHandle struct {
	entry *transferLedgerEntry
	owner bool
}

func (c *TransferLedgerComponent) begin(scene *ecs.Scene, req *M2MUnitTransferRequest) (*transferLedgerHandle, error) {
	if c == nil || scene == nil || req == nil {
		return nil, ErrTransferLedgerMissing
	}
	key := transferLedgerKey{oldActorID: req.OldActorID, rpcID: req.RpcID}
	digest, err := transferRequestDigest(req)
	if err != nil {
		return nil, err
	}
	unitID := transferUnitID(req)

	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrTransferLedgerClosed
	}
	if c.entries == nil {
		c.entries = make(map[transferLedgerKey]*transferLedgerEntry)
	}
	c.evictExpiredLocked(time.Now())
	if existing := c.entries[key]; existing != nil {
		if existing.digest != digest {
			c.mu.Unlock()
			return nil, ErrTransferCorrelationConflict
		}
		c.mu.Unlock()
		return &transferLedgerHandle{entry: existing}, nil
	}
	durable := c.RequireDurable || c.Store != nil
	now := time.Now()
	entry := &transferLedgerEntry{
		key:       key,
		digest:    digest,
		createdAt: now,
		done:      make(chan struct{}),
	}
	c.entries[key] = entry
	c.evictOverflowLocked()
	c.mu.Unlock()

	if durable {
		store, collection, attempts, backoff, err := c.storeForScene(scene)
		if err != nil {
			c.abortEntry(entry, req, err)
			return nil, err
		}
		recordID, err := transferLedgerRecordID(key, digest)
		if err != nil {
			c.abortEntry(entry, req, err)
			return nil, err
		}
		c.mu.Lock()
		entry.recordID = recordID
		closed := c.closed
		c.mu.Unlock()
		if closed {
			c.abortEntry(entry, req, ErrTransferLedgerClosed)
			return nil, ErrTransferLedgerClosed
		}
		record, found, err := queryTransferLedgerRecord(context.Background(), store, collection, recordID)
		if err != nil {
			c.abortEntry(entry, req, err)
			return nil, err
		}
		if found {
			if !bytes.Equal(record.Digest, digest[:]) {
				c.abortEntry(entry, req, ErrTransferCorrelationConflict)
				return nil, ErrTransferCorrelationConflict
			}
			switch record.State {
			case transferLedgerCompleted, transferLedgerFailed:
				c.restoreDurableEntry(entry, record)
				return &transferLedgerHandle{entry: entry}, nil
			case transferLedgerProcessing:
				// 另一个进程可能已经完成了目标侧副作用；在没有
				// recovery scanner 查询最终状态前禁止重复创建 Unit。
				c.abortEntry(entry, req, ErrTransferLedgerRecoveryRequired)
				return nil, ErrTransferLedgerRecoveryRequired
			default:
				err := fmt.Errorf("%w: unknown durable state %q", ErrTransferLedgerRecoveryRequired, record.State)
				c.abortEntry(entry, req, err)
				return nil, err
			}
		}
		record = &transferLedgerRecord{
			Id:          recordID,
			RpcID:       req.RpcID,
			OldActorID:  req.OldActorID,
			TargetActor: actor.SceneActorID(scene),
			TargetMap:   scene.Name(),
			UnitID:      unitID,
			Digest:      append([]byte(nil), digest[:]...),
			State:       transferLedgerProcessing,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := saveWithRetry(context.Background(), func(callCtx context.Context) error {
			return store.Insert(callCtx, record, collection)
		}, attempts, backoff); err != nil {
			// Insert 竞争通常表示另一个进程已经创建了同一条
			// processing 记录。重新读取后再决定是复用终态还是
			// 明确要求 recovery，不能把竞争错误直接当成普通失败。
			existing, found, queryErr := queryTransferLedgerRecord(
				context.Background(),
				store,
				collection,
				recordID,
			)
			if queryErr == nil && found {
				if !bytes.Equal(existing.Digest, digest[:]) {
					c.abortEntry(entry, req, ErrTransferCorrelationConflict)
					return nil, ErrTransferCorrelationConflict
				}
				switch existing.State {
				case transferLedgerCompleted, transferLedgerFailed:
					c.restoreDurableEntry(entry, existing)
					return &transferLedgerHandle{entry: entry}, nil
				case transferLedgerProcessing:
					c.abortEntry(entry, req, ErrTransferLedgerRecoveryRequired)
					return nil, ErrTransferLedgerRecoveryRequired
				}
			}
			c.abortEntry(entry, req, err)
			if queryErr != nil {
				return nil, fmt.Errorf("map_: begin durable transfer ledger: %w; requery: %v", err, queryErr)
			}
			return nil, fmt.Errorf("map_: begin durable transfer ledger: %w", err)
		}
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		c.abortEntry(entry, req, ErrTransferLedgerClosed)
		return nil, ErrTransferLedgerClosed
	}
	current := c.entries[key]
	if current != entry {
		if current == nil {
			c.entries[key] = entry
		} else if current.digest != digest {
			c.mu.Unlock()
			c.abortEntry(entry, req, ErrTransferCorrelationConflict)
			return nil, ErrTransferCorrelationConflict
		} else {
			c.mu.Unlock()
			c.abortEntry(entry, req, ErrTransferCorrelationConflict)
			return &transferLedgerHandle{entry: current}, nil
		}
	}
	c.mu.Unlock()
	return &transferLedgerHandle{entry: entry, owner: true}, nil
}

func (c *TransferLedgerComponent) restoreDurableEntry(
	entry *transferLedgerEntry,
	record *transferLedgerRecord,
) {
	if c == nil || entry == nil || record == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry.completed {
		return
	}
	entry.recordID = record.Id
	entry.response = record.Response
	entry.completed = true
	entry.completedAt = record.UpdatedAt
	if entry.createdAt.IsZero() {
		entry.createdAt = record.CreatedAt
	}
	close(entry.done)
}

func (c *TransferLedgerComponent) abortEntry(
	entry *transferLedgerEntry,
	req *M2MUnitTransferRequest,
	err error,
) {
	if c == nil || entry == nil {
		return
	}
	c.mu.Lock()
	if current := c.entries[entry.key]; current == entry {
		delete(c.entries, entry.key)
	}
	if !entry.completed {
		entry.response = transferErrorResponse(req, err)
		entry.completed = true
		entry.completedAt = time.Now()
		close(entry.done)
	}
	c.mu.Unlock()
}

func (c *TransferLedgerComponent) complete(
	ctx context.Context,
	scene *ecs.Scene,
	handle *transferLedgerHandle,
	response M2MUnitTransferResponse,
) error {
	if c == nil || handle == nil || handle.entry == nil || !handle.owner {
		return nil
	}
	entry := handle.entry
	c.mu.Lock()
	if entry.completed {
		c.mu.Unlock()
		return nil
	}
	durable := c.RequireDurable || c.Store != nil
	c.mu.Unlock()
	if !durable {
		c.publishEntry(entry, response, response.Error != 0)
		return nil
	}

	store, collection, attempts, backoff, err := c.storeForScene(scene)
	if err != nil {
		return c.failDurableCompletion(entry, err)
	}
	if entry.recordID <= 0 {
		return c.failDurableCompletion(entry, fmt.Errorf("%w: record id is zero", ErrTransferLedgerPersistence))
	}
	state := transferLedgerCompleted
	if response.Error != 0 {
		state = transferLedgerFailed
	}
	completedAt := time.Now()
	record := &transferLedgerRecord{
		Id:         entry.recordID,
		RpcID:      entry.key.rpcID,
		OldActorID: entry.key.oldActorID,
		Digest:     append([]byte(nil), entry.digest[:]...),
		State:      state,
		Response:   response,
		CreatedAt:  entry.createdAt,
		UpdatedAt:  completedAt,
	}
	if err := saveWithRetry(ctx, func(callCtx context.Context) error {
		return store.Save(callCtx, record.Id, record, collection)
	}, attempts, backoff); err != nil {
		return c.failDurableCompletion(
			entry,
			fmt.Errorf("%w: %v", ErrTransferLedgerPersistence, err),
		)
	}
	c.mu.Lock()
	if !entry.completed {
		entry.response = response
		entry.completed = true
		entry.completedAt = completedAt
		close(entry.done)
	}
	c.mu.Unlock()
	return nil
}

// QueryProcessing 返回 durable ledger 中仍处于 processing 的记录。
//
// 结果包含目标 Actor、地图名、UnitID 和 digest，供外部协调器证明目标
// Unit/Location 的最终状态；查询本身不执行任何业务副作用。
func (c *TransferLedgerComponent) QueryProcessing(
	ctx context.Context,
	scene *ecs.Scene,
) ([]TransferLedgerProcessingRecord, error) {
	if c == nil || scene == nil || ctx == nil {
		return nil, ErrTransferRequestInvalid
	}
	store, collection, _, _, err := c.storeForScene(scene)
	if err != nil {
		return nil, err
	}
	var records []transferLedgerRecord
	if err := store.Query(ctx, bson.M{"state": transferLedgerProcessing}, collection, &records); err != nil {
		return nil, err
	}
	result := make([]TransferLedgerProcessingRecord, 0, len(records))
	for _, record := range records {
		if record.State != transferLedgerProcessing {
			continue
		}
		result = append(result, record.processingRecord())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// RecoverProcessing 只收敛目标 ledger 的 processing 终态。
//
// Unknown/Pending/NotFound 会原样保留；Committed/Failed 必须返回与 RpcID
// 一致且语义匹配的响应，随后先持久化终态，再唤醒当前进程的等待者。该方法
// 不执行 Unit、AOI 或 Location 操作；source journal 的清理由另一侧的
// TransferRecoveryCoordinator 负责。
func (c *TransferLedgerComponent) RecoverProcessing(
	ctx context.Context,
	scene *ecs.Scene,
	coordinator TransferLedgerRecoveryCoordinator,
) ([]TransferLedgerProcessingRecord, error) {
	if c == nil || scene == nil || ctx == nil {
		return nil, ErrTransferRequestInvalid
	}
	if coordinator == nil {
		return nil, ErrTransferLedgerRecoveryCoordinatorMissing
	}
	records, err := c.QueryProcessing(ctx, scene)
	if err != nil {
		return nil, err
	}
	unresolved := make([]TransferLedgerProcessingRecord, 0, len(records))
	var recoveryErr error
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			if recoveryErr == nil {
				recoveryErr = err
			}
			unresolved = append(unresolved, record)
			continue
		}
		if record.ID <= 0 || record.RpcID == 0 || !record.OldActorID.IsValid() ||
			len(record.Digest) != sha256.Size {
			if recoveryErr == nil {
				recoveryErr = ErrTransferLedgerRecoveryStateInvalid
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
		case TransferLedgerRecoveryUnknown, TransferLedgerRecoveryPending, TransferLedgerRecoveryNotFound:
			unresolved = append(unresolved, record)
		case TransferLedgerRecoveryCommitted, TransferLedgerRecoveryFailed:
			if result.Response.RpcID != record.RpcID ||
				(result.State == TransferLedgerRecoveryCommitted && result.Response.Error != 0) ||
				(result.State == TransferLedgerRecoveryFailed && result.Response.Error == 0) {
				if recoveryErr == nil {
					recoveryErr = ErrTransferLedgerRecoveryStateInvalid
				}
				unresolved = append(unresolved, record)
				continue
			}
			state := transferLedgerCompleted
			if result.State == TransferLedgerRecoveryFailed {
				state = transferLedgerFailed
			}
			if err := c.persistRecoveredTerminal(ctx, scene, record, state, result.Response); err != nil {
				if recoveryErr == nil {
					recoveryErr = err
				}
				unresolved = append(unresolved, record)
			}
		default:
			if recoveryErr == nil {
				recoveryErr = fmt.Errorf("%w: %q", ErrTransferLedgerRecoveryStateInvalid, result.State)
			}
			unresolved = append(unresolved, record)
		}
	}
	return unresolved, recoveryErr
}

func (c *TransferLedgerComponent) persistRecoveredTerminal(
	ctx context.Context,
	scene *ecs.Scene,
	processing TransferLedgerProcessingRecord,
	state string,
	response M2MUnitTransferResponse,
) error {
	store, collection, attempts, backoff, err := c.storeForScene(scene)
	if err != nil {
		return err
	}
	now := time.Now()
	record := &transferLedgerRecord{
		Id:          processing.ID,
		RpcID:       processing.RpcID,
		OldActorID:  processing.OldActorID,
		TargetActor: processing.TargetActor,
		TargetMap:   processing.TargetMap,
		UnitID:      processing.UnitID,
		Digest:      append([]byte(nil), processing.Digest...),
		State:       state,
		Response:    response,
		CreatedAt:   processing.CreatedAt,
		UpdatedAt:   now,
	}
	if err := saveWithRetry(ctx, func(callCtx context.Context) error {
		return store.Save(callCtx, record.Id, record, collection)
	}, attempts, backoff); err != nil {
		return fmt.Errorf("%w: recover ledger record %d: %v", ErrTransferLedgerPersistence, record.Id, err)
	}
	key := transferLedgerKey{oldActorID: processing.OldActorID, rpcID: processing.RpcID}
	c.mu.Lock()
	entry := c.entries[key]
	if entry != nil && !entry.completed {
		entry.recordID = processing.ID
		entry.response = response
		entry.completed = true
		entry.completedAt = now
		close(entry.done)
	}
	c.mu.Unlock()
	return nil
}

// publishEntry 将已经确定的结果发布给当前进程内的等待者。
//
// durable ledger 必须先完成终态写入，再调用本函数；否则并发请求可能
// 提前获得成功响应，而进程崩溃后 durable record 仍停留在 processing。
func (c *TransferLedgerComponent) publishEntry(
	entry *transferLedgerEntry,
	response M2MUnitTransferResponse,
	removeOnError bool,
) {
	if c == nil || entry == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry.completed {
		return
	}
	entry.response = response
	entry.completed = true
	entry.completedAt = time.Now()
	if removeOnError && c.entries[entry.key] == entry {
		delete(c.entries, entry.key)
	}
	close(entry.done)
}

func (c *TransferLedgerComponent) failDurableCompletion(
	entry *transferLedgerEntry,
	cause error,
) error {
	if entry == nil {
		return fmt.Errorf("%w: entry is nil", ErrTransferLedgerPersistence)
	}
	if cause == nil {
		cause = ErrTransferLedgerPersistence
	}
	persistenceErr := fmt.Errorf("%w: %v", ErrTransferLedgerPersistence, cause)
	recoveryErr := fmt.Errorf("%w: %w", ErrTransferLedgerRecoveryRequired, persistenceErr)
	c.publishEntry(entry, M2MUnitTransferResponse{
		RpcID:   entry.key.rpcID,
		Error:   1,
		Message: recoveryErr.Error(),
	}, false)
	return recoveryErr
}

func (c *TransferLedgerComponent) response(handle *transferLedgerHandle) M2MUnitTransferResponse {
	if handle == nil || handle.entry == nil {
		return transferErrorResponse(nil, ErrTransferLedgerMissing)
	}
	entry := handle.entry
	if !handle.owner {
		<-entry.done
	}
	return entry.response
}

func (c *TransferLedgerComponent) storeForScene(
	scene *ecs.Scene,
) (transferJournalStore, string, int, time.Duration, error) {
	if c == nil || scene == nil {
		return nil, "", 0, 0, ErrTransferLedgerStoreMissing
	}
	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, "", 0, 0, ErrTransferLedgerClosed
	}
	store := c.Store
	attempts := c.MaxRetries + 1
	backoff := c.RetryBackoff
	c.mu.Unlock()
	if store != nil {
		return store, transferLedgerCollection, attempts, backoff, nil
	}
	component, ok := scene.GetComponent("DBManagerComponent")
	if !ok || component == nil {
		return nil, "", 0, 0, ErrTransferLedgerStoreMissing
	}
	manager, ok := component.(*db.DBManagerComponent)
	if !ok || manager == nil {
		return nil, "", 0, 0, ErrTransferLedgerStoreMissing
	}
	zoneDB, err := manager.GetZoneDB(scene.Zone())
	if err != nil {
		return nil, "", 0, 0, err
	}
	return zoneDB, transferLedgerCollection, attempts, backoff, nil
}

func queryTransferLedgerRecord(
	ctx context.Context,
	store transferJournalStore,
	collection string,
	id int64,
) (*transferLedgerRecord, bool, error) {
	var records []transferLedgerRecord
	if err := store.Query(ctx, bson.M{"_id": id}, collection, &records); err != nil {
		return nil, false, err
	}
	if len(records) == 0 {
		return nil, false, nil
	}
	return &records[0], true, nil
}

func transferLedgerRecordID(key transferLedgerKey, digest [sha256.Size]byte) (int64, error) {
	sum := sha256.Sum256(append(
		append(actorIDBytes(key.oldActorID), byte(key.rpcID>>24), byte(key.rpcID>>16), byte(key.rpcID>>8), byte(key.rpcID)),
		digest[:]...,
	))
	id := int64(binary.BigEndian.Uint64(sum[:8]) & uint64(^uint64(0)>>1))
	if id <= 0 {
		return 0, fmt.Errorf("map_: generated transfer ledger id is zero")
	}
	return id, nil
}

func actorIDBytes(id actor.ActorID) []byte {
	result := make([]byte, 20)
	binary.BigEndian.PutUint32(result[0:4], uint32(id.ProcessID))
	binary.BigEndian.PutUint64(result[4:12], uint64(id.FiberID))
	binary.BigEndian.PutUint64(result[12:20], uint64(id.InstanceID))
	return result
}

func (c *TransferLedgerComponent) evictExpiredLocked(now time.Time) {
	if c.TTL <= 0 {
		return
	}
	for key, entry := range c.entries {
		if entry == nil || !entry.completed {
			continue
		}
		if now.Sub(entry.completedAt) >= c.TTL {
			delete(c.entries, key)
		}
	}
}

func (c *TransferLedgerComponent) evictOverflowLocked() {
	if c.MaxEntries <= 0 || len(c.entries) <= c.MaxEntries {
		return
	}
	for len(c.entries) > c.MaxEntries {
		var oldestKey transferLedgerKey
		var oldest time.Time
		found := false
		for key, entry := range c.entries {
			if entry == nil || !entry.completed {
				continue
			}
			if !found || entry.completedAt.Before(oldest) {
				oldestKey = key
				oldest = entry.completedAt
				found = true
			}
		}
		if !found {
			return
		}
		delete(c.entries, oldestKey)
	}
}

func transferRequestDigest(req *M2MUnitTransferRequest) ([sha256.Size]byte, error) {
	if req == nil {
		return [sha256.Size]byte{}, ErrTransferRequestInvalid
	}
	data, err := json.Marshal(struct {
		RpcID      uint32        `json:"rpc_id"`
		OldActorID actor.ActorID `json:"old_actor_id"`
		Unit       []byte        `json:"unit"`
		Entitys    [][]byte      `json:"entitys"`
	}{
		RpcID:      req.RpcID,
		OldActorID: req.OldActorID,
		Unit:       req.Unit,
		Entitys:    req.Entitys,
	})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}
