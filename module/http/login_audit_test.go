package http

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingLoginAuditSink struct {
	mu     sync.Mutex
	events []LoginAuditEvent
	err    error
}

func (s *recordingLoginAuditSink) RecordLoginAudit(_ context.Context, event LoginAuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, event)
	return nil
}

func (s *recordingLoginAuditSink) snapshot() []LoginAuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LoginAuditEvent(nil), s.events...)
}

func TestLoginAuditSinkRequiresExplicitDatabase(t *testing.T) {
	if sink, err := NewMongoLoginAuditSink(nil); sink != nil ||
		!errors.Is(err, ErrLoginAuditSinkUnavailable) {
		t.Fatalf("NewMongoLoginAuditSink result = sink %#v err %v", sink, err)
	}
	if sink, err := NewDBManagerLoginAuditSink(nil, 1); sink != nil ||
		!errors.Is(err, ErrLoginAuditSinkUnavailable) {
		t.Fatalf("NewDBManagerLoginAuditSink result = sink %#v err %v", sink, err)
	}
}

func TestMongoLoginAuditSinkRejectsInvalidEventWithoutFallback(t *testing.T) {
	sink := &MongoLoginAuditSink{}
	if err := sink.RecordLoginAudit(context.Background(), LoginAuditEvent{
		Username: "user",
		At:       time.Time{},
	}); !errors.Is(err, ErrLoginAuditSinkUnavailable) {
		t.Fatalf("RecordLoginAudit error = %v, want %v", err, ErrLoginAuditSinkUnavailable)
	}
}
