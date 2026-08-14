package http

import (
	nethttp "net/http"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/ecs"
)

// HttpGetAreaListHandler 处理 `/get_area_list`。
type HttpGetAreaListHandler struct{}

// Handle 返回大区列表。
func (h *HttpGetAreaListHandler) Handle(_ *ecs.Scene, req *nethttp.Request, resp nethttp.ResponseWriter) error {
	if req == nil {
		return ErrRequestMissing
	}
	if resp == nil {
		return ErrResponseWriterMissing
	}
	if req.Method != nethttp.MethodGet {
		WriteNotFound(resp)
		return nil
	}
	cfg := config.GetGlobal()
	if cfg == nil {
		return ErrConfigurationMissing
	}
	areas := make([]AreaInfo, 0)
	for _, areaCfg := range cfg.Areas {
		areas = append(areas, AreaInfo{
			Id:        int32(areaCfg.ID),
			Name:      areaCfg.Name,
			ServerURL: areaCfg.ServerURL,
		})
	}
	WriteJSON(resp, nethttp.StatusOK, HttpAreaListResponse{
		HttpResponse: HttpResponse{},
		Areas:        areas,
	})
	return nil
}
