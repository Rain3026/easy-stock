package datahub

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeDoesNotTreatTransportSuccessAsHealthy(t *testing.T) {
	cases := []struct {
		name  string
		value Observation
		err   error
		want  string
	}{
		{"empty", Observation{}, nil, "empty"},
		{"old", Observation{Count: 1, Stale: true, Freshness: "verified"}, nil, "stale"},
		{"unknown time", Observation{Count: 1}, nil, "partial"},
		{"truncated", Observation{Count: 1, Freshness: "verified", Truncated: true}, nil, "partial"},
		{"permission", Observation{}, errors.New("403 token TOP_SECRET"), "permission_denied"},
		{"rate", Observation{}, errors.New("429"), "rate_limited"},
		{"timeout", Observation{}, context.DeadlineExceeded, "timeout"},
		{"configured", Observation{}, errors.New("请在系统设置配置 Token"), "unconfigured"},
		{"valid", Observation{Count: 2, Freshness: "verified"}, nil, "available"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, msg := classify(c.value, c.err)
			if got != c.want || strings.Contains(msg, "TOP_SECRET") {
				t.Fatalf("got %s %s", got, msg)
			}
		})
	}
}
func TestMonitorReadOnlySnapshotDeduplicatesAndExpires(t *testing.T) {
	var calls atomic.Int32
	gate := make(chan struct{})
	path := filepath.Join(t.TempDir(), "health.json")
	m := NewMonitor([]Probe{{SourceID: "sina", Capability: "quote", Run: func(ctx context.Context) (Observation, error) {
		calls.Add(1)
		select {
		case <-gate:
		case <-ctx.Done():
			return Observation{}, ctx.Err()
		}
		return Observation{Count: 1, Freshness: "verified"}, nil
	}}}, path)
	initial, _ := m.Snapshot()
	for _, h := range initial {
		if h.OK || h.CheckedAt != nil {
			t.Fatal("unprobed source is green")
		}
	}
	if calls.Load() != 0 || !m.Refresh() || m.Refresh() {
		t.Fatal("probe read/dedup failure")
	}
	close(gate)
	for i := 0; i < 100; i++ {
		_, busy := m.Snapshot()
		if !busy {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	m.Close()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	restored := NewMonitor(nil, path)
	defer restored.Close()
	loaded, _ := restored.Snapshot()
	for _, h := range loaded {
		if h.ID == "sina" && (!h.OK || h.LastSuccessAt == nil) {
			t.Fatal(h)
		}
	}
	restored.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	loaded, _ = restored.Snapshot()
	for _, h := range loaded {
		if h.ID == "sina" && (h.OK || h.Status != "expired") {
			t.Fatal(h)
		}
	}
}
func TestSourceFreshnessUsesSessionAndLunchtime(t *testing.T) {
	parse := func(s string) time.Time {
		v, e := time.ParseInLocation("2006-01-02 15:04", s, foundation.AStockLocation)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	for _, c := range []struct {
		at, now string
		stale   bool
	}{
		{"2026-10-09 15:00", "2026-10-11 12:00", false},
		{"2026-10-13 11:30", "2026-10-13 12:20", false},
		{"2026-10-13 10:00", "2026-10-13 10:10", true},
		{"2026-10-12 15:00", "2026-10-13 09:40", true},
		{"2026-10-13 15:00", "2026-10-13 21:00", false},
		{"2026-10-14 15:00", "2026-10-13 21:00", true},
	} {
		o := SessionObservation(1, parse(c.at), parse(c.now), true)
		if o.Stale != c.stale {
			t.Fatalf("%+v: %+v", c, o)
		}
	}
}
func TestExplicitUnitsAndIndependentBackups(t *testing.T) {
	v, e := AmountCNY(1800, "CNY_thousand")
	if e != nil || v != 1800000 {
		t.Fatal(v, e)
	}
	if _, e = AmountCNY(1800, ""); e == nil {
		t.Fatal("unknown unit accepted")
	}
	p := current("eastmoney")
	same := p
	same.Channel = "mcp"
	if CanSubstitute(p, same) {
		t.Fatal("same source counted as independent")
	}
	if CanSubstitute(p, degraded("tushare", "daily")) || CanSubstitute(p, pending("sina", "unverified")) {
		t.Fatal("non-equivalent backup enabled")
	}
	if !CanSubstitute(p, current("sina")) {
		t.Fatal("independent implemented route not usable")
	}
}
