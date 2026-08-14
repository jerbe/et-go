package map_

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
)

type fakeTransferJournalStore struct {
	mu      sync.Mutex
	records map[int64]TransferTransactionRecord
	saveErr error
}

type fakeTransferRecoveryCoordinator struct {
	result     TransferRecoveryResult
	resolveErr error
	cleanupErr error
	cleaned    []string
}

func (c *fakeTransferRecoveryCoordinator) Resolve(
	_ context.Context,
	_ TransferTransactionRecord,
) (TransferRecoveryResult, error) {
	if c.resolveErr != nil {
		return TransferRecoveryResult{}, c.resolveErr
	}
	return c.result, nil
}

func (c *fakeTransferRecoveryCoordinator) CleanupSource(
	_ context.Context,
	record TransferTransactionRecord,
	token string,
) error {
	if c.cleanupErr != nil {
		return c.cleanupErr
	}
	c.cleaned = append(c.cleaned, fmt.Sprintf("%d:%s", record.Id, token))
	return nil
}

type journalTestFiber struct{}

func (journalTestFiber) ID() int64      { return 1 }
func (journalTestFiber) ProcessID() int { return 1 }

func newJournalTestScene() *ecs.Scene {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map1")
	scene.SetFiber(journalTestFiber{})
	return scene
}

func newFakeTransferJournalStore() *fakeTransferJournalStore {
	return &fakeTransferJournalStore{records: make(map[int64]TransferTransactionRecord)}
}

func (s *fakeTransferJournalStore) Insert(_ context.Context, entity any, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := entity.(*TransferTransactionRecord)
	if !ok || record == nil || record.Id <= 0 {
		return errors.New("fake transfer journal: invalid record")
	}
	if _, exists := s.records[record.Id]; exists {
		return errors.New("fake transfer journal: duplicate record")
	}
	s.records[record.Id] = *record
	return nil
}

func (s *fakeTransferJournalStore) Query(_ context.Context, _ bson.M, _ string, results any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := results.(*[]TransferTransactionRecord)
	if !ok {
		return errors.New("fake transfer journal: invalid query result")
	}
	*target = (*target)[:0]
	for _, record := range s.records {
		if record.State == TransferTransactionPending || record.State == TransferTransactionCommitted {
			*target = append(*target, record)
		}
	}
	return nil
}

func (s *fakeTransferJournalStore) Save(_ context.Context, id int64, entity any, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	record, ok := entity.(*TransferTransactionRecord)
	if !ok || record == nil || record.Id != id {
		return errors.New("fake transfer journal: invalid save record")
	}
	s.records[id] = *record
	return nil
}

func (s *fakeTransferJournalStore) Remove(_ context.Context, id int64, _ string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[id]; !ok {
		return 0, nil
	}
	delete(s.records, id)
	return 1, nil
}

func TestTransferJournalPersistsStateTransitions(t *testing.T) {
	store := newFakeTransferJournalStore()
	journal := &TransferJournalComponent{
		Store:      store,
		MaxRetries: 1,
	}
	scene := newJournalTestScene()
	request := &M2MUnitTransferRequest{
		RpcID:      7,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 101),
		Entitys:    [][]byte{[]byte("component")},
	}
	target := actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5}

	record, err := journal.Begin(context.Background(), scene, request, target, "Map2")
	if err != nil {
		t.Fatalf("Begin error = %v", err)
	}
	if record.State != TransferTransactionPending || record.UnitID != 101 {
		t.Fatalf("begin record = %+v", record)
	}

	if err := journal.MarkState(context.Background(), scene, record, TransferTransactionCommitted, nil); err != nil {
		t.Fatalf("MarkState committed error = %v", err)
	}
	recoverable, err := journal.QueryRecoverable(context.Background(), scene)
	if err != nil {
		t.Fatalf("QueryRecoverable error = %v", err)
	}
	if len(recoverable) != 1 || recoverable[0].State != TransferTransactionCommitted {
		t.Fatalf("recoverable records = %+v", recoverable)
	}

	if err := journal.MarkState(context.Background(), scene, record, TransferTransactionSourceDisposed, nil); err != nil {
		t.Fatalf("MarkState source disposed error = %v", err)
	}
	recoverable, err = journal.QueryRecoverable(context.Background(), scene)
	if err != nil {
		t.Fatalf("QueryRecoverable after source dispose error = %v", err)
	}
	if len(recoverable) != 0 {
		t.Fatalf("recoverable records after source dispose = %+v, want empty", recoverable)
	}
}

func TestTransferJournalRecoverCommittedRequiresTokenAndCleansSource(t *testing.T) {
	store := newFakeTransferJournalStore()
	journal := &TransferJournalComponent{Store: store, MaxRetries: 1}
	scene := newJournalTestScene()
	record, err := journal.Begin(context.Background(), scene, &M2MUnitTransferRequest{
		RpcID:      21,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 201),
	}, actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5}, "Map2")
	if err != nil {
		t.Fatalf("Begin error = %v", err)
	}
	if err := journal.MarkState(context.Background(), scene, record, TransferTransactionCommitted, nil); err != nil {
		t.Fatalf("MarkState committed error = %v", err)
	}
	coordinator := &fakeTransferRecoveryCoordinator{
		result: TransferRecoveryResult{
			State:         TransferRecoveryCommitted,
			RecoveryToken: "recover-21",
		},
	}
	unresolved, err := journal.Recover(context.Background(), scene, coordinator)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved records = %+v, want empty", unresolved)
	}
	if len(coordinator.cleaned) != 1 || coordinator.cleaned[0] != fmt.Sprintf("%d:recover-21", record.Id) {
		t.Fatalf("cleanup calls = %v", coordinator.cleaned)
	}
	if got := store.records[record.Id].State; got != TransferTransactionSourceDisposed {
		t.Fatalf("durable state = %q, want %q", got, TransferTransactionSourceDisposed)
	}
}

func TestTransferJournalRecoverPreservesUncertainState(t *testing.T) {
	store := newFakeTransferJournalStore()
	journal := &TransferJournalComponent{Store: store, MaxRetries: 1}
	scene := newJournalTestScene()
	record, err := journal.Begin(context.Background(), scene, &M2MUnitTransferRequest{
		RpcID:      22,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 202),
	}, actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5}, "Map2")
	if err != nil {
		t.Fatalf("Begin error = %v", err)
	}
	if err := journal.MarkState(context.Background(), scene, record, TransferTransactionCommitted, nil); err != nil {
		t.Fatalf("MarkState committed error = %v", err)
	}
	coordinator := &fakeTransferRecoveryCoordinator{
		result: TransferRecoveryResult{State: TransferRecoveryCommitted},
	}
	unresolved, err := journal.Recover(context.Background(), scene, coordinator)
	if !errors.Is(err, ErrTransferRecoveryTokenMissing) {
		t.Fatalf("Recover error = %v, want %v", err, ErrTransferRecoveryTokenMissing)
	}
	if len(unresolved) != 1 || unresolved[0].Id != record.Id {
		t.Fatalf("unresolved records = %+v, want record %d", unresolved, record.Id)
	}
	if len(coordinator.cleaned) != 0 {
		t.Fatalf("cleanup must not run without token: %v", coordinator.cleaned)
	}
	if got := store.records[record.Id].State; got != TransferTransactionCommitted {
		t.Fatalf("durable state = %q, want %q", got, TransferTransactionCommitted)
	}
}

func TestTransferJournalRecoverFailedTargetMarksFailed(t *testing.T) {
	store := newFakeTransferJournalStore()
	journal := &TransferJournalComponent{Store: store, MaxRetries: 1}
	scene := newJournalTestScene()
	record, err := journal.Begin(context.Background(), scene, &M2MUnitTransferRequest{
		RpcID:      23,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 203),
	}, actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5}, "Map2")
	if err != nil {
		t.Fatalf("Begin error = %v", err)
	}
	coordinator := &fakeTransferRecoveryCoordinator{
		result: TransferRecoveryResult{
			State: TransferRecoveryFailed,
			Cause: errors.New("target rejected"),
		},
	}
	unresolved, err := journal.Recover(context.Background(), scene, coordinator)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved records = %+v, want empty", unresolved)
	}
	if got := store.records[record.Id].State; got != TransferTransactionFailed {
		t.Fatalf("durable state = %q, want %q", got, TransferTransactionFailed)
	}
	if got := store.records[record.Id].LastError; got != "target rejected" {
		t.Fatalf("durable error = %q", got)
	}
}

func TestTransferJournalRecoverRejectsInvalidPersistedRecord(t *testing.T) {
	store := newFakeTransferJournalStore()
	store.records[99] = TransferTransactionRecord{
		Id:    99,
		RpcID: 24,
		State: TransferTransactionCommitted,
		Digest: []byte{
			1, 2, 3,
		},
	}
	journal := &TransferJournalComponent{Store: store, MaxRetries: 1}
	scene := newJournalTestScene()
	coordinator := &fakeTransferRecoveryCoordinator{
		result: TransferRecoveryResult{
			State:         TransferRecoveryCommitted,
			RecoveryToken: "should-not-run",
		},
	}
	unresolved, err := journal.Recover(context.Background(), scene, coordinator)
	if !errors.Is(err, ErrTransferRecoveryStateInvalid) {
		t.Fatalf("Recover error = %v, want %v", err, ErrTransferRecoveryStateInvalid)
	}
	if len(unresolved) != 1 || len(coordinator.cleaned) != 0 {
		t.Fatalf("unresolved=%+v cleanup=%v", unresolved, coordinator.cleaned)
	}
}

func TestTransferJournalRejectsIllegalStateRegression(t *testing.T) {
	store := newFakeTransferJournalStore()
	journal := &TransferJournalComponent{Store: store, MaxRetries: 1}
	scene := newJournalTestScene()
	record, err := journal.Begin(context.Background(), scene, &M2MUnitTransferRequest{
		RpcID:      8,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 102),
	}, actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5}, "Map2")
	if err != nil {
		t.Fatalf("Begin error = %v", err)
	}
	if err := journal.MarkState(context.Background(), scene, record, TransferTransactionCommitted, nil); err != nil {
		t.Fatalf("MarkState committed error = %v", err)
	}
	err = journal.MarkState(context.Background(), scene, record, TransferTransactionPending, errors.New("late retry"))
	if !errors.Is(err, ErrTransferJournalStateInvalid) {
		t.Fatalf("illegal regression error = %v, want %v", err, ErrTransferJournalStateInvalid)
	}
	if record.State != TransferTransactionCommitted {
		t.Fatalf("record state after rejected regression = %q, want committed", record.State)
	}
}

func TestTransferJournalDoesNotMutateMemoryWhenSaveFails(t *testing.T) {
	store := newFakeTransferJournalStore()
	journal := &TransferJournalComponent{Store: store, MaxRetries: 1}
	scene := newJournalTestScene()
	record, err := journal.Begin(context.Background(), scene, &M2MUnitTransferRequest{
		RpcID:      9,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 103),
	}, actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5}, "Map2")
	if err != nil {
		t.Fatalf("Begin error = %v", err)
	}
	store.saveErr = errors.New("journal write failed")
	err = journal.MarkState(context.Background(), scene, record, TransferTransactionCommitted, nil)
	if !errors.Is(err, store.saveErr) {
		t.Fatalf("MarkState error = %v, want %v", err, store.saveErr)
	}
	if record.State != TransferTransactionPending {
		t.Fatalf("record state after failed save = %q, want pending", record.State)
	}
}

func TestTransferJournalRejectsInvalidUnitSnapshot(t *testing.T) {
	store := newFakeTransferJournalStore()
	journal := &TransferJournalComponent{Store: store, MaxRetries: 1}
	scene := newJournalTestScene()
	_, err := journal.Begin(context.Background(), scene, &M2MUnitTransferRequest{
		RpcID:      10,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       []byte("invalid"),
	}, actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5}, "Map2")
	if !errors.Is(err, ErrTransferRequestInvalid) {
		t.Fatalf("invalid snapshot error = %v, want %v", err, ErrTransferRequestInvalid)
	}
}

func mustMarshalUnitSnapshot(t *testing.T, id int64) []byte {
	t.Helper()
	data, err := bson.Marshal(unitSnapshot{ID: id})
	if err != nil {
		t.Fatalf("marshal unit snapshot error = %v", err)
	}
	return data
}
