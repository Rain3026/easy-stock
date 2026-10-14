package dailyresearch

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotsAreAppendOnlyAndJobsRecover(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "journal.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	v := Snapshot{ID: "first", TradeDate: "2026-10-09", Version: Version}
	if err = s.Append(ctx, v.ID, "snapshot", v.ID, v); err != nil {
		t.Fatal(err)
	}
	v.Status = "changed"
	if s.Append(ctx, v.ID, "snapshot", v.ID, v) == nil {
		t.Fatal("snapshot overwrite permitted")
	}
	v.ID = "second"
	if err = s.Append(ctx, v.ID, "snapshot", v.ID, v); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveJob(ctx, Job{ID: "inflight", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.Snapshot(ctx, "first")
	if err != nil || first.Status != "" {
		t.Fatalf("original modified: %+v %v", first, err)
	}
	rows, _ := s.Records(ctx, "snapshot", "")
	if len(rows) != 2 {
		t.Fatal("same-date version lost")
	}
	j, _ := s.Job(ctx, "inflight")
	if j.Status != "interrupted" {
		t.Fatal(j)
	}
	before, _ := json.Marshal(first)
	if err = s.Append(ctx, "verification", "verification", "first", Verification{SnapshotID: "first"}); err != nil {
		t.Fatal(err)
	}
	first, _ = s.Snapshot(ctx, "first")
	after, _ := json.Marshal(first)
	if string(before) != string(after) {
		t.Fatal("verification changed snapshot")
	}
}
func TestFixedExchangeWindowsAndRevisions(t *testing.T) {
	day := func(date string) time.Time {
		v, _ := time.ParseInLocation("2006-01-02", date, foundation.AStockLocation)
		return v
	}
	bar := func(date string, close float64) foundation.KLine {
		return foundation.KLine{Time: day(date), Close: close, Meta: foundation.SourceMeta{Source: "fixture:qfq"}}
	}
	base := bar("2026-09-30", 10)
	s := Snapshot{ID: "original", TradeDate: "2026-09-30", Baselines: map[string]foundation.KLine{"sh600000": base}, Themes: []Theme{{Candidates: []foundation.BoardStock{{Symbol: "sh600000", Name: "fixture"}}}}}
	lines := []foundation.KLine{base, bar("2026-10-08", 11), bar("2026-10-12", 12)}
	load := func(context.Context, string) ([]foundation.KLine, error) { return lines, nil }
	v := Verify(context.Background(), s, day("2026-10-12").Add(16*time.Hour), load)
	if v.Outcomes[0].DueDate != "2026-10-08" || v.Outcomes[1].DueDate != "2026-10-12" || v.Outcomes[2].Status != "pending" {
		t.Fatalf("windows: %+v", v.Outcomes)
	}
	if v.Outcomes[0].Status != "observed" || *v.Outcomes[0].ChangePercent < 9.99 {
		t.Fatal(v.Outcomes[0])
	}
	lines = []foundation.KLine{base, bar("2026-10-09", 11)}
	v = Verify(context.Background(), s, day("2026-10-12").Add(16*time.Hour), load)
	if v.Outcomes[0].Status != "unavailable" {
		t.Fatal("missing day incorrectly rolled forward")
	}
	lines = []foundation.KLine{bar("2026-09-30", 9), bar("2026-10-08", 11)}
	v = Verify(context.Background(), s, day("2026-10-12").Add(16*time.Hour), load)
	if v.Outcomes[0].Status != "revised" {
		t.Fatal("adjustment revision accepted")
	}
	v = Verify(context.Background(), s, day("2026-10-08").Add(14*time.Hour), load)
	if v.Outcomes[0].Status != "pending" {
		t.Fatal("unfinished bar counted")
	}
}

func TestStockContextVerifiesWithoutThemeMembership(t *testing.T) {
	day, _ := time.ParseInLocation("2006-01-02", "2026-10-12", foundation.AStockLocation)
	base := foundation.KLine{Time: day, Close: 10, Meta: foundation.SourceMeta{Source: "fixture"}}
	end := base
	end.Time = day.AddDate(0, 0, 1)
	end.Close = 11
	s := Snapshot{ID: "stock-context", EntityID: "stock:600176.SH", TradeDate: "2026-10-12", Baselines: map[string]foundation.KLine{"600176.SH": base}}
	v := Verify(context.Background(), s, end.Time.Add(16*time.Hour), func(context.Context, string) ([]foundation.KLine, error) { return []foundation.KLine{base, end}, nil })
	if len(v.Outcomes) != 3 || v.Outcomes[0].Status != "observed" || v.Outcomes[1].Status != "pending" {
		t.Fatalf("context not verified: %+v", v)
	}
}
