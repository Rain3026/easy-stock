package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"strings"
	"time"
)

func (c *Client) Realtime(ctx context.Context, symbols []string) ([]foundation.Quote, error) {
	keys := []string{}
	wanted := map[string]bool{}
	for _, symbol := range symbols {
		n, e := foundation.NormalizeSymbol(symbol)
		if e != nil {
			return nil, e
		}
		if !wanted[n.Canonical] {
			keys = append(keys, n.Canonical)
			wanted[n.Canonical] = true
		}
	}
	if len(keys) == 0 || len(keys) > 80 {
		return nil, fmt.Errorf("quote request must contain 1 to 80 symbols")
	}
	rows, e := c.query(ctx, "rt_k", map[string]any{"ts_code": strings.Join(keys, ",")}, "ts_code,name,trade_time,pre_close,open,high,low,close,vol,amount", 3*time.Second)
	if e != nil {
		return nil, e
	}
	out := []foundation.Quote{}
	for _, r := range rows {
		code := str(r, "ts_code")
		if !wanted[code] {
			continue
		}
		at := parseTime(str(r, "trade_time"))
		price, previous := num(r, "close"), num(r, "pre_close")
		if at.IsZero() || price <= 0 || previous <= 0 {
			continue
		}
		out = append(out, foundation.Quote{Symbol: code, Name: str(r, "name"), Price: price, PreviousClose: previous, Open: num(r, "open"), High: num(r, "high"), Low: num(r, "low"), Volume: num(r, "vol"), Amount: num(r, "amount"), Change: price - previous, ChangePercent: (price/previous - 1) * 100, TradeTime: at, Meta: foundation.SourceMeta{Family: "tushare", Channel: "http", Source: "tushare:rt_k", FetchedAt: time.Now(), AmountUnit: "CNY", VolumeUnit: "shares", Adjustment: "none", TradeDate: at.Format("2006-01-02")}})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Tushare 实时报价缺少有效价格或来源时间")
	}
	return out, nil
}
