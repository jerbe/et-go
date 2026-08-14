package event

import (
	"log/slog"
	"reflect"
	"runtime/debug"
	"sync"
)

// EventID 事件标识
type EventID string

// Handler 事件处理函数
type Handler func(args any)

type handlerEntry struct {
	id      uint64
	handler Handler
	once    bool
}

// Bus 是事件总线，实现发布/订阅模式。
// 对应 ET 框架的 AEvent<Scene, Args> 事件系统。
type Bus struct {
	mu       sync.RWMutex
	handlers map[EventID][]handlerEntry
	nextID   uint64
	logger   *slog.Logger
}

// NewBus 创建事件总线
func NewBus() *Bus {
	return NewBusWithLogger(slog.Default())
}

// NewBusWithLogger 使用指定 logger 创建事件总线。
func NewBusWithLogger(logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{
		handlers: make(map[EventID][]handlerEntry),
		logger:   logger,
	}
}

// Subscribe 订阅事件，返回取消函数。
func (b *Bus) Subscribe(eventID EventID, handler Handler) func() {
	if b == nil || handler == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextID++
	entryID := b.nextID
	b.handlers[eventID] = append(b.handlers[eventID], handlerEntry{
		id:      entryID,
		handler: handler,
	})
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.unsubscribeByID(eventID, entryID)
		})
	}
}

// SubscribeOnce 一次性订阅事件，返回取消函数。
func (b *Bus) SubscribeOnce(eventID EventID, handler Handler) func() {
	if b == nil || handler == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextID++
	entryID := b.nextID
	b.handlers[eventID] = append(b.handlers[eventID], handlerEntry{
		id:      entryID,
		handler: handler,
		once:    true,
	})
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.unsubscribeByID(eventID, entryID)
		})
	}
}

// Unsubscribe 按 handler 取消订阅。
func (b *Bus) Unsubscribe(eventID EventID, handler Handler) {
	if b == nil || handler == nil {
		return
	}
	pointer := reflect.ValueOf(handler).Pointer()
	if pointer == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	entries, ok := b.handlers[eventID]
	if !ok || len(entries) == 0 {
		return
	}

	filtered := entries[:0]
	for _, entry := range entries {
		if entry.handler == nil || reflect.ValueOf(entry.handler).Pointer() == pointer {
			continue
		}
		filtered = append(filtered, entry)
	}
	if len(filtered) == 0 {
		delete(b.handlers, eventID)
		return
	}
	b.handlers[eventID] = append([]handlerEntry(nil), filtered...)
}

// Publish 发布事件（同步调用所有处理器）
func (b *Bus) Publish(eventID EventID, args any) {
	if b == nil {
		return
	}
	b.mu.RLock()
	entries := append([]handlerEntry(nil), b.handlers[eventID]...)
	b.mu.RUnlock()

	if len(entries) == 0 {
		return
	}

	onceIDs := make([]uint64, 0)
	for _, entry := range entries {
		b.safeCall(eventID, entry.handler, args)
		if entry.once {
			onceIDs = append(onceIDs, entry.id)
		}
	}

	if len(onceIDs) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, entryID := range onceIDs {
		b.removeEntryLocked(eventID, entryID)
	}
}

// Clear 清除指定事件的所有处理器
func (b *Bus) Clear(eventID EventID) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.handlers, eventID)
}

// ClearAll 清除所有事件的处理器。
func (b *Bus) ClearAll() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = make(map[EventID][]handlerEntry)
	b.nextID = 0
}

func (b *Bus) safeCall(eventID EventID, handler Handler, args any) {
	if handler == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil && b.logger != nil {
			b.logger.Error(
				"事件处理器 panic",
				"event_id", string(eventID),
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
		}
	}()
	handler(args)
}

func (b *Bus) unsubscribeByID(eventID EventID, entryID uint64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeEntryLocked(eventID, entryID)
}

func (b *Bus) removeEntryLocked(eventID EventID, entryID uint64) {
	entries, ok := b.handlers[eventID]
	if !ok || len(entries) == 0 {
		return
	}

	filtered := entries[:0]
	for _, entry := range entries {
		if entry.id == entryID {
			continue
		}
		filtered = append(filtered, entry)
	}
	if len(filtered) == 0 {
		delete(b.handlers, eventID)
		return
	}
	b.handlers[eventID] = append([]handlerEntry(nil), filtered...)
}
