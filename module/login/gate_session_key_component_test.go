package login

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestGateSessionKeyComponentAddGetRemoveAndExpire(t *testing.T) {
	component := NewGateSessionKeyComponent(20 * time.Millisecond)
	component.Awake()
	component.Add("abc", 1)
	if got, ok := component.Get("abc"); !ok || got != 1 {
		t.Fatalf("Get = %d,%v", got, ok)
	}
	component.Remove("abc")
	if _, ok := component.Get("abc"); ok {
		t.Fatal("token should be removed")
	}

	component.Add("expire", 2)
	time.Sleep(50 * time.Millisecond)
	if _, ok := component.Get("expire"); ok {
		t.Fatal("token should expire")
	}
}

func TestGateSessionKeyComponentConcurrent(t *testing.T) {
	component := NewGateSessionKeyComponent(time.Second)
	component.Awake()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			token := fmt.Sprintf("token-%d", index)
			component.Add(token, int64(index))
			component.Get(token)
			component.Remove(token)
		}(i)
	}
	wg.Wait()
}

func TestGateSessionKeyComponentDoesNotReopenAfterDestroy(t *testing.T) {
	component := NewGateSessionKeyComponent(time.Second)
	component.OnDestroy()
	if err := component.Add("closed", 1); err != ErrGateSessionKeyClosed {
		t.Fatalf("Add after destroy error = %v, want %v", err, ErrGateSessionKeyClosed)
	}

	if _, ok := component.Get("closed"); ok {
		t.Fatal("destroyed component should not accept new token")
	}
}

func TestGateSessionKeyComponentRejectsMissingTimer(t *testing.T) {
	component := NewGateSessionKeyComponent(time.Second)
	component.SetAfterFunc(func(time.Duration, func()) *time.Timer { return nil })

	if err := component.Add("no-timer", 1); err != ErrGateSessionKeyTimerMissing {
		t.Fatalf("Add without timer error = %v, want %v", err, ErrGateSessionKeyTimerMissing)
	}
	if _, ok := component.Get("no-timer"); ok {
		t.Fatal("token without timer must not be retained")
	}
}
