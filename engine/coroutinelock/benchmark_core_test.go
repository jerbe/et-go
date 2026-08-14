package coroutinelock

import (
	"context"
	"sync/atomic"
	"testing"
)

func BenchmarkCoreAcquireReleaseNoContention(b *testing.B) {
	lock := New()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		release, err := lock.Acquire(ctx, LockTypeLogin, int64(index))
		if err != nil {
			b.Fatalf("acquire error: %v", err)
		}
		release()
	}
}

func BenchmarkCoreAcquireReleaseHighContention(b *testing.B) {
	lock := New()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			release, err := lock.Acquire(ctx, LockTypeDB, 1)
			if err != nil {
				b.Fatalf("acquire error: %v", err)
			}
			release()
		}
	})
}

func BenchmarkCoreAcquireReleaseManyKeys(b *testing.B) {
	lock := New()
	ctx := context.Background()
	var counter atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := counter.Add(1) % 1024
			release, err := lock.Acquire(ctx, LockTypeLocation, key)
			if err != nil {
				b.Fatalf("acquire error: %v", err)
			}
			release()
		}
	})
}
