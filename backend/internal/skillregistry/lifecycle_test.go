package skillregistry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestLifecyclePublicationIsolationAndRollback(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	s, e := Open(db)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	b, e := s.Register(ctx, Bundle{Name: "research", Files: map[string][]byte{"SKILL.md": []byte("Original research method with source checks."), "reference.txt": []byte("fixed")}})
	if e != nil {
		t.Fatal(e)
	}
	entity := ""
	for i := 0; ; i++ {
		entity = fmt.Sprint("stock:", i)
		if EntitySplit(entity) == "training" {
			break
		}
	}
	f, e := s.AddFeedback(ctx, Feedback{Name: b.Name, Hash: b.Hash, JobID: "old-job", EvidenceID: "old-fact", EntityID: entity, Cutoff: time.Now().Add(-72 * time.Hour), Category: "method", Note: "Missing explicit contrary evidence"})
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.CreateCandidate(ctx, b.Name, b.Hash, "Candidate research method with source and contrary checks.", "Add explicit contrary evidence", []string{f.ID})
	if e != nil {
		t.Fatal(e)
	}
	active, _ := s.Resolve(ctx, b)
	if active.Hash != b.Hash {
		t.Fatal("candidate leaked into active task")
	}
	evaluation, e := s.SaveEvaluation(ctx, c, Evaluation{JobID: "evaluation"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Release(ctx, c.ID, evaluation.ID, b.Hash); e == nil {
		t.Fatal("insufficient samples published")
	}
	m := Metrics{Structure: true, Citations: true, Missing: true, Counter: true, DurationMS: 1000, Tokens: 100}
	cases := []EvaluationCase{}
	for i := 0; len(cases) < 5; i++ {
		entity := fmt.Sprint("stock:", i)
		if EntitySplit(entity) != "holdout" {
			continue
		}
		cases = append(cases, EvaluationCase{EntityID: entity, EvidenceID: entity, FeedbackID: entity, Cutoff: time.Now().Add(-24 * time.Hour), Base: m, Candidate: m})
	}
	evaluation, e = s.SaveEvaluation(ctx, c, Evaluation{JobID: "real-shadow-fixture", Cases: cases})
	if e != nil || !evaluation.Passed {
		t.Fatal(e, evaluation.Reasons)
	}
	release, e := s.Release(ctx, c.ID, evaluation.ID, b.Hash)
	if e != nil {
		t.Fatal(e)
	}
	active, e = s.Resolve(ctx, b)
	if e != nil || active.Hash != c.Hash {
		t.Fatal(e, "new tasks not using candidate")
	}
	old, e := s.Get(ctx, b.Name, b.Hash)
	if e != nil || string(old.Files["SKILL.md"]) != "Original research method with source checks." {
		t.Fatal("running task version overwritten")
	}
	if _, e = s.Rollback(ctx, release.ID, "wrong-active-hash"); e == nil {
		t.Fatal("concurrent update overwritten")
	}
	if _, e = s.Rollback(ctx, release.ID, c.Hash); e != nil {
		t.Fatal(e)
	}
	active, _ = s.Resolve(ctx, b)
	if active.Hash != b.Hash {
		t.Fatal("rollback failed")
	}
	for _, table := range []string{"skill_versions", "skill_events"} {
		if _, e = db.Exec("DELETE FROM " + table); e == nil {
			t.Fatal("immutable history deleted", table)
		}
	}
	holdout := Feedback{Name: b.Name, Hash: b.Hash, JobID: "held", EvidenceID: "held", EntityID: cases[0].EntityID, Cutoff: time.Now().Add(-time.Hour), Category: "model", Note: "reserved sample"}
	holdout, e = s.AddFeedback(ctx, holdout)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateCandidate(ctx, b.Name, b.Hash, "New text with enough characters for test candidate.", "Do not use holdout", []string{holdout.ID}); e == nil {
		t.Fatal("holdout leaked into training")
	}
	cases[0].Candidate.Citations = false
	if ok, _ := Assess(c, cases); ok {
		t.Fatal("invalid citations passed")
	}
	cases[0].Candidate = m
	cases[0].Cutoff = time.Now().Add(time.Hour)
	if ok, _ := Assess(c, cases); ok {
		t.Fatal("future evidence passed")
	}
	rows, _ := s.Events(ctx, "release")
	var restored Release
	_ = json.Unmarshal(rows[0], &restored)
	if restored.Kind != "rollback" {
		t.Fatal("rollback history missing")
	}
}
