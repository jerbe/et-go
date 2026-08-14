package actorlocation

import (
	"context"
	"sync"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

// MessageSenderClient 定义按 ActorID 发送消息的能力。
type MessageSenderClient interface {
	Send(actorID actor.ActorID, msgID uint16, payload []byte) error
	Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)
}

// MessageLocationSender 根据位置发送消息。
type MessageLocationSender struct {
	ecs.BaseComponent
	locationType LocationType
	proxy        *LocationProxyComponent
	sender       MessageSenderClient
	mu           sync.Mutex
	cache        map[int64]actor.ActorID
}

// NewMessageLocationSender 创建位置消息发送器。
func NewMessageLocationSender(locationType LocationType, proxy *LocationProxyComponent, sender MessageSenderClient) *MessageLocationSender {
	return &MessageLocationSender{
		locationType: locationType,
		proxy:        proxy,
		sender:       sender,
		cache:        make(map[int64]actor.ActorID),
	}
}

// Type 返回组件类型名称。
func (s *MessageLocationSender) Type() string { return "MessageLocationSender" }

// Send 根据位置发送消息。
func (s *MessageLocationSender) Send(key int64, msgID uint16, payload []byte) error {
	if err := s.validate(); err != nil {
		return err
	}
	actorID, err := s.actorForKey(key)
	if err != nil {
		return err
	}
	if err := s.sender.Send(actorID, msgID, payload); err == nil {
		return nil
	}
	s.invalidate(key)
	actorID, err = s.actorForKey(key)
	if err != nil {
		return err
	}
	return s.sender.Send(actorID, msgID, payload)
}

// Call 根据位置发送 RPC 请求。
func (s *MessageLocationSender) Call(ctx context.Context, key int64, msgID uint16, payload []byte) ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	actorID, err := s.actorForKey(key)
	if err != nil {
		return nil, err
	}
	resp, err := s.sender.Call(ctx, actorID, msgID, payload)
	if err == nil {
		return resp, nil
	}
	s.invalidate(key)
	actorID, err = s.actorForKey(key)
	if err != nil {
		return nil, err
	}
	return s.sender.Call(ctx, actorID, msgID, payload)
}

func (s *MessageLocationSender) actorForKey(key int64) (actor.ActorID, error) {
	if err := s.validate(); err != nil {
		return actor.ActorID{}, err
	}
	if key <= 0 {
		return actor.ActorID{}, ErrZeroLocationKey
	}
	s.mu.Lock()
	if s.cache == nil {
		s.cache = make(map[int64]actor.ActorID)
	}
	if actorID, ok := s.cache[key]; ok && actorID.IsValid() {
		s.mu.Unlock()
		return actorID, nil
	}
	s.mu.Unlock()

	actorID, err := s.proxy.Get(int(s.locationType), key)
	if err != nil {
		return actor.ActorID{}, err
	}
	if !actorID.IsValid() {
		return actor.ActorID{}, ErrLocationNotFound
	}

	s.mu.Lock()
	if s.cache == nil {
		s.cache = make(map[int64]actor.ActorID)
	}
	s.cache[key] = actorID
	s.mu.Unlock()
	return actorID, nil
}

func (s *MessageLocationSender) invalidate(key int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
}

func (s *MessageLocationSender) validate() error {
	if s == nil || s.proxy == nil {
		return ErrLocationProxyRequired
	}
	if s.sender == nil {
		return ErrMessageSenderRequired
	}
	return nil
}
