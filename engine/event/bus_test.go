package event

import (
	"context"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type capturedLog struct {
	message string
	attrs   map[string]any
}

type captureHandler struct {
	mu      sync.Mutex
	records []capturedLog
	attrs   []slog.Attr
	groups  []string
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any)
	groupPrefix := strings.Join(h.groups, ".")
	for _, attr := range h.attrs {
		key := attr.Key
		if groupPrefix != "" {
			key = groupPrefix + "." + key
		}
		attrs[key] = attr.Value.Any()
	}
	record.Attrs(func(attr slog.Attr) bool {
		key := attr.Key
		if groupPrefix != "" {
			key = groupPrefix + "." + key
		}
		attrs[key] = attr.Value.Any()
		return true
	})

	h.mu.Lock()
	h.records = append(h.records, capturedLog{
		message: record.Message,
		attrs:   attrs,
	})
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cloned := h.clone()
	cloned.attrs = append(cloned.attrs, attrs...)
	return cloned
}

func (h *captureHandler) WithGroup(name string) slog.Handler {
	cloned := h.clone()
	cloned.groups = append(cloned.groups, name)
	return cloned
}

func (h *captureHandler) clone() *captureHandler {
	cloned := &captureHandler{
		records: h.records,
		attrs:   append([]slog.Attr(nil), h.attrs...),
		groups:  append([]string(nil), h.groups...),
	}
	return cloned
}

func (h *captureHandler) snapshot() []capturedLog {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]capturedLog(nil), h.records...)
}

func TestBusSubscribePublishOrder(t *testing.T) {
	bus := NewBus()
	sequence := make([]int, 0, 2)
	bus.Subscribe("demo", func(any) { sequence = append(sequence, 1) })
	bus.Subscribe("demo", func(any) { sequence = append(sequence, 2) })

	bus.Publish("demo", nil)

	if !reflect.DeepEqual(sequence, []int{1, 2}) {
		t.Fatalf("sequence = %v, want [1 2]", sequence)
	}
}

func TestBusCancelAndUnsubscribe(t *testing.T) {
	bus := NewBus()
	callsA := 0
	callsB := 0
	handlerB := func(any) { callsB++ }

	cancelA := bus.Subscribe("evt", func(any) { callsA++ })
	bus.Subscribe("evt", handlerB)

	bus.Publish("evt", nil)
	cancelA()
	cancelA()
	bus.Unsubscribe("evt", handlerB)
	bus.Publish("evt", nil)

	if callsA != 1 {
		t.Fatalf("callsA = %d, want 1", callsA)
	}
	if callsB != 1 {
		t.Fatalf("callsB = %d, want 1", callsB)
	}
}

func TestBusSubscribeOnce(t *testing.T) {
	bus := NewBus()
	calls := 0
	bus.SubscribeOnce("once", func(any) { calls++ })

	bus.Publish("once", nil)
	bus.Publish("once", nil)

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestBusSubscribeOnceCancelBeforePublish(t *testing.T) {
	bus := NewBus()
	calls := 0
	cancel := bus.SubscribeOnce("once", func(any) { calls++ })
	cancel()
	bus.Publish("once", nil)

	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestBusClearAndClearAll(t *testing.T) {
	bus := NewBus()
	first := 0
	second := 0
	bus.Subscribe("a", func(any) { first++ })
	bus.Subscribe("b", func(any) { second++ })

	bus.Clear("a")
	bus.Publish("a", nil)
	bus.Publish("b", nil)
	bus.ClearAll()
	bus.Publish("b", nil)

	if first != 0 {
		t.Fatalf("first = %d, want 0", first)
	}
	if second != 1 {
		t.Fatalf("second = %d, want 1", second)
	}
}

func TestBusDifferentEventIDsIsolation(t *testing.T) {
	bus := NewBus()
	alpha := 0
	beta := 0
	bus.Subscribe("alpha", func(any) { alpha++ })
	bus.Subscribe("beta", func(any) { beta++ })

	bus.Publish("alpha", nil)

	if alpha != 1 {
		t.Fatalf("alpha = %d, want 1", alpha)
	}
	if beta != 0 {
		t.Fatalf("beta = %d, want 0", beta)
	}
}

func TestBusPanicProtectionAndLogging(t *testing.T) {
	handler := &captureHandler{}
	logger := slog.New(handler)
	bus := NewBusWithLogger(logger)

	sequence := make([]int, 0, 2)
	bus.Subscribe("panic.event", func(any) { sequence = append(sequence, 1) })
	bus.Subscribe("panic.event", func(any) { panic("boom") })
	bus.Subscribe("panic.event", func(any) { sequence = append(sequence, 3) })

	bus.Publish("panic.event", nil)

	if !reflect.DeepEqual(sequence, []int{1, 3}) {
		t.Fatalf("sequence = %v, want [1 3]", sequence)
	}

	records := handler.snapshot()
	if len(records) == 0 {
		t.Fatal("expected panic log record")
	}

	last := records[len(records)-1]
	if last.message != "事件处理器 panic" {
		t.Fatalf("log message = %q, want %q", last.message, "事件处理器 panic")
	}
	if got := last.attrs["event_id"]; got != "panic.event" {
		t.Fatalf("event_id = %v, want panic.event", got)
	}
	if got := last.attrs["panic"]; got != "boom" {
		t.Fatalf("panic = %v, want boom", got)
	}
	stack, _ := last.attrs["stack"].(string)
	if stack == "" || !strings.Contains(stack, "goroutine") {
		t.Fatalf("stack = %q, want non-empty goroutine stack", stack)
	}
}

type testEventA struct {
	ID   int64
	Name string
}

type testEventB struct {
	ID int64
}

func TestTypedSubscribePublishAndCancel(t *testing.T) {
	bus := NewBus()
	received := make([]testEventA, 0, 2)
	otherCalls := 0

	cancel := Subscribe[testEventA](bus, func(evt testEventA) {
		received = append(received, evt)
	})
	Subscribe[testEventB](bus, func(testEventB) {
		otherCalls++
	})

	Publish(bus, testEventA{ID: 7, Name: "alpha"})
	Publish(bus, testEventB{ID: 11})
	cancel()
	Publish(bus, testEventA{ID: 8, Name: "beta"})

	if len(received) != 1 {
		t.Fatalf("received count = %d, want 1", len(received))
	}
	if received[0].ID != 7 || received[0].Name != "alpha" {
		t.Fatalf("received = %+v, want {ID:7 Name:alpha}", received[0])
	}
	if otherCalls != 1 {
		t.Fatalf("otherCalls = %d, want 1", otherCalls)
	}
}

func TestTypedSubscribeOnce(t *testing.T) {
	bus := NewBus()
	count := 0
	SubscribeOnce[testEventA](bus, func(testEventA) { count++ })

	Publish(bus, testEventA{ID: 1})
	Publish(bus, testEventA{ID: 2})

	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func BenchmarkEventIDFromTypeCached(b *testing.B) {
	for index := 0; index < b.N; index++ {
		_ = eventIDFromType[testEventA]()
	}
}
