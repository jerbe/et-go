package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

type stubHTTPHandler struct {
	err error
}

func (h *stubHTTPHandler) Handle(_ *ecs.Scene, _ *nethttp.Request, resp nethttp.ResponseWriter) error {
	if h.err != nil {
		return h.err
	}
	WriteJSON(resp, nethttp.StatusOK, HttpResponse{})
	return nil
}

func TestDispatcher(t *testing.T) {
	dispatcher := &HttpDispatcher{}
	dispatcher.Register("/ok", &stubHTTPHandler{})

	req := httptest.NewRequest(nethttp.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	dispatcher.Dispatch(ecs.NewScene(ecs.SceneTypeHTTP, 1, "http"), rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q, want application/json; charset=utf-8", got)
	}

	req = httptest.NewRequest(nethttp.MethodGet, "/missing", nil)
	rec = httptest.NewRecorder()
	dispatcher.Dispatch(nil, rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var notFoundResp HttpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &notFoundResp); err != nil {
		t.Fatalf("decode 404 resp err = %v", err)
	}
	if notFoundResp.Error != nethttp.StatusNotFound || notFoundResp.Message != "404 Page Not Found" {
		t.Fatalf("unexpected 404 resp %+v", notFoundResp)
	}

	dispatcher.Register("/err", &stubHTTPHandler{err: errors.New("boom")})
	req = httptest.NewRequest(nethttp.MethodGet, "/err", nil)
	rec = httptest.NewRecorder()
	dispatcher.Dispatch(nil, rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var internalResp HttpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &internalResp); err != nil {
		t.Fatalf("decode 500 resp err = %v", err)
	}
	if internalResp.Error != nethttp.StatusInternalServerError || internalResp.Message != "500 Internal Server Error" {
		t.Fatalf("unexpected 500 resp %+v", internalResp)
	}
}
