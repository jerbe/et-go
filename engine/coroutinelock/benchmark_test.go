package coroutinelock

import (
	"context"
	"sync/atomic"
	"testing"
)

func BenchmarkAcquireRelease_NoContention(b *testing.B) {
	lock := New()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		release, err := lock.AcquireByType(ctx, LockTypeLogin, int64(index))
		if err != nil {
			b.Fatalf("acquire error: %v", err)
		}
		release()
	}
}

func BenchmarkAcquireRelease_HighContention(b *testing.B) {
	lock := New()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			release, err := lock.AcquireByType(ctx, LockTypeDB, 1)
			if err != nil {
				b.Fatalf("acquire error: %v", err)
			}
			release()
		}
	})
}

func BenchmarkAcquireRelease_ManyKeys(b *testing.B) {
	lock := New()
	ctx := context.Background()
	var seq atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := seq.Add(1) % 1024
			release, err := lock.AcquireByType(ctx, LockTypeLocation, key)
			if err != nil {
				b.Fatalf("acquire error: %v", err)
			}
			release()
		}
	})
}
