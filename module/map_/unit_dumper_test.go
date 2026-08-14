package map_

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestSaveWithRetryRetriesUntilSuccess(t *testing.T) {
	attempts := 0
	err := saveWithRetry(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary")
		}
		return nil
	}, 3, time.Millisecond)
	if err != nil {
		t.Fatalf("saveWithRetry error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestSaveWithRetryReturnsLastError(t *testing.T) {
	expected := errors.New("permanent")
	attempts := 0
	err := saveWithRetry(context.Background(), func(context.Context) error {
		attempts++
		return expected
	}, 3, 0)
	if !errors.Is(err, expected) {
		t.Fatalf("saveWithRetry error = %v, want %v", err, expected)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestSaveWithRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts := 0
	err := saveWithRetry(ctx, func(context.Context) error {
		attempts++
		return errors.New("should not run")
	}, 3, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("saveWithRetry error = %v, want context canceled", err)
	}
	if attempts != 0 {
		t.Fatalf("attempts = %d, want 0", attempts)
	}
}

func TestUnitDumperDeadLettersCopyDataAndBoundSize(t *testing.T) {
	component := &UnitDumperComponent{MaxRetries: 1, MaxDeadLetters: 1}
	first := persistedEntitySnapshot{id: 1, collection: "account", data: []byte("first")}
	second := persistedEntitySnapshot{id: 2, collection: "account", data: []byte("second")}
	component.addDeadLetter(first, errors.New("first failure"))
	component.addDeadLetter(second, errors.New("second failure"))

	letters := component.DeadLetters()
	if len(letters) != 1 || letters[0].ID != 2 {
		t.Fatalf("dead letters = %+v, want only second entry", letters)
	}
	letters[0].Data[0] = 'X'
	if string(component.DeadLetters()[0].Data) != "second" {
		t.Fatal("DeadLetters must return copied snapshot data")
	}
}

type fakeUnitDumpStore struct {
	mu        sync.Mutex
	queue     map[int64]unitDumpTask
	saves     []persistedEntitySnapshot
	events    []string
	insertErr error
	saveErr   error
	removeErr error
	queryErr  error
}

func newFakeUnitDumpStore() *fakeUnitDumpStore {
	return &fakeUnitDumpStore{queue: make(map[int64]unitDumpTask)}
}

func (s *fakeUnitDumpStore) Insert(_ context.Context, entity any, collection string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "insert:"+collection)
	if s.insertErr != nil {
		return s.insertErr
	}
	task, ok := entity.(*unitDumpTask)
	if !ok || task == nil {
		return errors.New("fake: invalid queue task")
	}
	s.queue[task.Id] = *task
	return nil
}

func (s *fakeUnitDumpStore) Query(_ context.Context, _ bson.M, collection string, results any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "query:"+collection)
	if s.queryErr != nil {
		return s.queryErr
	}
	tasks, ok := results.(*[]unitDumpTask)
	if !ok {
		return errors.New("fake: invalid query result")
	}
	*tasks = (*tasks)[:0]
	for _, task := range s.queue {
		*tasks = append(*tasks, task)
	}
	return nil
}

func (s *fakeUnitDumpStore) Save(_ context.Context, _ int64, entity any, collection string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "save:"+collection)
	if s.saveErr != nil {
		return s.saveErr
	}
	switch value := entity.(type) {
	case *unitDumpTask:
		s.queue[value.Id] = *value
	case persistedEntitySnapshot:
		s.saves = append(s.saves, persistedEntitySnapshot{
			id:         value.id,
			collection: value.collection,
			data:       append([]byte(nil), value.data...),
		})
	default:
		return errors.New("fake: invalid save entity")
	}
	return nil
}

func (s *fakeUnitDumpStore) Remove(_ context.Context, id int64, collection string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "remove:"+collection)
	if s.removeErr != nil {
		return 0, s.removeErr
	}
	if _, ok := s.queue[id]; !ok {
		return 0, nil
	}
	delete(s.queue, id)
	return 1, nil
}

func TestUnitDumperPersistsQueueBeforeBusinessDocument(t *testing.T) {
	store := newFakeUnitDumpStore()
	component := &UnitDumperComponent{MaxRetries: 1}
	entity := persistedEntitySnapshot{
		id:         7,
		collection: "hero",
		data:       []byte(`{"_id":7}`),
	}

	if err := component.saveSnapshot(context.Background(), store, entity); err != nil {
		t.Fatalf("saveSnapshot error = %v", err)
	}
	if len(store.queue) != 0 {
		t.Fatalf("durable queue entries = %d, want 0 after success", len(store.queue))
	}
	if len(store.saves) != 1 || store.saves[0].id != entity.id {
		t.Fatalf("business saves = %+v, want one snapshot", store.saves)
	}
	if len(store.events) < 3 || store.events[0] != "insert:"+unitDumpQueueCollection ||
		store.events[1] != "save:"+entity.collection || store.events[2] != "remove:"+unitDumpQueueCollection {
		t.Fatalf("event order = %v, want queue insert -> business save -> queue remove", store.events)
	}
}

func TestUnitDumperDoesNotWriteBusinessDocumentWhenQueueInsertFails(t *testing.T) {
	store := newFakeUnitDumpStore()
	store.insertErr = errors.New("queue unavailable")
	component := &UnitDumperComponent{MaxRetries: 1}

	err := component.saveSnapshot(context.Background(), store, persistedEntitySnapshot{
		id:         7,
		collection: "hero",
		data:       []byte(`{"_id":7}`),
	})
	if err == nil || !errors.Is(err, store.insertErr) {
		t.Fatalf("saveSnapshot error = %v, want queue error", err)
	}
	if len(store.saves) != 0 {
		t.Fatalf("business saves = %+v, want none when queue insert fails", store.saves)
	}
}

func TestUnitDumperRecoversPendingQueue(t *testing.T) {
	store := newFakeUnitDumpStore()
	store.queue[99] = unitDumpTask{
		Id:         99,
		TargetID:   7,
		Collection: "hero",
		Data:       []byte(`{"_id":7}`),
		CreatedAt:  time.Now(),
	}
	component := &UnitDumperComponent{MaxRetries: 1}

	if err := component.recoverPending(context.Background(), store); err != nil {
		t.Fatalf("recoverPending error = %v", err)
	}
	if len(store.queue) != 0 {
		t.Fatalf("queue entries after recovery = %d, want 0", len(store.queue))
	}
	if len(store.saves) != 1 || store.saves[0].id != 7 || store.saves[0].collection != "hero" {
		t.Fatalf("recovered saves = %+v", store.saves)
	}
}
