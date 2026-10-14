package httpapi

import (
	"context"
	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/datahub"
	"easy-stock/backend/internal/foundation"
	"easy-stock/backend/internal/marketpulse"
	"easy-stock/backend/internal/stockanalysis"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) evidenceRoutes() {
	s.mux.HandleFunc("GET /api/v1/evidence", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.dailyResearch.store.Facts().List(r.Context(), r.URL.Query().Get("entity"), 50)
		if e != nil {
			writeError(w, 400, "证据检索失败")
			return
		}
		writeJSON(w, 200, map[string]any{"data": v})
	})
	s.mux.HandleFunc("GET /api/v1/evidence/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.dailyResearch.store.Facts().Get(r.Context(), r.PathValue("id"))
		if e != nil {
			writeError(w, 404, "证据快照不存在")
			return
		}
		writeJSON(w, 200, map[string]any{"data": v})
	})
}
func sourceRef(id string, m foundation.SourceMeta, at *time.Time) datahub.EvidenceSource {
	family := m.Family
	if family == "" {
		family = strings.Split(m.Source, ":")[0]
	}
	state := "observed"
	if m.Stale {
		state = "stale"
	}
	return datahub.EvidenceSource{ID: id, Family: family, Channel: m.Channel, URL: m.SourceURL, DataAt: at, CapturedAt: m.FetchedAt, Status: state}
}
func (s *Server) freezeDailyEvidence(ctx context.Context, v *dr.Snapshot) error {
	if v.MarketPulse != nil {
		themes := []foundation.ThemeOverview{}
		var meta foundation.SourceMeta
		for _, t := range v.Themes {
			themes = append(themes, t.Overview)
			meta.Stale = meta.Stale || t.Meta.Stale
		}
		j := marketpulse.Judge(*v.MarketPulse, v.Indexes, themes, meta, v.Cutoff)
		v.Judgement = &j
	}
	copy := *v
	copy.EvidenceID = ""
	raw, e := json.Marshal(copy)
	if e != nil {
		return e
	}
	refs := []datahub.EvidenceSource{sourceRef("indexes", v.IndexMeta, nil)}
	for _, t := range v.Themes {
		refs = append(refs, sourceRef("theme:"+t.Overview.Theme, t.Meta, nil))
	}
	if v.MarketPulse != nil {
		refs = append(refs, datahub.EvidenceSource{ID: "market-pulse", Family: v.MarketPulse.Source, DataAt: timePointer(v.MarketPulse.At), CapturedAt: v.MarketPulse.FetchedAt, Status: v.Status})
	}
	f, e := s.dailyResearch.store.Facts().Freeze(ctx, datahub.FactSnapshot{EntityID: "market:cn", Kind: "market-evidence", OriginID: v.ID, MethodVersion: v.Version, CapturedAt: v.Cutoff, CutoffAt: v.Cutoff, Sources: refs, Gaps: v.Gaps, Payload: raw})
	if e == nil {
		v.EvidenceID = f.ID
	}
	return e
}
func timePointer(raw string) *time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if at, e := time.ParseInLocation(layout, raw, foundation.AStockLocation); e == nil {
			return &at
		}
	}
	return nil
}
func (s *Server) freezeStockEvidence(ctx context.Context, v *stockanalysis.ResearchSnapshot) error {
	copy := *v
	copy.EvidenceID = ""
	copy.SkillRefs = nil
	copy.ToolCalls = nil
	raw, e := json.Marshal(copy)
	if e != nil {
		return e
	}
	refs := []datahub.EvidenceSource{}
	for _, src := range v.Sources {
		ref := datahub.EvidenceSource{ID: src.ID, Family: src.Provider, URL: src.URL, CapturedAt: src.CapturedAt, Status: src.TimeStatus}
		if !src.PublishedAt.IsZero() {
			at := src.PublishedAt
			ref.PublishedAt = &at
		}
		refs = append(refs, ref)
	}
	// A stage-only update resolves to the same content-addressed fact ID. A
	// supplement changes the evidence body and therefore gets a different ID.
	f, e := s.dailyResearch.store.Facts().Freeze(ctx, datahub.FactSnapshot{EntityID: "stock:" + v.Symbol, Kind: "stock-evidence", OriginID: v.ID, MethodVersion: stockanalysis.ResearchCompressionVersion, CapturedAt: v.CapturedAt, CutoffAt: v.CutoffAt, Sources: refs, Gaps: v.Limitations, Payload: raw})
	if e == nil {
		v.EvidenceID = f.ID
	}
	return e
}
