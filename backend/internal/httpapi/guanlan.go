package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/guanlan"
	"easy-stock/backend/internal/portfolioinspection"
)

func (s *Server) initGuanlan(cfg Config) {
	path := cfg.GuanlanDBPath
	if path == "" && cfg.SettingsPath != "" {
		path = filepath.Join(filepath.Dir(cfg.SettingsPath), "guanlan.db")
	}
	store, e := guanlan.Open(path)
	if e != nil {
		s.startupError = errors.Join(s.startupError, e)
		store, _ = guanlan.Open(":memory:")
	}
	s.guanlan = store
}
func boardBody(w http.ResponseWriter, r *http.Request, value any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(value) != nil || ensureJSONEOF(dec) != nil {
		writeError(w, 400, "无效的看板请求")
		return false
	}
	return true
}
func boardResult(w http.ResponseWriter, value any, e error) {
	if e == nil {
		writeJSON(w, 200, map[string]any{"data": value})
		return
	}
	switch {
	case errors.Is(e, guanlan.ErrConflict):
		writeError(w, 409, e.Error())
	case errors.Is(e, guanlan.ErrNotFound):
		writeError(w, 404, e.Error())
	case errors.Is(e, guanlan.ErrInvalid):
		writeError(w, 400, e.Error())
	default:
		writeError(w, 500, "看板数据读取或保存失败，原配置保留")
	}
}
func boardRevision(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.URL.Query().Get("expected_revision"), 10, 64)
}
func (s *Server) guanlanRoutes() {
	s.mux.HandleFunc("GET /api/v1/boards/catalog", func(w http.ResponseWriter, r *http.Request) {
		refs, e := s.guanlan.References(r.Context())
		if e != nil {
			boardResult(w, nil, e)
			return
		}
		writeJSON(w, 200, map[string]any{"data": guanlan.Catalog(), "references": refs})
	})
	s.mux.HandleFunc("GET /api/v1/boards/state", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.guanlan.Snapshot(r.Context())
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("POST /api/v1/boards/drafts", func(w http.ResponseWriter, r *http.Request) {
		var req guanlan.CreateRequest
		if !boardBody(w, r, &req) {
			return
		}
		v, e := s.guanlan.CreateDraft(r.Context(), req)
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("PUT /api/v1/boards/drafts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Config   guanlan.Config `json:"config"`
			Expected int64          `json:"expected_revision"`
		}
		if !boardBody(w, r, &req) {
			return
		}
		v, e := s.guanlan.UpdateDraft(r.Context(), r.PathValue("id"), req.Config, req.Expected)
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("DELETE /api/v1/boards/drafts/{id}", func(w http.ResponseWriter, r *http.Request) {
		rev, e := boardRevision(r)
		if e != nil {
			writeError(w, 400, "缺少草稿修订号")
			return
		}
		e = s.guanlan.DiscardDraft(r.Context(), r.PathValue("id"), rev)
		boardResult(w, true, e)
	})
	s.mux.HandleFunc("POST /api/v1/boards/drafts/{id}/save", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Expected       int64 `json:"expected_revision"`
			Activate       bool  `json:"activate"`
			ActiveRevision int64 `json:"active_revision"`
		}
		if !boardBody(w, r, &req) {
			return
		}
		v, e := s.guanlan.SaveDraft(r.Context(), r.PathValue("id"), req.Expected, req.Activate, req.ActiveRevision)
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("GET /api/v1/boards/versions", func(w http.ResponseWriter, r *http.Request) {
		page, size := 1, 2
		var e error
		if r.URL.Query().Has("page") {
			page, e = strconv.Atoi(r.URL.Query().Get("page"))
			if e != nil {
				writeError(w, 400, "无效页码")
				return
			}
		}
		if r.URL.Query().Has("page_size") {
			size, e = strconv.Atoi(r.URL.Query().Get("page_size"))
			if e != nil {
				writeError(w, 400, "无效每页条数")
				return
			}
		}
		v, e := s.guanlan.Versions(r.Context(), r.URL.Query().Get("q"), page, size, r.URL.Query().Get("archived") == "1")
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("GET /api/v1/boards/versions/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.guanlan.Version(r.Context(), r.PathValue("id"))
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("PATCH /api/v1/boards/versions/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name     *string `json:"name"`
			Archived *bool   `json:"archived"`
			Expected int64   `json:"expected_revision"`
		}
		if !boardBody(w, r, &req) {
			return
		}
		v, e := s.guanlan.Metadata(r.Context(), r.PathValue("id"), req.Name, req.Archived, req.Expected)
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("POST /api/v1/boards/active", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       string `json:"version_id"`
			Expected int64  `json:"expected_revision"`
		}
		if !boardBody(w, r, &req) {
			return
		}
		v, e := s.guanlan.Activate(r.Context(), req.ID, req.Expected)
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("PUT /api/v1/boards/library", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Library  map[string]guanlan.Preference `json:"library"`
			Expected int64                         `json:"expected_revision"`
		}
		if !boardBody(w, r, &req) {
			return
		}
		e := s.guanlan.Preferences(r.Context(), req.Library, req.Expected)
		boardResult(w, true, e)
	})
	s.mux.HandleFunc("POST /api/v1/boards/presets", func(w http.ResponseWriter, r *http.Request) {
		var req guanlan.Preset
		if !boardBody(w, r, &req) {
			return
		}
		v, e := s.guanlan.PutPreset(r.Context(), req)
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("DELETE /api/v1/boards/presets/{id}", func(w http.ResponseWriter, r *http.Request) {
		rev, e := boardRevision(r)
		if e != nil {
			writeError(w, 400, "缺少预设修订号")
			return
		}
		e = s.guanlan.DeletePreset(r.Context(), r.PathValue("id"), rev)
		boardResult(w, true, e)
	})
	s.mux.HandleFunc("POST /api/v1/boards/watchlists", func(w http.ResponseWriter, r *http.Request) {
		var req guanlan.Watchlist
		if !boardBody(w, r, &req) {
			return
		}
		v, e := s.guanlan.PutWatchlist(r.Context(), req)
		boardResult(w, v, e)
	})
	s.mux.HandleFunc("GET /api/v1/boards/research-results", s.boardResearchResults)
	s.mux.HandleFunc("GET /api/v1/boards/risk-results", s.boardRiskResults)
}

// No model, MCP, or data-provider call is made here. Evidence stays in its original archive.
func (s *Server) boardResearchResults(w http.ResponseWriter, r *http.Request) {
	ids, ok := boardBindingIDs(w, r, "ids")
	if !ok {
		return
	}
	snapshots, ok := boardBindingIDs(w, r, "snapshots")
	if !ok {
		return
	}
	jobs, e := s.dailyResearch.store.Jobs(r.Context())
	if e != nil {
		boardResult(w, nil, e)
		return
	}
	type result struct {
		ID          string         `json:"id"`
		UpdatedAt   any            `json:"updated_at"`
		GeneratedAt any            `json:"generated_at,omitempty"`
		Status      string         `json:"status"`
		Error       string         `json:"error,omitempty"`
		Subject     string         `json:"subject"`
		Debate      *dr.Debate     `json:"debate"`
		Snapshot    map[string]any `json:"snapshot"`
	}
	values := []result{}
	// Keep the normal picker bounded, but explicitly resolve saved historical IDs.
	base := []dr.Job{}
	for _, job := range jobs {
		if job.Kind == "debate" || job.Debate != nil {
			base = append(base, job)
			if len(base) == 30 {
				break
			}
		}
	}
	jobs = base
	for _, id := range ids {
		job, e := s.dailyResearch.store.Job(r.Context(), id)
		if e == nil {
			jobs = append(jobs, job)
		} else if !errors.Is(e, sql.ErrNoRows) {
			boardResult(w, nil, e)
			return
		}
	}
	for _, id := range snapshots {
		old, e := s.dailyResearch.store.JobsForSnapshot(r.Context(), id)
		if e != nil {
			boardResult(w, nil, e)
			return
		}
		jobs = append(jobs, old...)
	}
	seen := map[string]bool{}
	snapshotCache := map[string]dr.Snapshot{}
	for _, job := range jobs {
		if seen[job.ID] {
			continue
		}
		seen[job.ID] = true
		if job.Kind != "debate" && job.Debate == nil {
			continue
		}
		snapshotID := job.SnapshotID
		if job.Debate != nil {
			snapshotID = job.Debate.SnapshotID
		}
		snap, cached := snapshotCache[snapshotID]
		if !cached {
			var e error
			snap, e = s.dailyResearch.store.Snapshot(r.Context(), snapshotID)
			if e != nil {
				continue
			}
			snapshotCache[snapshotID] = snap
		}
		var argument struct {
			Subject string `json:"subject"`
		}
		_ = json.Unmarshal(job.Request, &argument)
		var debate *dr.Debate
		if job.Debate != nil {
			safe := guanlan.Clone(*job.Debate)
			for i := range safe.Responses {
				safe.Responses[i].Raw = ""
			}
			debate = &safe
			argument.Subject = safe.Subject
		}
		var generated any
		if job.Status == "succeeded" && job.Debate != nil {
			generated = job.UpdatedAt
		}
		values = append(values, result{GeneratedAt: generated, ID: job.ID, UpdatedAt: job.UpdatedAt, Status: job.Status, Error: job.Error, Subject: argument.Subject, Debate: debate,
			Snapshot: map[string]any{"id": snap.ID, "version": snap.Version, "trade_date": snap.TradeDate, "cutoff": snap.Cutoff, "gaps": snap.Gaps, "evidence_id": snap.EvidenceID}})
	}
	boardResult(w, values, nil)
}

func boardBindingIDs(w http.ResponseWriter, r *http.Request, key string) ([]string, bool) {
	values := []string{}
	seen := map[string]bool{}
	raw := r.URL.Query().Get(key)
	if len(raw) > 16000 {
		writeError(w, 400, "绑定参数过长")
		return nil, false
	}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if len(id) > 200 || len(values) >= 60 {
			writeError(w, 400, "绑定参数数量或长度超限")
			return nil, false
		}
		if !seen[id] {
			values = append(values, id)
			seen[id] = true
		}
	}
	return values, true
}
func (s *Server) boardRiskResults(w http.ResponseWriter, r *http.Request) {
	if s.portfolioInspection == nil {
		writeError(w, 503, "持仓巡检服务不可用")
		return
	}
	ids, ok := boardBindingIDs(w, r, "ids")
	if !ok {
		return
	}
	plans, ok := boardBindingIDs(w, r, "plans")
	if !ok {
		return
	}
	jobs, e := s.portfolioInspection.ListPage(r.Context(), 30, 0)
	if e != nil {
		boardResult(w, nil, e)
		return
	}
	for _, id := range ids {
		job, e := s.portfolioInspection.Get(r.Context(), id)
		if e == nil {
			jobs = append(jobs, job)
		} else if !errors.Is(e, sql.ErrNoRows) {
			boardResult(w, nil, e)
			return
		}
	}
	for _, id := range plans {
		old, e := s.portfolioInspection.ListPage(r.Context(), 1, 0, id)
		if e != nil {
			boardResult(w, nil, e)
			return
		}
		jobs = append(jobs, old...)
	}
	values := []map[string]any{}
	seen := map[string]bool{}
	for _, job := range jobs {
		if seen[job.ID] {
			continue
		}
		seen[job.ID] = true
		values = append(values, summarizeRiskJob(job))
	}
	boardResult(w, values, nil)
}
func summarizeRiskJob(job portfolioinspection.Job) map[string]any {
	var report any
	if r := job.Report; r != nil {
		evidence := []map[string]any{}
		for _, h := range r.Holdings {
			at := h.ResearchCutoffAt
			if at.IsZero() && h.Analysis != nil {
				at = h.Analysis.ResearchReport.CutoffAt
			}
			evidence = append(evidence, map[string]any{"label": h.Holding.Symbol + " 证据截至", "at": at})
			if h.CurrentQuote != nil {
				evidence = append(evidence, map[string]any{"label": h.Holding.Symbol + " 行情", "at": h.CurrentQuote.TradeTime, "source": h.CurrentQuote.Meta.Source})
			}
		}
		report = map[string]any{"evidence_times": evidence, "id": r.ID, "generated_at": r.GeneratedAt, "prompt_version": r.PromptVersion, "algorithm_version": r.AlgorithmVersion, "model": r.Model, "conclusion": map[string]any{"risk_level": r.Conclusion.RiskLevel, "executive_summary": r.Conclusion.ExecutiveSummary, "primary_risks": r.Conclusion.PrimaryRisks, "source": r.Conclusion.Source, "data_limitations": r.Conclusion.DataLimitations}}
	}
	value := map[string]any{"id": job.ID, "status": job.Status, "error": job.Error, "request": map[string]any{"portfolio_plan_id": job.Request.PortfolioPlanID}, "started_at": job.StartedAt, "completed_at": job.CompletedAt, "updated_at": job.UpdatedAt, "report": report}
	if job.TotalStocks > 0 {
		value["coverage_percent"] = job.CoveragePercent
	}
	return value
}
