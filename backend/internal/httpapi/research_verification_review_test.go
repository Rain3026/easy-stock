package httpapi

import (
	"context"
	"crypto/sha256"
	dr "easy-stock/backend/internal/dailyresearch"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestVerificationReviewWaitsForObservedEvidence(t *testing.T) {
	g := &fakeAgentGateway{}
	s := researchTestServer(t, g)
	ctx := context.Background()
	snapshot := dr.Snapshot{ID: "frozen-review", TradeDate: "2026-10-13", Cutoff: time.Now()}
	if e := s.dailyResearch.store.Append(ctx, snapshot.ID, "snapshot", "", snapshot); e != nil {
		t.Fatal(e)
	}
	for _, outcomes := range [][]dr.Outcome{nil, {{Status: "pending"}}, {{Status: "revised"}}} {
		if outcomes != nil {
			if e := s.dailyResearch.store.Append(ctx, dr.NewID(), "verification", snapshot.ID, dr.Verification{SnapshotID: snapshot.ID, CheckedAt: time.Now(), Outcomes: outcomes}); e != nil {
				t.Fatal(e)
			}
		}
		w := researchRequest(s, "POST", "/api/v1/daily-research/"+snapshot.ID+"/explain", `{}`)
		if w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(g.prompts) != 0 {
		t.Fatal("model called without observed evidence")
	}
	original, e := s.dailyResearch.store.Snapshot(ctx, snapshot.ID)
	if e != nil || !original.Cutoff.Equal(snapshot.Cutoff) {
		t.Fatal("original judgement changed", e)
	}
}
func TestTrainingExcerptPreservesOriginalIdentity(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"facts": []string{strings.Repeat("资料", 70000)}, "price": 12.34})
	before := string(raw)
	got := skillTrainingExcerpt(raw)
	if string(raw) != before || len(got) > 64<<10 {
		t.Fatal("mutated original or exceeded bound")
	}
	var v map[string]any
	if e := json.Unmarshal(got, &v); e != nil {
		t.Fatal(e)
	}
	if v["original_sha256"] != fmt.Sprintf("%x", sha256.Sum256(raw)) || v["omitted"] != true {
		t.Fatal("excerpt lacks original identity")
	}
	if v["excerpt"].(map[string]any)["price"] != 12.34 {
		t.Fatal("changed numeric fact")
	}
}
