package http

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerbe/et-go/config"
)

func TestAreaListHandler(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)

	config.SetGlobal(&config.Config{
		Areas: []config.StartAreaConfig{
			{ID: 1, Name: "Area 1", ServerURL: "http://area-1"},
			{ID: 2, Name: "Area 2", ServerURL: "http://area-2"},
		},
	})
	req := httptest.NewRequest(nethttp.MethodGet, "/get_area_list", nil)
	rec := httptest.NewRecorder()
	if err := (&HttpGetAreaListHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp HttpAreaListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode err = %v", err)
	}
	if len(resp.Areas) != 2 || resp.Areas[0].ServerURL != "http://area-1" || resp.Areas[1].Name != "Area 2" {
		t.Fatalf("unexpected areas %+v", resp.Areas)
	}
}

func TestAreaListHandlerDoesNotFallbackToZones(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)

	config.SetGlobal(&config.Config{
		Zones: []config.StartZoneConfig{
			{ID: 1, Name: "Zone 1", ServerURL: "http://zone-1", IsLogic: true},
		},
	})
	req := httptest.NewRequest(nethttp.MethodGet, "/get_area_list", nil)
	rec := httptest.NewRecorder()
	if err := (&HttpGetAreaListHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	var resp HttpAreaListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode err = %v", err)
	}
	if len(resp.Areas) != 0 {
		t.Fatalf("unexpected areas %+v", resp.Areas)
	}
}

func TestAreaListHandlerRejectsMissingGlobalConfiguration(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)
	config.SetGlobal(nil)

	req := httptest.NewRequest(nethttp.MethodGet, "/get_area_list", nil)
	rec := httptest.NewRecorder()
	if err := (&HttpGetAreaListHandler{}).Handle(nil, req, rec); err != ErrConfigurationMissing {
		t.Fatalf("Handle without config error = %v, want %v", err, ErrConfigurationMissing)
	}
}

func TestAreaListHandlerRejectsNonGetMethod(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)
	config.SetGlobal(&config.Config{})

	req := httptest.NewRequest(nethttp.MethodPost, "/get_area_list", nil)
	rec := httptest.NewRecorder()
	if err := (&HttpGetAreaListHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("Handle non-GET error = %v", err)
	}
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("non-GET status = %d, want %d", rec.Code, nethttp.StatusOK)
	}
}
