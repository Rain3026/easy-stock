package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTurnoverUnitsAndIntersection(t *testing.T) {
	a := []Row{{"trade_date": "20261012", "amount": 714483421.1574}, {"trade_date": "20261009", "amount": 1.0}}
	b := []Row{{"trade_date": "20261012", "amount": 822865988.22233}}
	got := mergeHistory(a, b)
	if len(got) != 1 || got[0].Amount < 15373.49 || got[0].Amount > 15373.50 {
		t.Fatalf("thousand yuan to yi, intersection only: %+v", got)
	}
}
func TestTargetAndHistoryExcludeFocus(t *testing.T) {
	for _, tc := range []struct{ now, date, session string }{{"2026-10-13 06:30:00", "2026-10-12", "closed"}, {"2026-10-13 11:45:00", "2026-10-13", "lunch"}, {"2026-10-13 10:30:00", "2026-10-13", "intraday"}, {"2026-10-04 10:30:00", "2026-09-30", "closed"}} {
		d, s := Target(parseTime(tc.now))
		if d.Format("2006-01-02") != tc.date || s != tc.session {
			t.Fatal(tc, d, s)
		}
	}
	target := parseTime("2026-10-13 11:00:00")
	days := []Day{}
	d := target
	for len(days) < 20 {
		d = d.AddDate(0, 0, -1)
		if foundation.IsAStockTradingDay(d) {
			days = append([]Day{{d.Format("2006-01-02"), 100}}, days...)
		}
	}
	if !completeHistory(days, target) {
		t.Fatal("valid previous 20 rejected")
	}
	days[0].Date = "2026-01-01"
	if completeHistory(days, target) {
		t.Fatal("calendar gap admitted")
	}
}
func TestRealtimeMustHaveBothMarketsSameDayAndFresh(t *testing.T) {
	now := parseTime("2026-10-13 11:00:00")
	rows := []Row{{"ts_code": "000002.SH", "amount": 1e12, "trade_time": "2026-10-13 11:00:00"}, {"ts_code": "399107.SZ", "amount": 1e12, "trade_time": "2026-10-13 11:00:00"}}
	a, _, ok := realtimeAmount(rows, "2026-10-13", now, "intraday")
	if !ok || a != 20000 {
		t.Fatal(a, ok)
	}
	rows[1]["trade_time"] = "2026-10-12 15:00:00"
	if _, _, ok = realtimeAmount(rows, "2026-10-13", now, "intraday"); ok {
		t.Fatal("cross-date accepted")
	}
	if _, _, ok = realtimeAmount(rows[:1], "2026-10-13", now, "intraday"); ok {
		t.Fatal("one market accepted")
	}
	if !fresh(parseTime("2026-10-13 11:30:00"), parseTime("2026-10-13 12:30:00"), "lunch") {
		t.Fatal("lunch should retain 11:30 observation")
	}
}
func fixtureClient(t *testing.T, fn func(string, map[string]any) ([]string, [][]any)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			API    string         `json:"api_name"`
			Params map[string]any `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		f, rows := fn(req.API, req.Params)
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"fields": f, "items": rows}})
	}))
	t.Cleanup(srv.Close)
	c := New(func() string { return "secret" }, t.TempDir())
	c.URL = srv.URL
	return c
}
func TestSameTimeForecastAndMinuteGaps(t *testing.T) {
	run := func(gap bool) {
		c := fixtureClient(t, func(api string, p map[string]any) ([]string, [][]any) {
			date := "2026-10-13"
			amount := 110e8
			if api == "idx_mins" {
				date = "2026-10-12"
				amount = 100e8
			}
			rows := [][]any{}
			for i := 0; i < 12; i++ {
				if gap && i == 4 {
					continue
				}
				m := 575 + i*5
				rows = append(rows, []any{fmt.Sprintf("%s %02d:%02d:00", date, m/60, m%60), amount})
			}
			return []string{"trade_time", "amount"}, rows
		})
		e, r, at, err := c.estimate(context.Background(), "2026-10-13", Day{"2026-10-12", 18000}, parseTime("2026-10-13 10:30:00"), "intraday")
		if gap {
			if err == nil {
				t.Fatal("minute gap admitted")
			}
			return
		}
		if err != nil || e == nil || *e < 19799 || *e > 19801 || r == nil || *r < 9.99 || at != "10:30" {
			t.Fatal(e, r, at, err)
		}
	}
	run(false)
	run(true)
}
func TestBreadthRejectsSmallSample(t *testing.T) {
	c := fixtureClient(t, func(string, map[string]any) ([]string, [][]any) {
		return []string{"ts_code", "trade_date", "pct_chg", "amount"}, [][]any{{"600000.SH", "20261012", 1.0, 100.0}}
	})
	if b, e := c.breadth(context.Background(), "2026-10-12", "closed", time.Now()); b != nil || e == nil {
		t.Fatal("partial breadth accepted")
	}
}
func TestAPIErrorRedactsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"code":-1,"msg":"bad secret-token"}`) }))
	defer srv.Close()
	c := New(func() string { return "secret-token" }, "")
	c.URL = srv.URL
	_, e := c.query(context.Background(), "daily", nil, "", 0)
	if e == nil || strings.Contains(e.Error(), "secret-token") {
		t.Fatal("secret exposed")
	}
}

func TestOldIndexQuoteIsExplicitlyStale(t *testing.T) {
	c := fixtureClient(t, func(string, map[string]any) ([]string, [][]any) {
		return []string{"ts_code", "trade_time", "close", "pre_close"}, [][]any{{"000001.SH", "2000-01-04 15:00:00", 101.0, 100.0}}
	})
	rows, err := c.Indexes(context.Background())
	if err != nil || len(rows) != 1 || !rows[0].Meta.Stale || rows[0].Change != 1 {
		t.Fatalf("old quotes must not be treated as live: %+v, %v", rows, err)
	}
}
