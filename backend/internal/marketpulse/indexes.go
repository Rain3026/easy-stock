package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"strings"
	"time"
)

// Timed domestic quotes avoid treating abbreviated index feeds without a source timestamp as live.
func (c *Client) Indexes(ctx context.Context) ([]foundation.MarketIndexSnapshot, error) {
	ids := map[string][2]string{"000001.SH": {"sse", "上证指数"}, "399001.SZ": {"szse", "深证成指"}, "399006.SZ": {"chinext", "创业板指"}, "000300.SH": {"csi300", "沪深300"}, "000852.SH": {"csi1000", "中证1000"}}
	rows, e := c.query(ctx, "rt_idx_k", map[string]any{"ts_code": "000001.SH,399001.SZ,399006.SZ,000300.SH,000852.SH"}, "ts_code,trade_time,close,pre_close", time.Minute)
	if e != nil {
		return nil, e
	}
	out := []foundation.MarketIndexSnapshot{}
	now := time.Now()
	target, session := Target(now)
	for _, r := range rows {
		code := str(r, "ts_code")
		id, ok := ids[code]
		if !ok || num(r, "close") <= 0 || num(r, "pre_close") <= 0 {
			continue
		}
		t := parseTime(str(r, "trade_time"))
		if t.IsZero() {
			continue
		}
		status := "trading"
		if t.Hour() >= 15 || t.Format("2006-01-02") != now.In(foundation.AStockLocation).Format("2006-01-02") {
			status = "closed"
		}
		stale := t.Format("2006-01-02") != target.Format("2006-01-02") || !fresh(t, now, session)
		out = append(out, foundation.MarketIndexSnapshot{ID: id[0], Code: strings.Split(code, ".")[0], Name: id[1], Market: "CN", Price: num(r, "close"), Change: num(r, "close") - num(r, "pre_close"), ChangePercent: (num(r, "close")/num(r, "pre_close") - 1) * 100, TradeTime: t, Status: status, Meta: foundation.SourceMeta{Source: "tushare:rt_idx_k", FetchedAt: now, TradeDate: t.Format("2006-01-02"), Stale: stale}})
	}
	return out, nil
}
