package actorlocation

import (
	"context"
	"time"
)

const defaultAcquireTimeout = 10 * time.Second

func timeLimitedContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), timeout)
}
