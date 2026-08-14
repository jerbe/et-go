package login

import (
	"context"
	"sync"
	"time"
)

type memoryRevocationEntry struct {
	accountID int64
	expiresAt time.Time
}

// MemoryAccessTokenRevocationStore 是进程内撤销存储，适合测试和单进程部署。
//
// 它不提供跨进程或重启持久化能力；生产多进程必须注入共享存储实现。
//
// TODO(security): 真实 MongoDB/多进程部署验收、凭据托管和故障恢复演练仍需
// 外部环境；MongoAccessTokenRevocationStore 已提供明确的共享存储实现，
// 不在此进程内存实现中伪造跨进程语义。
type MemoryAccessTokenRevocationStore struct {
	mu      sync.Mutex
	entries map[string]memoryRevocationEntry
}

func NewMemoryAccessTokenRevocationStore() *MemoryAccessTokenRevocationStore {
	return &MemoryAccessTokenRevocationStore{
		entries: make(map[string]memoryRevocationEntry),
	}
}

func (s *MemoryAccessTokenRevocationStore) IsRevoked(ctx context.Context, token AccessTokenRevocation) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	if s == nil || token.TokenID == "" || token.AccountID <= 0 || token.ExpiresAt.IsZero() {
		return false, ErrTokenRevocationStoreUnavailable
	}
	now := tokenNowFunc()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked(now)
	entry, ok := s.entries[token.TokenID]
	if ok && entry.accountID != token.AccountID {
		return false, ErrTokenRevocationStoreUnavailable
	}
	return ok, nil
}

func (s *MemoryAccessTokenRevocationStore) Revoke(ctx context.Context, token AccessTokenRevocation) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if s == nil || token.TokenID == "" || token.AccountID <= 0 || token.ExpiresAt.IsZero() {
		return ErrTokenRevocationStoreUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]memoryRevocationEntry)
	}
	s.evictLocked(tokenNowFunc())
	if existing, ok := s.entries[token.TokenID]; ok && existing.accountID != token.AccountID {
		return ErrTokenRevocationStoreUnavailable
	}
	s.entries[token.TokenID] = memoryRevocationEntry{
		accountID: token.AccountID,
		expiresAt: token.ExpiresAt,
	}
	return nil
}

func (s *MemoryAccessTokenRevocationStore) evictLocked(now time.Time) {
	for tokenID, entry := range s.entries {
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
			delete(s.entries, tokenID)
		}
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return ErrTokenContextRequired
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
