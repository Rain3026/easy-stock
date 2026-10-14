package httpapi

import (
	"context"
	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/datahub"
	"encoding/json"
	"testing"
	"time"
)

func TestResearchContextKeepsEntityAndParentEvidence(t *testing.T) {
	s := researchTestServer(t, nil)
	ctx := context.Background()
	now := time.Now().Add(-time.Hour)
	a, e := s.dailyResearch.store.Facts().Freeze(ctx, datahub.FactSnapshot{EntityID: "stock:600176.SH", Kind: "tool-quote", MethodVersion: "v1", CapturedAt: now, CutoffAt: now, Payload: json.RawMessage(`{"price":12}`)})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.dailyResearch.store.Facts().Freeze(ctx, datahub.FactSnapshot{EntityID: "stock:600000.SH", Kind: "tool-quote", MethodVersion: "v1", CapturedAt: now, CutoffAt: now, Payload: json.RawMessage(`{"price":15}`)})
	if e != nil {
		t.Fatal(e)
	}
	p := researchRequest(s, "POST", "/api/v1/research/contexts", `{"kind":"evidence","evidence_ids":["`+a.ID+`","`+b.ID+`"]}`)
	if p.Code != 400 {
		t.Fatal("mixed entity accepted", p.Code)
	}
	p = researchRequest(s, "POST", "/api/v1/research/contexts", `{"kind":"evidence","evidence_ids":["`+a.ID+`"]}`)
	if p.Code != 201 {
		t.Fatal(p.Code, p.Body.String())
	}
	var out struct {
		Data dr.Snapshot `json:"data"`
	}
	json.Unmarshal(p.Body.Bytes(), &out)
	if !researchSubject(out.Data, a.EntityID) || researchSubject(out.Data, b.EntityID) || researchSubject(out.Data, "market") {
		t.Fatal("subject escaped context")
	}
	p = researchRequest(s, "POST", "/api/v1/research/contexts", `{"kind":"evidence","evidence_ids":["`+a.ID+`"],"parent_id":"`+out.Data.ID+`"}`)
	if p.Code != 201 {
		t.Fatal(p.Body.String())
	}
	var child struct {
		Data dr.Snapshot `json:"data"`
	}
	json.Unmarshal(p.Body.Bytes(), &child)
	if child.Data.ID == out.Data.ID || child.Data.ParentID != out.Data.ID {
		t.Fatal("supplement overwrote parent")
	}
	prior, _ := s.dailyResearch.store.Snapshot(ctx, out.Data.ID)
	if prior.ParentID != "" || !prior.Cutoff.Equal(now) {
		t.Fatal("original context changed")
	}
}
func TestSkillEvaluationContractDetectsFalseCitation(t *testing.T) {
	m, _ := contractMetrics(`{"thesis":"x","evidence":["invented"],"conditions":["next close"],"risks":["counter"],"uncertainty":"missing"}`, "fact-1", 10, 100, nil)
	if !m.Structure || m.Citations || !m.Counter || !m.Missing {
		t.Fatal(m)
	}
}
