package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"math"
	"time"
)

func requiredNumber(r Row, key string) (float64, error) {
	v, ok := r[key].(float64)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("missing/invalid %s", key)
	}
	return v, nil
}
func requiredNumbers(r Row, keys ...string) ([]float64, error) {
	out := make([]float64, len(keys))
	for i, k := range keys {
		v, e := requiredNumber(r, k)
		if e != nil {
			return nil, e
		}
		out[i] = v
	}
	return out, nil
}
func (c *Client) MarketMarginSeries(ctx context.Context, limit int) ([]foundation.MarketMarginPoint, foundation.SourceMeta, error) {
	if limit <= 0 {
		limit = 120
	}
	if limit > 500 {
		limit = 500
	}
	now := time.Now()
	meta := foundation.SourceMeta{Family: "tushare", Channel: "http", Source: "tushare:margin", SourceURL: "https://tushare.pro/document/2?doc_id=58", AmountUnit: "CNY", FetchedAt: now, AvailableFields: []string{"financing_balance", "securities_lending_balance", "margin_balance", "margin_balance_change", "financing_buy_amount", "financing_repay_amount", "financing_net_buy_amount", "securities_lending_sell_volume"}, FallbackReason: "三市同日汇总；次日披露，融券偿还量未提供"}
	rows, err := c.query(ctx, "margin", map[string]any{"start_date": now.AddDate(0, 0, -limit*2-60).Format("20060102"), "end_date": now.Format("20060102")}, "", 5*time.Minute)
	if err != nil {
		return nil, meta, err
	}
	if len(rows) >= 4000 {
		return nil, meta, fmt.Errorf("margin response may be truncated")
	}
	points := []foundation.MarginExchangePoint{}
	for _, r := range rows {
		day, e := time.Parse("20060102", str(r, "trade_date"))
		if e != nil {
			return nil, meta, e
		}
		v, e := requiredNumbers(r, "rzye", "rqye", "rzrqye", "rzmre", "rzche", "rqmcl")
		if e != nil {
			return nil, meta, e
		}
		points = append(points, foundation.MarginExchangePoint{Exchange: str(r, "exchange_id"), Point: foundation.MarketMarginPoint{TradeDate: day.Format("2006-01-02"), FinancingBalance: v[0], SecuritiesLendingBalance: v[1], MarginBalance: v[2], FinancingBuyAmount: v[3], FinancingRepayAmount: v[4], SecuritiesLendingSellVolume: v[5]}})
	}
	meta.LatencyMS = time.Since(now).Milliseconds()
	return foundation.CompleteMarginSeries(points, limit, meta)
}
