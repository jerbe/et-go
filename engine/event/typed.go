package event

import (
	"reflect"
	"sync"
)

var typedEventIDCache sync.Map

func eventIDFromType[T any]() EventID {
	typ := reflect.TypeFor[T]()
	if cached, ok := typedEventIDCache.Load(typ); ok {
		return cached.(EventID)
	}

	eventID := EventID(typ.String())
	typedEventIDCache.Store(typ, eventID)
	return eventID
}

// Subscribe 泛型订阅，提供编译期类型安全。
func Subscribe[T any](bus *Bus, handler func(T)) func() {
	if bus == nil || handler == nil {
		return func() {}
	}
	return bus.Subscribe(eventIDFromType[T](), func(args any) {
		handler(args.(T))
	})
}

// SubscribeOnce 泛型一次性订阅。
func SubscribeOnce[T any](bus *Bus, handler func(T)) func() {
	if bus == nil || handler == nil {
		return func() {}
	}
	return bus.SubscribeOnce(eventIDFromType[T](), func(args any) {
		handler(args.(T))
	})
}

// Publish 泛型发布。
func Publish[T any](bus *Bus, args T) {
	if bus == nil {
		return
	}
	bus.Publish(eventIDFromType[T](), args)
}
