package marketpulse

import (
	"context"
	"easy-stock/backend/internal/datahub"
	"easy-stock/backend/internal/foundation"
	"time"
)

// Probe performs one bounded, uncached query. It does not load the dashboard or run AI.
func (c *Client) Probe(ctx context.Context) (datahub.Observation, error) {
	now := time.Now()
	end := foundation.LatestCompletedAStockSession(now)
	rows, err := c.query(ctx, "index_daily", map[string]any{"ts_code": "000002.SH", "start_date": end.AddDate(0, 0, -7).Format("20060102"), "end_date": end.Format("20060102")}, "ts_code,trade_date,amount", 0)
	if err != nil {
		return datahub.Observation{}, err
	}
	var latest time.Time
	count := 0
	for _, r := range rows {
		if str(r, "ts_code") != "000002.SH" || num(r, "amount") <= 0 {
			continue
		}
		at, e := time.ParseInLocation("20060102", str(r, "trade_date"), foundation.AStockLocation)
		if e == nil {
			count++
			if at.After(latest) {
				latest = at
			}
		}
	}
	o := datahub.SessionObservation(count, latest, now, false)
	o.Detail = "上证 A 指日成交额样本；不代表分钟/全市场接口权限"
	return o, nil
}
