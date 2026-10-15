package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/guanlan"
	"easy-stock/backend/internal/portfolioinspection"
)

func TestGuanlanHTTPContractsAndNoAgentRequirement(t *testing.T) {
	store, e := guanlan.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	// No Agent gateway is configured. All display/configuration operations must work without it.
	s := &Server{mux: http.NewServeMux(), guanlan: store}
	s.guanlanRoutes()
	call := func(method, path, body string, code int) map[string]json.RawMessage {
		t.Helper()
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != code {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var out map[string]json.RawMessage
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	cat := call("GET", "/api/v1/boards/catalog", "", 200)
	var defs []guanlan.Definition
	json.Unmarshal(cat["data"], &defs)
	if len(defs) != 8 {
		t.Fatal("wrong initial catalog")
	}
	for _, d := range defs {
		if e := guanlan.ValidateInstance(d.Default); e != nil {
			t.Fatal(d.ID, e)
		}
	}
	call("POST", "/api/v1/boards/drafts", `{"name":"非法","script":"run"}`, 400)
	created := call("POST", "/api/v1/boards/drafts", `{"name":"我的配置"}`, 200)
	var draft guanlan.Draft
	json.Unmarshal(created["data"], &draft)
	call("PUT", "/api/v1/boards/drafts/"+draft.ID, `{"config":{"schema":1,"name":"编辑","snap":true,"items":[]},"expected_revision":0}`, 409)
	result := call("POST", "/api/v1/boards/drafts/"+draft.ID+"/save", `{"expected_revision":1,"activate":false,"active_revision":0}`, 200)
	var saved guanlan.SaveResult
	json.Unmarshal(result["data"], &saved)
	call("GET", "/api/v1/boards/versions?page=1&page_size=2", "", 200)
	call("GET", "/api/v1/boards/versions?page=-1", "", 400)
	call("POST", "/api/v1/boards/active", `{"version_id":"`+saved.Version.ID+`","expected_revision":0}`, 200)
	call("PATCH", "/api/v1/boards/versions/"+saved.Version.ID, `{"archived":true,"expected_revision":1}`, 400)
	call("POST", "/api/v1/boards/watchlists", `{"name":"关注","symbols":["600000.SH","600000.SH"],"revision":0}`, 200)
	call("GET", "/api/v1/boards/state", "", 200)
}

func TestGuanlanPinnedResearchBeyondRecentQueueIsReadOnly(t *testing.T) {
	ctx := context.Background()
	store, e := dr.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	snap := dr.Snapshot{ID: "frozen-evidence", Version: dr.Version, Cutoff: time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC), TradeDate: "2026-10-14"}
	if e = store.Append(ctx, snap.ID, "snapshot", snap.ID, snap); e != nil {
		t.Fatal(e)
	}
	old := dr.Job{ID: "pinned-old", Kind: "debate", SnapshotID: snap.ID, Status: "succeeded", Debate: &dr.Debate{SnapshotID: snap.ID, Subject: "fixture", Responses: []dr.Response{{Raw: "large raw content must not be sent to widgets", Argument: dr.Argument{Thesis: "frozen fixture conclusion"}}}}}
	if e = store.SaveJob(ctx, old); e != nil {
		t.Fatal(e)
	}
	for range 110 {
		if e = store.SaveJob(ctx, dr.Job{ID: dr.NewID(), Kind: "verification", SnapshotID: snap.ID, Status: "succeeded"}); e != nil {
			t.Fatal(e)
		}
	}
	s := &Server{dailyResearch: &dailyResearchEngine{store: store}}
	w := httptest.NewRecorder()
	s.boardResearchResults(w, httptest.NewRequest("GET", "/api/v1/boards/research-results?ids=pinned-old&snapshots=frozen-evidence", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	raw := w.Body.String()
	if !strings.Contains(raw, "pinned-old") || !strings.Contains(raw, "frozen fixture conclusion") || strings.Contains(raw, "large raw content") {
		t.Fatal("pinned binding missing or raw result leaked", raw)
	}
	stored, e := store.Job(ctx, old.ID)
	if e != nil || stored.Debate.Responses[0].Raw != old.Debate.Responses[0].Raw {
		t.Fatal("read rewrote original judgement", e)
	}
	records, e := store.Records(ctx, "snapshot", snap.ID)
	if e != nil || len(records) != 1 {
		t.Fatal("read altered snapshot records", e)
	}
}

func TestGuanlanRiskSummaryExcludesPrivateHoldingsAndKeepsMethod(t *testing.T) {
	job := portfolioinspection.Job{ID: "risk-fixture", Status: "succeeded", Request: portfolioinspection.Request{PortfolioPlanID: "plan-fixture"}, Report: &portfolioinspection.Report{ID: "report-fixture", PromptVersion: "frozen-method", Model: "recorded-model", Conclusion: portfolioinspection.AIReport{RiskLevel: "fixture", ExecutiveSummary: "saved summary"}, Facts: map[string]portfolioinspection.Fact{"private": {}}}}
	value := summarizeRiskJob(job)
	raw, e := json.Marshal(value)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "\"facts\"") || strings.Contains(string(raw), "\"holdings\"") || !strings.Contains(string(raw), "frozen-method") || !strings.Contains(string(raw), "recorded-model") {
		t.Fatal("summary exposed full report or lost provenance", string(raw))
	}
	if _, ok := value["coverage_percent"]; ok {
		t.Fatal("unknown coverage filled with zero")
	}
}

func TestGuanlanMetadataCrossOriginPATCHPreflight(t *testing.T) {
	s := &Server{token: "fixture-token"}
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/boards/versions/fixture", nil)
	r.Header.Set("Origin", "http://127.0.0.1:20075")
	r.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	w := httptest.NewRecorder()
	s.withCORS(w, r)
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), http.MethodPatch) {
		t.Fatal("metadata PATCH rejected by browser preflight")
	}
	mutation := httptest.NewRequest(http.MethodPatch, r.URL.String(), nil)
	if s.authorized(mutation) {
		t.Fatal("CORS must not bypass mutation authentication")
	}
	mutation.Header.Set("X-A-Stock-Token", "fixture-token")
	if !s.authorized(mutation) {
		t.Fatal("authorized metadata request rejected")
	}
}

func TestGuanlanRiskEvidenceTimesUseOriginalCutoffs(t *testing.T) {
	at := time.Date(2026, 10, 13, 7, 0, 0, 0, time.UTC)
	job := portfolioinspection.Job{Report: &portfolioinspection.Report{GeneratedAt: at.Add(24 * time.Hour), Holdings: []portfolioinspection.HoldingResult{{Holding: portfolioinspection.Holding{Symbol: "TEST"}, ResearchCutoffAt: at}, {Holding: portfolioinspection.Holding{Symbol: "MISSING"}}}}}
	value := summarizeRiskJob(job)
	raw, _ := json.Marshal(value)
	if !strings.Contains(string(raw), "TEST 证据截至") || !strings.Contains(string(raw), "2026-10-13T07:00:00Z") || !strings.Contains(string(raw), "0001-01-01T00:00:00Z") {
		t.Fatal("original/unknown evidence timestamps lost", string(raw))
	}
	if job.Report.Holdings[1].ResearchCutoffAt != (time.Time{}) {
		t.Fatal("read backfilled unknown evidence")
	}
}
