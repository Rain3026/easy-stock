package sina

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"easy-stock/backend/internal/foundation"
)

func amountFixtureRows() []map[string]any {
	rows := []map[string]any{}
	start, _ := time.ParseInLocation("2006-01-02", "2026-09-07", foundation.AStockLocation)
	end, _ := time.ParseInLocation("2006-01-02", "2026-10-14", foundation.AStockLocation)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if !foundation.IsAStockTradingDay(day) {
			continue
		}
		for m := 575; m <= 900; m += 5 {
			if m > 690 && m <= 780 {
				continue
			}
			rows = append(rows, map[string]any{"day": day.Add(time.Duration(m) * time.Minute).Format("2006-01-02 15:04:05"), "amount": "100000000"})
		}
	}
	return rows
}

func TestMarketAmountsUseCompleteMinutesNotVolumeOrFutureBars(t *testing.T) {
	rows := amountFixtureRows()
	// A truncated earliest day must not inflate history coverage.
	rows = rows[9:]
	b, _ := json.Marshal(rows)
	now, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-10-14 11:27:00", foundation.AStockLocation)
	bars, err := parseMarketAmounts(append([]byte("callback("), append(b, ')')...), "000002.SH", foundation.SourceMeta{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := bars[len(bars)-1].Time.Format("15:04"); got != "11:25" {
		t.Fatal("forming bar admitted", got)
	}
	days := completeAmountDays(bars)
	if len(days) != 20 || days[0].Time.Format("2006-01-02") != "2026-09-08" || days[19].Amount != 48e8 {
		t.Fatalf("bad complete-day series: %+v", days)
	}
	for _, row := range rows {
		if row["day"] == "2026-10-13 10:00:00" {
			row["day"] = "2026-10-13 10:05:00"
		}
	}
	b, _ = json.Marshal(rows)
	bars, err = parseMarketAmounts(b, "000002.SH", foundation.SourceMeta{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(completeAmountDays(bars)) != 19 {
		t.Fatal("duplicate masked missing bucket")
	}
}

func TestMarketAmountsRejectInvalidAmountAndConflictingDuplicates(t *testing.T) {
	now, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-10-14 11:27:00", foundation.AStockLocation)
	for _, payload := range []string{
		`[{"day":"2026-10-14 09:35:00","volume":"999"}]`,
		`[{"day":"2026-10-14 09:35:00","amount":null}]`,
		`[{"day":"2026-10-14 09:35:00","amount":"-1"}]`,
		`[{"day":"2026-10-14 09:35:00","amount":"NaN"}]`,
		`[{"day":"2026-10-14 09:35:00","amount":"+Inf"}]`,
		`[{"day":"2026-10-14 09:35:00","amount":"1"},{"day":"2026-10-14 09:35:00","amount":"2"}]`,
		`[{"day":"2026-10-14 12:00:00","amount":"1"}]`,
	} {
		// Use a later cutoff for malformed lunch buckets.
		if _, err := parseMarketAmounts([]byte(payload), "000002.SH", foundation.SourceMeta{}, now.Add(4*time.Hour)); err == nil {
			t.Fatal("invalid amount accepted", payload)
		}
	}
}

func TestMarketAmountProviderContractAndSharedFetch(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("scale") != "5" || r.URL.Query().Get("datalen") != "1023" || r.URL.Query().Get("symbol") != "sh000002" {
			t.Error("incorrect index history request")
		}
		b, _ := json.Marshal(amountFixtureRows()[:48])
		fmt.Fprintf(w, "callback(%s);", b)
	}))
	defer srv.Close()
	c := NewClient(WithKLineBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	for _, period := range []string{"day", "5"} {
		bars, err := c.MarketAmountBars(context.Background(), "000002.SH", period, 80)
		if err != nil {
			t.Fatal(err)
		}
		if bars[0].Meta.Family != "sina" || bars[0].Meta.AmountUnit != "CNY" || bars[0].Meta.Adjustment != "none" {
			t.Fatal("missing strict provenance")
		}
		bars[0].Amount = -1
	}
	if calls != 1 {
		t.Fatal("daily/minute did not reuse the same snapshot")
	}
	if _, err := c.MarketAmountBars(context.Background(), "399001.SZ", "day", 20); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatal("wrong Shenzhen universe accepted")
	}
}
