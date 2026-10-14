package httpapi

import (
	"context"
	"easy-stock/backend/internal/agent"
	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/foundation"
	"easy-stock/backend/internal/review"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func researchTestServer(t *testing.T, gateway agent.Gateway) *Server {
	t.Helper()
	s := NewServer(Config{MarketOverview: &fakeMarketOverviewProvider{}, ThemeOverview: fakeThemeOverviewProvider{}, SectorMap: fakePagedSectorMapProvider{}, KLinePrimary: fakeKLineProvider{}, AgentGateway: gateway})
	// Research/settings tests must not depend on real market endpoints or a
	// network response arriving inside the overview's 150ms fast path.
	s.dailyOverview.client.Token = func() string { return "" }
	s.dailyOverview.client.Fallback = nil
	s.dailyOverview.client.SecondaryFallback = nil
	s.dailyOverview.client.BreadthFallback = nil
	s.dailyOverview.client.BreadthSecondary = nil
	s.limitUpSnapshots = nil
	t.Cleanup(func() { s.Close() })
	return s
}
func researchRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func waitResearchJob(t *testing.T, s *Server, w *httptest.ResponseRecorder) dr.Job {
	t.Helper()
	if w.Code != 202 {
		t.Fatalf("start %d %s", w.Code, w.Body.String())
	}
	var p struct {
		Data dr.Job `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &p)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		j, err := s.dailyResearch.store.Job(context.Background(), p.Data.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status != "running" {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return dr.Job{}
}
func TestDailyResearchRetiresLegacyWrites(t *testing.T) {
	s := researchTestServer(t, nil)
	ctx := context.Background()
	if _, err := s.reviewStore.UpsertPost(ctx, review.Post{ID: "kept", ExternalID: "kept", Source: "official", Title: "archive", ContentText: "original"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/reviews/sync", "/api/v1/reviews/daily-summary", "/api/v1/reviews/remote-daily/sync", "/api/v1/reviews/daily-summary/schedule"} {
		w := researchRequest(s, "POST", path, `{}`)
		if w.Code != 410 {
			t.Fatalf("legacy write %s %d", path, w.Code)
		}
	}
	w := researchRequest(s, "GET", "/api/v1/reviews/posts", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "archive") {
		t.Fatal("historical data lost")
	}
	s.RunReviewScheduler(ctx)
	s.RunRemoteDailyReviewScheduler(ctx)
	s.RunDailySummaryScheduler(ctx)
}
func TestDailyResearchFreezesFullPoolAndUsesNewPortfolioSource(t *testing.T) {
	g := &fakeAgentGateway{}
	s := researchTestServer(t, g)
	j := waitResearchJob(t, s, researchRequest(s, "POST", "/api/v1/daily-research", `{}`))
	if j.Status != "succeeded" {
		t.Fatal(j.Error)
	}
	if len(g.prompts) != 0 {
		t.Fatal("fact collection called model")
	}
	v, err := s.dailyResearch.store.Snapshot(context.Background(), j.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.sectorMap.Build(context.Background(), v.Themes[0].Overview.Theme)
	ranked := rankThemeCandidates(flattenThemeCandidates(m))
	sortThemeCandidates(ranked, "rank_score")
	if len(v.Themes[0].Members) != len(ranked) || len(v.Themes[0].Candidates) != 3 {
		t.Fatalf("pool truncated: %d / %d", len(v.Themes[0].Members), len(ranked))
	}
	for i, c := range v.Themes[0].Candidates {
		if c.Symbol != ranked[i].Stock.Symbol {
			t.Fatal("ranking drift")
		}
	}
	summary, err := (researchSummarySource{s.dailyResearch.store}).GetDailySummary(context.Background(), v.TradeDate)
	if err != nil || !strings.Contains(summary.ExecutiveSummary, v.ID) {
		t.Fatal("portfolio source not migrated", err)
	}
	newer := v
	newer.ID = "same-date-newer"
	if err := s.dailyResearch.store.Append(context.Background(), newer.ID, "snapshot", newer.ID, newer); err != nil {
		t.Fatal(err)
	}
	exact, err := (researchSummarySource{s.dailyResearch.store}).GetResearchSummary(context.Background(), v.ID)
	if err != nil || !strings.Contains(exact.ExecutiveSummary, v.ID) {
		t.Fatal("selected portfolio snapshot moved to a later version")
	}
}

func TestDailyResearchCancellationDoesNotArchivePartialVerdict(t *testing.T) {
	started := make(chan struct{})
	g := &fakeAgentGateway{promptFunc: func(ctx context.Context, _ string) (agent.PromptResult, error) {
		close(started)
		<-ctx.Done()
		return agent.PromptResult{}, ctx.Err()
	}}
	s := researchTestServer(t, g)
	v := dr.Snapshot{ID: "cancel-snapshot", TradeDate: "2026-10-09"}
	if err := s.dailyResearch.store.Append(context.Background(), v.ID, "snapshot", v.ID, v); err != nil {
		t.Fatal(err)
	}
	w := researchRequest(s, "POST", "/api/v1/daily-research/cancel-snapshot/debate", `{"subject":"market"}`)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("model stage never started")
	}
	if conflict := researchRequest(s, "POST", "/api/v1/daily-research", ""); conflict.Code != 409 {
		t.Fatal("concurrent duplicate accepted")
	}
	var p struct {
		Data dr.Job `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &p)
	if cancelled := researchRequest(s, "POST", "/api/v1/daily-research/jobs/"+p.Data.ID+"/cancel", ""); cancelled.Code != 200 {
		t.Fatal(cancelled.Code)
	}
	j := waitResearchJob(t, s, w)
	if j.Status != "cancelled" {
		t.Fatalf("status %s", j.Status)
	}
	records, _ := s.dailyResearch.store.Records(context.Background(), "debate", v.ID)
	if len(records) != 0 {
		t.Fatal("partial verdict marked complete")
	}
}
func TestDailyResearchDebateIndependentEvidenceAndInvalidReply(t *testing.T) {
	g := &fakeAgentGateway{promptResult: agent.PromptResult{Runtime: "fixture", Content: `{"thesis":"fixture thesis","evidence":["market"],"risks":["risk"],"conditions":["check close"],"uncertainty":"limited evidence"}`}}
	s := researchTestServer(t, g)
	v := dr.Snapshot{ID: "frozen", TradeDate: "2026-10-09", Version: dr.Version, Indexes: []foundation.MarketIndexSnapshot{{Name: "fixture", Price: 100}}}
	s.dailyResearch.store.Append(context.Background(), v.ID, "snapshot", v.ID, v)
	j := waitResearchJob(t, s, researchRequest(s, "POST", "/api/v1/daily-research/frozen/debate", `{"subject":"market"}`))
	if j.Status != "succeeded" {
		t.Fatal(j.Error)
	}
	if len(g.prompts) != 5 || strings.Contains(g.prompts[1], "fixture thesis") || !strings.Contains(g.prompts[2], "fixture thesis") {
		t.Fatal("independent/rebuttal sequence violated")
	}
	for _, o := range g.promptOptions {
		if !o.DisableTools || o.MaxAttempts != 1 || !o.Sandbox {
			t.Fatal("unbounded role options")
		}
	}
	records, _ := s.dailyResearch.store.Records(context.Background(), "debate", v.ID)
	if len(records) != 1 {
		t.Fatal("debate not archived")
	}
	reused := researchRequest(s, "POST", "/api/v1/daily-research/frozen/debate", `{"subject":"market"}`)
	if reused.Code != 200 || len(g.prompts) != 5 {
		t.Fatal("identical research was not reused")
	}
	g.promptResult.Content = `{"thesis":"invented","evidence":["future_fact"],"conditions":["later"],"uncertainty":"unknown"}`
	j = waitResearchJob(t, s, researchRequest(s, "POST", "/api/v1/daily-research/frozen/debate", `{"subject":"market","force":true}`))
	if j.Status != "failed" || j.Debate.Calls != 1 {
		t.Fatal("invalid evidence accepted")
	}
	records, _ = s.dailyResearch.store.Records(context.Background(), "debate", v.ID)
	if len(records) != 1 {
		t.Fatal("failure overwrote a judgement")
	}
}
func TestDailyResearchRejectsUnsupportedConfigAndUnknownSubjects(t *testing.T) {
	s := researchTestServer(t, nil)
	for _, body := range []string{`{"rounds":2,"timeout_seconds":180,"max_calls":3}`, `{"rounds":1,"timeout_seconds":180,"max_calls":5,"bull":{"runtime":"invented"}}`, `{"rounds":1,"timeout_seconds":180,"max_calls":5,"bear":{"profile_id":"missing"}}`} {
		if w := researchRequest(s, "PUT", "/api/v1/daily-research/settings", body); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}

func TestDailyOverviewSettingAndFrozenPulse(t *testing.T) {
	s := researchTestServer(t, nil)
	for _, body := range []string{`{"observation_line":0}`, `{"observation_line":-1}`, `{"observation_line":1000001}`} {
		if w := researchRequest(s, "PUT", "/api/v1/daily-research/overview/settings", body); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if w := researchRequest(s, "PUT", "/api/v1/daily-research/overview/settings", `{"observation_line":20000}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	job := waitResearchJob(t, s, researchRequest(s, "POST", "/api/v1/daily-research", `{}`))
	if job.Status != "succeeded" {
		t.Fatal(job.Error)
	}
	v, e := s.dailyResearch.store.Snapshot(context.Background(), job.SnapshotID)
	if e != nil || v.MarketPulse == nil || v.MarketPulse.ObservationLine != 20000 {
		t.Fatal(v.MarketPulse, e)
	}
	researchRequest(s, "PUT", "/api/v1/daily-research/overview/settings", `{"observation_line":18100}`)
	frozen, _ := s.dailyResearch.store.Snapshot(context.Background(), job.SnapshotID)
	if frozen.MarketPulse.ObservationLine != 20000 {
		t.Fatal("live setting changed archived observation")
	}
	w := researchRequest(s, "GET", "/api/v1/daily-research/overview", "")
	if w.Code == 202 {
		// A cold overview may legitimately return accepted while collecting.
		select {
		case <-s.dailyOverview.overview.done:
			w = researchRequest(s, "GET", "/api/v1/daily-research/overview", "")
		case <-time.After(2 * time.Second):
			t.Fatal("fixture overview did not finish")
		}
	}
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"observation_line":18100`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
