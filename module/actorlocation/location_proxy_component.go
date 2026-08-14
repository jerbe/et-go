package actorlocation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

// RPCClient 定义位置代理使用的 RPC 能力。
type RPCClient interface {
	Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)
}

// LocationProxyComponent 封装与位置服务的 RPC 通信。
type LocationProxyComponent struct {
	ecs.BaseComponent
	mu            sync.RWMutex
	caller        RPCClient
	locationActor actor.ActorID
	rpcID         atomic.Uint32
}

// Type 返回组件类型名称。
func (p *LocationProxyComponent) Type() string { return "LocationProxyComponent" }

// SetCaller 设置 RPC 调用方。
func (p *LocationProxyComponent) SetCaller(caller RPCClient) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.caller = caller
}

// SetLocationActor 设置位置服务 ActorID。
func (p *LocationProxyComponent) SetLocationActor(actorID actor.ActorID) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.locationActor = actorID
}

// Add 注册位置。
func (p *LocationProxyComponent) Add(locationType int, key int64, actorID actor.ActorID) error {
	payload, err := p.call(context.Background(), MsgObjectAddRequest, &ObjectAddRequest{
		RpcID:   p.nextRPCID(),
		Type:    LocationType(locationType),
		Key:     key,
		ActorID: actorID,
	})
	if err != nil {
		return err
	}
	return parseCommonResponse(MsgObjectAddRequest, payload)
}

// Get 查询位置。
func (p *LocationProxyComponent) Get(locationType int, key int64) (actor.ActorID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	return p.GetContext(ctx, locationType, key)
}

// GetContext 查询位置；位置被显式锁定时等待锁释放并重试。
func (p *LocationProxyComponent) GetContext(ctx context.Context, locationType int, key int64) (actor.ActorID, error) {
	if key <= 0 {
		return actor.ActorID{}, ErrZeroLocationKey
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		payload, err := p.call(ctx, MsgObjectGetRequest, &ObjectGetRequest{
			RpcID: p.nextRPCID(),
			Type:  LocationType(locationType),
			Key:   key,
		})
		if err != nil {
			return actor.ActorID{}, err
		}
		resp, err := unmarshalGetResponse(payload)
		if err != nil {
			return actor.ActorID{}, err
		}
		if resp.Error == 0 {
			return resp.ActorID, nil
		}
		responseErr := &ResponseError{Code: resp.Error, Message: resp.Message}
		if !errors.Is(responseErr, ErrLocationLocked) {
			return actor.ActorID{}, responseErr
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return actor.ActorID{}, ctx.Err()
		}
	}
}

// Lock 加锁。
func (p *LocationProxyComponent) Lock(locationType int, key int64, actorID actor.ActorID, timeMs int) error {
	payload, err := p.call(context.Background(), MsgObjectLockRequest, &ObjectLockRequest{
		RpcID:   p.nextRPCID(),
		Type:    LocationType(locationType),
		Key:     key,
		ActorID: actorID,
		Time:    timeMs,
	})
	if err != nil {
		return err
	}
	return parseCommonResponse(MsgObjectLockRequest, payload)
}

// Unlock 解锁。
func (p *LocationProxyComponent) Unlock(locationType int, key int64, oldActorID, newActorID actor.ActorID) error {
	payload, err := p.call(context.Background(), MsgObjectUnlockRequest, &ObjectUnlockRequest{
		RpcID:      p.nextRPCID(),
		Type:       LocationType(locationType),
		Key:        key,
		OldActorID: oldActorID,
		NewActorID: newActorID,
	})
	if err != nil {
		return err
	}
	return parseCommonResponse(MsgObjectUnlockRequest, payload)
}

// Remove 删除位置。
func (p *LocationProxyComponent) Remove(locationType int, key int64) error {
	payload, err := p.call(context.Background(), MsgObjectRemoveRequest, &ObjectRemoveRequest{
		RpcID: p.nextRPCID(),
		Type:  LocationType(locationType),
		Key:   key,
	})
	if err != nil {
		return err
	}
	return parseCommonResponse(MsgObjectRemoveRequest, payload)
}

func (p *LocationProxyComponent) call(ctx context.Context, msgID uint16, req any) ([]byte, error) {
	if p == nil {
		return nil, ErrLocationProxyRequired
	}
	p.mu.RLock()
	caller := p.caller
	locationActor := p.locationActor
	p.mu.RUnlock()
	if caller == nil {
		return nil, ErrProxyCallerRequired
	}
	if !locationActor.IsValid() {
		locationActor = p.resolveLocationActor()
		if locationActor.IsValid() {
			p.mu.Lock()
			if !p.locationActor.IsValid() {
				p.locationActor = locationActor
			} else {
				locationActor = p.locationActor
			}
			p.mu.Unlock()
		}
	}
	if !locationActor.IsValid() {
		return nil, ErrLocationActorRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := marshalRequest(msgID, req)
	if err != nil {
		return nil, err
	}
	return caller.Call(ctx, locationActor, msgID, payload)
}

func (p *LocationProxyComponent) resolveLocationActor() actor.ActorID {
	if p == nil {
		return actor.ActorID{}
	}
	p.mu.RLock()
	locationActor := p.locationActor
	p.mu.RUnlock()
	if locationActor.IsValid() {
		return locationActor
	}
	entity := p.GetEntity()
	if entity == nil || entity.Scene() == nil {
		return actor.ActorID{}
	}
	actorID, ok := actor.ResolveSceneActor(entity.Scene().Zone(), ecs.SceneTypeLocation, "")
	if !ok {
		return actor.ActorID{}
	}
	return actorID
}

func (p *LocationProxyComponent) nextRPCID() uint32 {
	if p == nil {
		return 0
	}
	for {
		id := p.rpcID.Add(1)
		if id != 0 {
			return id
		}
	}
}

func parseCommonResponse(msgID uint16, payload []byte) error {
	if len(payload) == 0 {
		// protobuf 的零值成功响应可以编码为空字节；这里不能把合法零值
		// 响应误判成依赖缺失。
		return nil
	}

	resp, err := unmarshalCommonResponse(msgID, payload)
	if err != nil {
		return err
	}
	if resp.Error != 0 {
		return &ResponseError{Code: resp.Error, Message: resp.Message}
	}
	return nil
}
