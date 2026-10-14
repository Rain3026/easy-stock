package httpapi

import (
	"easy-stock/backend/internal/foundation"
	"testing"
	"time"
)

func TestLimitUpLadderRejectsPreopenDateRelabel(t *testing.T) {
	day := func(s string) time.Time {
		v, _ := time.ParseInLocation("2006-01-02 15:04", s, foundation.AStockLocation)
		return v
	}
	events := []foundation.LimitUpEvent{{Symbol: "600176.SH", Date: day("2026-10-13 00:00"), Streak: 3}, {Symbol: "600176.SH", Date: day("2026-10-14 00:00"), Streak: 10}}
	got, e := buildLimitUpLadder(events, nil, day("2026-10-14 00:15"))
	if e != nil || got.Current.TradeDate != "2026-10-13" || got.SessionStatus != "最近交易日" {
		t.Fatal(got, e)
	}
}
