package map_

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
)

type fakeTransferLedgerStore struct {
	mu                   sync.Mutex
	records              map[int64]transferLedgerRecord
	saveErr              error
	terminalSaveStarted  chan struct{}
	terminalSaveRelease  <-chan struct{}
	terminalSaveSignaled bool
}

type fakeTransferLedgerRecoveryCoordinator struct {
	result     TransferLedgerRecoveryResult
	resolveErr error
	records    []TransferLedgerProcessingRecord
}

func (c *fakeTransferLedgerRecoveryCoordinator) Resolve(
	_ context.Context,
	record TransferLedgerProcessingRecord,
) (TransferLedgerRecoveryResult, error) {
	c.records = append(c.records, record)
	if c.resolveErr != nil {
		return TransferLedgerRecoveryResult{}, c.resolveErr
	}
	return c.result, nil
}

func newFakeTransferLedgerStore() *fakeTransferLedgerStore {
	return &fakeTransferLedgerStore{records: make(map[int64]transferLedgerRecord)}
}

func (s *fakeTransferLedgerStore) Insert(_ context.Context, entity any, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := entity.(*transferLedgerRecord)
	if !ok || record == nil || record.Id <= 0 {
		return errors.New("fake transfer ledger: invalid record")
	}
	if _, exists := s.records[record.Id]; exists {
		return errors.New("fake transfer ledger: duplicate record")
	}
	s.records[record.Id] = *record
	return nil
}

func (s *fakeTransferLedgerStore) Query(_ context.Context, filter bson.M, _ string, results any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := results.(*[]transferLedgerRecord)
	if !ok {
		return errors.New("fake transfer ledger: invalid query result")
	}
	*target = (*target)[:0]
	id, hasID := filter["_id"].(int64)
	for key, record := range s.records {
		if hasID && key != id {
			continue
		}
		*target = append(*target, record)
	}
	return nil
}

func (s *fakeTransferLedgerStore) Save(_ context.Context, id int64, entity any, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := entity.(*transferLedgerRecord)
	if !ok || record == nil || record.Id != id {
		return errors.New("fake transfer ledger: invalid save record")
	}
	if record.State != transferLedgerProcessing && s.terminalSaveRelease != nil {
		if !s.terminalSaveSignaled {
			s.terminalSaveSignaled = true
			close(s.terminalSaveStarted)
		}
		<-s.terminalSaveRelease
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	s.records[id] = *record
	return nil
}

func (s *fakeTransferLedgerStore) Remove(_ context.Context, id int64, _ string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[id]; !ok {
		return 0, nil
	}
	delete(s.records, id)
	return 1, nil
}

func TestTransferLedgerPersistsCompletedResponse(t *testing.T) {
	store := newFakeTransferLedgerStore()
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      9,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 202),
	}

	first, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("first begin error = %v", err)
	}
	if !first.owner {
		t.Fatal("first begin should own processing")
	}
	response := M2MUnitTransferResponse{RpcID: request.RpcID}
	if err := ledger.complete(context.Background(), scene, first, response); err != nil {
		t.Fatalf("complete error = %v", err)
	}

	second, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("second begin error = %v", err)
	}
	if second.owner {
		t.Fatal("second begin should reuse durable response")
	}
	if got := ledger.response(second); got != response {
		t.Fatalf("reused response = %+v, want %+v", got, response)
	}
}

func TestTransferLedgerSharesInFlightRequest(t *testing.T) {
	store := newFakeTransferLedgerStore()
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      11,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 404),
	}

	first, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("first begin error = %v", err)
	}
	if !first.owner {
		t.Fatal("first begin should own processing")
	}
	second, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("second begin error = %v", err)
	}
	if second.owner {
		t.Fatal("second begin should wait for the in-flight owner")
	}

	responseCh := make(chan M2MUnitTransferResponse, 1)
	go func() {
		responseCh <- ledger.response(second)
	}()
	select {
	case response := <-responseCh:
		t.Fatalf("in-flight response returned before completion: %+v", response)
	case <-time.After(10 * time.Millisecond):
	}

	response := M2MUnitTransferResponse{RpcID: request.RpcID}
	if err := ledger.complete(context.Background(), scene, first, response); err != nil {
		t.Fatalf("complete error = %v", err)
	}
	select {
	case got := <-responseCh:
		if got != response {
			t.Fatalf("in-flight response = %+v, want %+v", got, response)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight response did not finish")
	}
}

func TestTransferLedgerRejectsDurableProcessingWithoutRecovery(t *testing.T) {
	store := newFakeTransferLedgerStore()
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      10,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 303),
	}
	digest, err := transferRequestDigest(request)
	if err != nil {
		t.Fatalf("transferRequestDigest error = %v", err)
	}
	key := transferLedgerKey{oldActorID: request.OldActorID, rpcID: request.RpcID}
	recordID, err := transferLedgerRecordID(key, digest)
	if err != nil {
		t.Fatalf("transferLedgerRecordID error = %v", err)
	}
	store.records[recordID] = transferLedgerRecord{
		Id:         recordID,
		RpcID:      request.RpcID,
		OldActorID: request.OldActorID,
		Digest:     append([]byte(nil), digest[:]...),
		State:      transferLedgerProcessing,
	}

	if _, err := ledger.begin(scene, request); !errors.Is(err, ErrTransferLedgerRecoveryRequired) {
		t.Fatalf("begin error = %v, want %v", err, ErrTransferLedgerRecoveryRequired)
	}
}

func TestTransferLedgerRecoverProcessingCommitsDurableRecord(t *testing.T) {
	store := newFakeTransferLedgerStore()
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      14,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 707),
	}
	first, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("begin error = %v", err)
	}
	if !first.owner {
		t.Fatal("first begin should own processing")
	}
	coordinator := &fakeTransferLedgerRecoveryCoordinator{
		result: TransferLedgerRecoveryResult{
			State: TransferLedgerRecoveryCommitted,
			Response: M2MUnitTransferResponse{
				RpcID: request.RpcID,
			},
		},
	}
	unresolved, err := ledger.RecoverProcessing(context.Background(), scene, coordinator)
	if err != nil {
		t.Fatalf("RecoverProcessing error = %v", err)
	}
	if len(unresolved) != 0 || len(coordinator.records) != 1 {
		t.Fatalf("unresolved=%+v coordinator records=%+v", unresolved, coordinator.records)
	}
	if coordinator.records[0].UnitID != 707 || coordinator.records[0].TargetMap != "Map2" {
		t.Fatalf("processing record = %+v", coordinator.records[0])
	}
	recordID, err := transferLedgerRecordID(
		transferLedgerKey{oldActorID: request.OldActorID, rpcID: request.RpcID},
		mustDigest(t, request),
	)
	if err != nil {
		t.Fatalf("record id error = %v", err)
	}
	if got := store.records[recordID].State; got != transferLedgerCompleted {
		t.Fatalf("durable state = %q, want %q", got, transferLedgerCompleted)
	}
	second, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("begin after recovery error = %v", err)
	}
	if second.owner || ledger.response(second).Error != 0 {
		t.Fatalf("recovered response handle = owner=%v response=%+v", second.owner, ledger.response(second))
	}
}

func TestTransferLedgerRecoverProcessingPreservesUnknownState(t *testing.T) {
	store := newFakeTransferLedgerStore()
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      15,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 708),
	}
	if _, err := ledger.begin(scene, request); err != nil {
		t.Fatalf("begin error = %v", err)
	}
	unresolved, err := ledger.RecoverProcessing(context.Background(), scene,
		&fakeTransferLedgerRecoveryCoordinator{
			result: TransferLedgerRecoveryResult{State: TransferLedgerRecoveryUnknown},
		})
	if err != nil {
		t.Fatalf("RecoverProcessing error = %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved records = %+v, want one", unresolved)
	}
	recordID, err := transferLedgerRecordID(
		transferLedgerKey{oldActorID: request.OldActorID, rpcID: request.RpcID},
		mustDigest(t, request),
	)
	if err != nil {
		t.Fatalf("record id error = %v", err)
	}
	if got := store.records[recordID].State; got != transferLedgerProcessing {
		t.Fatalf("durable state = %q, want %q", got, transferLedgerProcessing)
	}
}

func TestTransferLedgerRecoverProcessingRejectsMismatchedResponse(t *testing.T) {
	store := newFakeTransferLedgerStore()
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      16,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 709),
	}
	if _, err := ledger.begin(scene, request); err != nil {
		t.Fatalf("begin error = %v", err)
	}
	unresolved, err := ledger.RecoverProcessing(context.Background(), scene,
		&fakeTransferLedgerRecoveryCoordinator{
			result: TransferLedgerRecoveryResult{
				State: TransferLedgerRecoveryCommitted,
				Response: M2MUnitTransferResponse{
					RpcID: request.RpcID + 1,
				},
			},
		})
	if !errors.Is(err, ErrTransferLedgerRecoveryStateInvalid) {
		t.Fatalf("RecoverProcessing error = %v, want %v", err, ErrTransferLedgerRecoveryStateInvalid)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved records = %+v, want one", unresolved)
	}
}

func TestTransferLedgerPublishesOnlyAfterDurableCommit(t *testing.T) {
	store := newFakeTransferLedgerStore()
	store.terminalSaveStarted = make(chan struct{})
	release := make(chan struct{})
	store.terminalSaveRelease = release
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      12,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 505),
	}
	first, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("first begin error = %v", err)
	}

	completeErr := make(chan error, 1)
	go func() {
		completeErr <- ledger.complete(context.Background(), scene, first, M2MUnitTransferResponse{
			RpcID: request.RpcID,
		})
	}()
	select {
	case <-store.terminalSaveStarted:
	case <-time.After(time.Second):
		t.Fatal("durable terminal save did not start")
	}

	second, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("second begin error = %v", err)
	}
	response := make(chan M2MUnitTransferResponse, 1)
	go func() {
		response <- ledger.response(second)
	}()
	select {
	case got := <-response:
		t.Fatalf("response published before durable commit: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-completeErr; err != nil {
		t.Fatalf("complete error = %v", err)
	}
	select {
	case got := <-response:
		if got.Error != 0 || got.RpcID != request.RpcID {
			t.Fatalf("completed response = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("response did not publish after durable commit")
	}
}

func TestTransferLedgerDurableCommitFailureRequiresRecovery(t *testing.T) {
	store := newFakeTransferLedgerStore()
	store.saveErr = errors.New("terminal write failed")
	ledger := &TransferLedgerComponent{
		Store:          store,
		RequireDurable: true,
		MaxRetries:     1,
	}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	request := &M2MUnitTransferRequest{
		RpcID:      13,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 606),
	}
	first, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("first begin error = %v", err)
	}
	err = ledger.complete(context.Background(), scene, first, M2MUnitTransferResponse{
		RpcID: request.RpcID,
	})
	if !errors.Is(err, ErrTransferLedgerRecoveryRequired) {
		t.Fatalf("complete error = %v, want %v", err, ErrTransferLedgerRecoveryRequired)
	}

	second, err := ledger.begin(scene, request)
	if err != nil {
		t.Fatalf("retry begin error = %v", err)
	}
	got := ledger.response(second)
	if got.Error == 0 || got.RpcID != request.RpcID ||
		!strings.Contains(got.Message, ErrTransferLedgerRecoveryRequired.Error()) {
		t.Fatalf("recovery response = %+v", got)
	}
	recordID, err := transferLedgerRecordID(
		transferLedgerKey{oldActorID: request.OldActorID, rpcID: request.RpcID},
		mustDigest(t, request),
	)
	if err != nil {
		t.Fatalf("record id error = %v", err)
	}
	if store.records[recordID].State != transferLedgerProcessing {
		t.Fatalf("durable state = %q, want processing", store.records[recordID].State)
	}
}

func mustDigest(t *testing.T, request *M2MUnitTransferRequest) [sha256.Size]byte {
	t.Helper()
	digest, err := transferRequestDigest(request)
	if err != nil {
		t.Fatalf("transfer digest error = %v", err)
	}
	return digest
}
