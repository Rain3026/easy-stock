package httpapi

import (
	"context"
	"easy-stock/backend/internal/agent"
	dr "easy-stock/backend/internal/dailyresearch"
	"encoding/json"
	"errors"
	"testing"
)

func TestDebateRetryKeepsCompletedRolesAndOriginal(t *testing.T) {
	calls := 0
	good := `{"thesis":"test","evidence":["market"],"conditions":["verify next close"],"risks":["data"],"uncertainty":"limited"}`
	g := &fakeAgentGateway{promptFunc: func(context.Context, string) (agent.PromptResult, error) {
		calls++
		if calls == 2 {
			return agent.PromptResult{}, errors.New("role unavailable")
		}
		return agent.PromptResult{Content: good}, nil
	}}
	s := researchTestServer(t, g)
	v := dr.Snapshot{ID: "resume-facts", TradeDate: "2026-10-09"}
	s.dailyResearch.store.Append(context.Background(), v.ID, "snapshot", v.ID, v)
	first := waitResearchJob(t, s, researchRequest(s, "POST", "/api/v1/daily-research/resume-facts/debate", `{"subject":"market"}`))
	if first.Status != "failed" || len(first.Debate.Responses) != 1 {
		t.Fatal(first)
	}
	next := waitResearchJob(t, s, researchRequest(s, "POST", "/api/v1/daily-research/jobs/"+first.ID+"/retry", `{}`))
	if next.Status != "succeeded" || calls != 6 || len(next.Debate.Responses) != 5 || next.PreviousID != first.ID {
		t.Fatal(next.Status, next.Error, calls)
	}
	prior, _ := s.dailyResearch.store.Job(context.Background(), first.ID)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(prior)
	if string(a) != string(b) {
		t.Fatal("retry overwrote original attempt")
	}
}

func TestDebateAllowsFrozenFactBundleCitation(t *testing.T) {
	g := &fakeAgentGateway{promptFunc: func(context.Context, string) (agent.PromptResult, error) {
		return agent.PromptResult{Content: `{"thesis":"observed","evidence":["fact-real"],"conditions":["verify next close"],"risks":["gap"],"uncertainty":"limited"}`}, nil
	}}
	s := researchTestServer(t, g)
	v := dr.Snapshot{ID: "facts", EvidenceID: "fact-real", TradeDate: "2026-10-13"}
	s.dailyResearch.store.Append(context.Background(), v.ID, "snapshot", v.ID, v)
	j := waitResearchJob(t, s, researchRequest(s, "POST", "/api/v1/daily-research/facts/debate", `{"subject":"market"}`))
	if j.Status != "succeeded" {
		t.Fatal(j.Error)
	}
}
