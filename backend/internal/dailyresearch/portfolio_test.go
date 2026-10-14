package dailyresearch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestPortfolioMigrationCASAndRollbackAppend(t *testing.T) {
	s, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	a := json.RawMessage(`{"plans":[{"id":"original"}]}`)
	one, e := s.SavePortfolio(ctx, a, 0, "browser-import")
	if e != nil || one.Revision != 1 {
		t.Fatal(one, e)
	}
	same, e := s.SavePortfolio(ctx, a, 0, "browser-import")
	if e != nil || same.Revision != 1 {
		t.Fatal("import not idempotent")
	}
	b := json.RawMessage(`{"plans":[{"id":"modified"}]}`)
	two, e := s.SavePortfolio(ctx, b, 1, "edit")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SavePortfolio(ctx, a, 1, "edit"); !errors.Is(e, ErrPlanConflict) {
		t.Fatal("stale writer overwrote data", e)
	}
	three, e := s.SavePortfolio(ctx, a, two.Revision, "restore")
	if e != nil || three.Revision != 3 {
		t.Fatal(e)
	}
	history, e := s.PortfolioHistory(ctx)
	if e != nil || len(history) != 3 || history[2].Hash != one.Hash {
		t.Fatal("original lost")
	}
	if _, e = s.db.Exec(`UPDATE portfolio_plan_versions SET origin='bad'`); e == nil {
		t.Fatal("mutable versions")
	}
}
