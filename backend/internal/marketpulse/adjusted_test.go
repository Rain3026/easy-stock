package marketpulse

import (
	"easy-stock/backend/internal/foundation"
	"math"
	"testing"
	"time"
)

func TestAdjustedHistorySplitUsesOneAnchorAndLeavesTurnoverUnscaled(t *testing.T) {
	now := time.Date(2026, 8, 12, 18, 0, 0, 0, foundation.AStockLocation)
	price := func(day string, p float64) Row {
		return Row{"ts_code": "600000.SH", "trade_date": day, "open": p, "high": p, "low": p, "close": p, "vol": 100.0, "amount": 1000.0}
	}
	prices := []Row{price("20260812", 10), price("20260811", 20)}
	factors := []Row{{"ts_code": "600000.SH", "trade_date": "20260812", "adj_factor": 2.0}, {"ts_code": "600000.SH", "trade_date": "20260811", "adj_factor": 1.0}}
	bars, e := adjustDaily(prices, factors, "600000.SH", now)
	if e != nil || len(bars) != 2 || bars[0].Close != 10 || bars[1].ChangePercent != 0 || bars[0].Amount != 1e6 || bars[0].Volume != 100 {
		t.Fatal(bars, e)
	}
	if _, e = adjustDaily(prices, factors[:1], "600000.SH", now); e == nil {
		t.Fatal("missing factor accepted")
	}
	if _, e = adjustDaily(append(prices, prices[0]), factors, "600000.SH", now); e == nil {
		t.Fatal("duplicate accepted")
	}
	factors[0]["adj_factor"] = math.NaN()
	if _, e = adjustDaily(prices, factors, "600000.SH", now); e == nil {
		t.Fatal("NaN accepted")
	}
}
func TestAdjustedBeforeCloseExcludesPartialDayAndAggregatesAfterAdjusting(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, foundation.AStockLocation)
	prices := []Row{}
	factors := []Row{}
	for _, day := range []string{"20260810", "20260811", "20260812"} {
		prices = append(prices, Row{"ts_code": "600000.SH", "trade_date": day, "open": 10.0, "high": 12.0, "low": 9.0, "close": 11.0, "vol": 100.0, "amount": 1000.0})
		factors = append(factors, Row{"ts_code": "600000.SH", "trade_date": day, "adj_factor": 2.0})
	}
	bars, e := adjustDaily(prices, factors, "600000.SH", now)
	if e != nil || len(bars) != 2 {
		t.Fatal(bars, e)
	}
	weeks := aggregateAdjusted(bars, "week")
	if len(weeks) != 1 || weeks[0].Volume != 200 || weeks[0].Close != 11 || weeks[0].Time.Day() != 11 {
		t.Fatal(weeks)
	}
}
