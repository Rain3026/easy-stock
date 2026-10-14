package datahub

import (
	"easy-stock/backend/internal/foundation"
	"time"
)

// SessionObservation uses the exchange calendar, not elapsed weekend days.
// A source with no business timestamp remains unverified even if fetched now.
func SessionObservation(count int, at, now time.Time, intraday bool) Observation {
	o := Observation{Count: count, DataAt: at, Freshness: "unverified"}
	if at.IsZero() {
		return o
	}
	at = at.In(foundation.AStockLocation)
	now = now.In(foundation.AStockLocation)
	o.TradeDate = at.Format("2006-01-02")
	o.Freshness = "verified"
	if at.After(now.Add(2 * time.Minute)) {
		o.Stale = true
		return o
	}
	expected := foundation.LatestCompletedAStockSession(now)
	minute := now.Hour()*60 + now.Minute()
	if intraday && foundation.IsAStockTradingDay(now) && minute >= 570 {
		expected = now
	}
	o.Stale = o.TradeDate < expected.Format("2006-01-02")
	if intraday && !o.Stale {
		boundary := 15 * 60
		if foundation.IsAStockTradingDay(now) && minute >= 570 && minute < 900 {
			boundary = minute
			if minute >= 690 && minute < 780 {
				boundary = 690
			}
		}
		if at.Hour()*60+at.Minute() < boundary-5 {
			o.Stale = true
		}
	}
	return o
}
