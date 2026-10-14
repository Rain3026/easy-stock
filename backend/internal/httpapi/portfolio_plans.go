package httpapi

import (
	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/foundation"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) portfolioPlanRoutes() {
	s.mux.HandleFunc("GET /api/v1/portfolio-plans", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.dailyResearch.store.Portfolio(r.Context())
		if e != nil {
			writeError(w, 500, "读取方案失败")
			return
		}
		writeJSON(w, 200, map[string]any{"data": v})
	})
	s.mux.HandleFunc("GET /api/v1/portfolio-plans/versions", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.dailyResearch.store.PortfolioHistory(r.Context())
		if e != nil {
			writeError(w, 500, "读取方案版本失败")
			return
		}
		writeJSON(w, 200, map[string]any{"data": v})
	})
	s.mux.HandleFunc("PUT /api/v1/portfolio-plans", s.portfolioPlanSave)
}
func validatePortfolioState(raw json.RawMessage) error {
	var state struct {
		ActiveID string `json:"activeId"`
		Plans    []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Draft struct {
				TotalAssets float64 `json:"totalAssets"`
				Profile     string  `json:"profile"`
				Holdings    []struct {
					Symbol string  `json:"symbol"`
					Weight float64 `json:"weight"`
					Cost   string  `json:"costPrice"`
				} `json:"holdings"`
			} `json:"draft"`
		} `json:"plans"`
	}
	if json.Unmarshal(raw, &state) != nil || len(state.Plans) == 0 || len(state.Plans) > 100 {
		return errors.New("无效的方案集合")
	}
	ids := map[string]bool{}
	for _, p := range state.Plans {
		if p.ID == "" || len(p.ID) > 150 || ids[p.ID] || len([]rune(p.Name)) > 80 || strings.TrimSpace(p.Name) == "" || p.Draft.TotalAssets <= 0 {
			return errors.New("方案名称、资产或标识无效")
		}
		ids[p.ID] = true
		if p.Draft.Profile != "balanced" && p.Draft.Profile != "steady" && p.Draft.Profile != "aggressive" {
			return errors.New("交易风格无效")
		}
		if len(p.Draft.Holdings) > 10 {
			return errors.New("单方案最多10只持仓")
		}
		sum := 0.0
		symbols := map[string]bool{}
		for _, h := range p.Draft.Holdings {
			n, e := foundation.NormalizeSymbol(h.Symbol)
			if e != nil || symbols[n.Canonical] || h.Weight < 0 || h.Weight > 100 {
				return errors.New("持仓代码或占比无效")
			}
			symbols[n.Canonical] = true
			sum += h.Weight
			if h.Cost != "" {
				if cost, e := strconv.ParseFloat(h.Cost, 64); e != nil || cost <= 0 || math.IsInf(cost, 0) || math.IsNaN(cost) {
					return errors.New("成本须为正数或留空")
				}
			}
		}
		if sum > 100.000001 {
			return errors.New("持仓占比总和超过100%")
		}
	}
	if !ids[state.ActiveID] {
		return errors.New("当前方案不存在")
	}
	return nil
}
func (s *Server) portfolioPlanSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State    json.RawMessage `json:"state"`
		Expected int64           `json:"expected_revision"`
		Origin   string          `json:"origin"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || ensureJSONEOF(dec) != nil {
		writeError(w, 400, "无效的方案保存请求")
		return
	}
	if e := validatePortfolioState(req.State); e != nil {
		writeError(w, 400, e.Error())
		return
	}
	if req.Origin != "browser-import" && req.Origin != "restore" {
		req.Origin = "edit"
	}
	v, e := s.dailyResearch.store.SavePortfolio(r.Context(), req.State, req.Expected, req.Origin)
	if errors.Is(e, dr.ErrPlanConflict) {
		writeJSON(w, 409, map[string]any{"error": "方案已被其他页面更新，本地改动保留，请合并后重试", "data": v})
		return
	}
	if e != nil {
		writeError(w, 500, "保存方案失败，本地副本保留")
		return
	}
	writeJSON(w, 200, map[string]any{"data": v})
}
