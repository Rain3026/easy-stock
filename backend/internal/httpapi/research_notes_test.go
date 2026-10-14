package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestNotesRetainParentBeyondFirstPageAndRejectOtherEntity(t *testing.T) {
	s := researchTestServer(t, nil)
	ctx := context.Background()
	old := researchNote{ID: "old", EntityID: "stock:600176.SH", Text: "original"}
	if e := s.dailyResearch.store.Append(ctx, old.ID, "note", old.EntityID, old); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 105; i++ {
		v := old
		v.ID = fmt.Sprint(i)
		s.dailyResearch.store.Append(ctx, v.ID, "note", v.EntityID, v)
	}
	w := researchRequest(s, "POST", "/api/v1/research/notes", `{"entity_id":"stock:600176.SH","parent_id":"old","text":"revised"}`)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	raw, _ := s.dailyResearch.store.Record(ctx, "note", "old")
	var prior researchNote
	json.Unmarshal(raw, &prior)
	if prior.Text != "original" {
		t.Fatal("original overwritten")
	}
	w = researchRequest(s, "POST", "/api/v1/research/notes", `{"entity_id":"stock:600000.SH","parent_id":"old","text":"wrong entity"}`)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
