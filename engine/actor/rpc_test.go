package actor

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestRpcManagerRegisterResolve(t *testing.T) {
	manager := NewRpcManager()

	rpcID1, channel1 := manager.Register()
	rpcID2, channel2 := manager.Register()
	if rpcID2 <= rpcID1 {
		t.Fatalf("rpcID should increase: first=%d second=%d", rpcID1, rpcID2)
	}

	payload := []byte("rpc-response")
	manager.Resolve(rpcID1, RpcResponse{Payload: payload})

	select {
	case response := <-channel1:
		if response.Err != nil {
			t.Fatalf("response err = %v", response.Err)
		}
		if !bytes.Equal(response.Payload, payload) {
			t.Fatalf("payload = %q, want %q", response.Payload, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("wait response timeout")
	}

	manager.Resolve(rpcID2, RpcResponse{Payload: []byte("ok2")})
	select {
	case <-channel2:
	case <-time.After(time.Second):
		t.Fatal("wait second response timeout")
	}
}

func TestRpcManagerRemove(t *testing.T) {
	manager := NewRpcManager()
	rpcID, ch := manager.Register()

	manager.Remove(rpcID)
	manager.Resolve(rpcID, RpcResponse{Payload: []byte("should-ignore")})

	select {
	case <-ch:
		t.Fatal("removed callback should not receive response")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRpcManagerConcurrentRegisterResolve(t *testing.T) {
	manager := NewRpcManager()
	const workers = 64

	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)

	for index := 0; index < workers; index++ {
		go func(index int) {
			defer waitGroup.Done()
			rpcID, ch := manager.Register()
			manager.Resolve(rpcID, RpcResponse{Payload: []byte{byte(index)}})

			select {
			case response := <-ch:
				if len(response.Payload) != 1 {
					t.Errorf("response payload len = %d, want 1", len(response.Payload))
				}
			case <-time.After(time.Second):
				t.Errorf("wait response timeout")
			}
		}(index)
	}

	waitGroup.Wait()
}

func TestRpcManagerTimeout(t *testing.T) {
	manager := NewRpcManager(WithRPCTimeout(20 * time.Millisecond))

	_, ch := manager.Register()
	select {
	case response := <-ch:
		if response.Err != ErrTimeout {
			t.Fatalf("response err = %v, want %v", response.Err, ErrTimeout)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("wait timeout response timeout")
	}
}
