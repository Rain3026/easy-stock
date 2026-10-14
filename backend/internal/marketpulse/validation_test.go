package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"math"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidationRetainsExactQuoteAfterOtherSourcesExceedTTL(t *testing.T) {
	var quoteCalls atomic.Int32
	c := fixtureClient(t, func(api string, _ map[string]any) ([]string, [][]any) {
		if api == "rt_idx_k" {
			amount := float64(quoteCalls.Add(1)) * 1e8
			return []string{"ts_code", "trade_time", "amount"}, [][]any{
				{"000002.SH", "2026-10-14 14:35:00", amount}, {"399107.SZ", "2026-10-14 14:35:00", amount},
			}
		}
		return nil, nil
	})
	c.validationOnly = true
	c.BreadthSecondary = breadthFunc(func(context.Context, string, string, time.Time) (*foundation.MarketBreadth, error) {
		c.mu.Lock()
		for key, entry := range c.cache {
			entry.At = time.Now().Add(-time.Hour)
			c.cache[key] = entry
		}
		c.mu.Unlock()
		return nil, nil
	})
	v := c.captureValidation(context.Background(), parseTime("2026-10-14 14:35:00"))
	for _, stream := range v.Streams {
		if stream.API == "rt_idx_k" {
			if quoteCalls.Load() != 1 || v.Primary.Actual == nil || len(stream.Rows) != 2 ||
				(num(stream.Rows[0], "amount")+num(stream.Rows[1], "amount"))/1e8 != *v.Primary.Actual {
				t.Fatalf("raw quote was refetched after TTL: calls=%d primary=%+v rows=%+v", quoteCalls.Load(), v.Primary, stream.Rows)
			}
			return
		}
	}
	t.Fatal("missing original realtime evidence")
}

func TestRealtimeRejectsSkewAndInvalidAmounts(t *testing.T) {
	rows := []Row{{"ts_code": "000002.SH", "amount": 1e12, "trade_time": "2026-10-14 10:35:00"}, {"ts_code": "399107.SZ", "amount": 1e12, "trade_time": "2026-10-14 10:32:00"}}
	now := parseTime("2026-10-14 10:35:00")
	if _, _, ok := realtimeAmount(rows, "2026-10-14", now, "intraday"); ok {
		t.Fatal("two individually fresh but asynchronous amounts accepted")
	}
	rows[1]["trade_time"] = "2026-10-14 10:34:10"
	if _, _, ok := realtimeAmount(rows, "2026-10-14", now, "intraday"); !ok {
		t.Fatal("bounded polling skew rejected")
	}
	rows[1]["amount"] = math.Inf(1)
	if _, _, ok := realtimeAmount(rows, "2026-10-14", now, "intraday"); ok {
		t.Fatal("infinite amount accepted")
	}
}

func TestConflictingDuplicateMinuteRejected(t *testing.T) {
	rows := []Row{{"trade_time": "2026-10-14 09:35:00", "amount": 10e8}, {"trade_time": "2026-10-14 09:35:00", "amount": 10e8}}
	if !completeMinutes(rows, "2026-10-14", "09:35") || cumulative(rows, "2026-10-14")["09:35"] != 10 {
		t.Fatal("identical duplicate not safely deduplicated")
	}
	delete(rows[1], "amount")
	if completeMinutes(rows, "2026-10-14", "09:35") {
		t.Fatal("missing amount treated as zero")
	}
	rows[1]["amount"] = 11e8
	if completeMinutes(rows, "2026-10-14", "09:35") {
		t.Fatal("conflicting duplicate accepted")
	}
}

func TestValidationWindowsExcludeReplayAndOutsideSession(t *testing.T) {
	for _, tc := range []struct{ at, window string }{
		{"2026-10-14 07:45:00", "outside"}, {"2026-10-14 09:35:00", "opening"},
		{"2026-10-14 10:35:00", "morning"}, {"2026-10-14 12:35:00", "outside"},
		{"2026-10-14 14:35:00", "afternoon"}, {"2026-10-14 15:35:00", "close"},
		{"2026-10-14 16:35:00", "close"}, {"2026-10-17 10:35:00", "outside"},
		{"2026-10-01 10:35:00", "outside"},
	} {
		if got := ValidationWindow(parseTime(tc.at)); got != tc.window {
			t.Fatalf("%s: %s", tc.at, got)
		}
	}
}
