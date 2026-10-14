package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

func (c *Client) AdjustedKLine(ctx context.Context, symbol string, limit int) ([]foundation.KLine, error) {
	return c.KLine(ctx, symbol, "day", limit)
}
func (c *Client) KLine(ctx context.Context, symbol, period string, limit int) ([]foundation.KLine, error) {
	n, e := foundation.NormalizeSymbol(symbol)
	if e != nil {
		return nil, e
	}
	if !(strings.HasSuffix(n.Canonical, ".SH") && strings.HasPrefix(n.Canonical, "6") || strings.HasSuffix(n.Canonical, ".SZ") && (strings.HasPrefix(n.Canonical, "0") || strings.HasPrefix(n.Canonical, "3"))) {
		return nil, fmt.Errorf("Tushare adjusted fallback only supports SH/SZ A shares")
	}
	switch period {
	case "", "daily", "101":
		period = "day"
	case "weekly", "102":
		period = "week"
	case "monthly", "103":
		period = "month"
	}
	if period != "day" && period != "week" && period != "month" {
		return nil, fmt.Errorf("Tushare复权备用仅支持日/周/月历史，不替代分钟行情")
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 500 {
		limit = 500
	}
	days := limit*2 + 90
	if period == "week" {
		days = limit*8 + 90
	}
	if period == "month" {
		days = limit*32 + 90
	}
	if days > 8000 {
		days = 8000
	}
	now := time.Now().In(foundation.AStockLocation)
	params := map[string]any{"ts_code": n.Canonical, "start_date": now.AddDate(0, 0, -days).Format("20060102"), "end_date": now.Format("20060102")}
	var raw, factors []Row
	var re, fe error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); raw, re = c.query(ctx, "daily", params, "", time.Minute) }()
	go func() { defer wg.Done(); factors, fe = c.query(ctx, "adj_factor", params, "", time.Minute) }()
	wg.Wait()
	if re != nil {
		return nil, re
	}
	if fe != nil {
		return nil, fe
	}
	if len(raw) >= 6000 || len(factors) >= 6000 {
		return nil, fmt.Errorf("adjusted history may be truncated")
	}
	bars, e := adjustDaily(raw, factors, n.Canonical, now)
	if e != nil {
		return nil, e
	}
	if period != "day" {
		bars = aggregateAdjusted(bars, period)
	}
	if len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}
	return bars, nil
}
func adjustDaily(raw, factors []Row, symbol string, now time.Time) ([]foundation.KLine, error) {
	factor := map[string]float64{}
	anchorDay := ""
	anchor := 0.0
	for _, r := range factors {
		day := str(r, "trade_date")
		d, e := time.Parse("20060102", day)
		f, fe := requiredNumber(r, "adj_factor")
		if e != nil || fe != nil || f <= 0 || str(r, "ts_code") != symbol || d.Format("20060102") > now.Format("20060102") {
			return nil, fmt.Errorf("invalid adjustment factor")
		}
		if _, ok := factor[day]; ok {
			return nil, fmt.Errorf("duplicate adjustment factor")
		}
		factor[day] = f
		if day > anchorDay {
			anchorDay = day
			anchor = f
		}
	}
	if len(raw) == 0 || anchor <= 0 {
		return nil, fmt.Errorf("adjusted history missing prices/factors")
	}
	meta := foundation.SourceMeta{Family: "tushare", Channel: "http", Source: "tushare:daily+adj_factor", SourceURL: "https://tushare.pro/document/2?doc_id=28", FetchedAt: time.Now(), Adjustment: "qfq", AmountUnit: "CNY", VolumeUnit: "lots", AvailableFields: []string{"open", "high", "low", "close", "volume", "amount"}, FallbackReason: "前复权=原价×当日因子/最新因子；基准日 " + anchorDay + "，整段替换；与其他厂商复权算法可能有差异，不逐点拼接；只含已完成日线，换手率未提供"}
	bars := []foundation.KLine{}
	seen := map[string]bool{}
	cutoff := foundation.LatestCompletedAStockSession(now).Format("20060102")
	for _, r := range raw {
		day := str(r, "trade_date")
		if day > cutoff {
			continue
		}
		at, e := time.ParseInLocation("20060102", day, foundation.AStockLocation)
		if e != nil || str(r, "ts_code") != symbol || seen[day] {
			return nil, fmt.Errorf("invalid/duplicate adjusted price row")
		}
		seen[day] = true
		f, ok := factor[day]
		if !ok {
			return nil, fmt.Errorf("复权因子缺日，拒绝拼接未复权价格")
		}
		v, e := requiredNumbers(r, "open", "high", "low", "close", "vol", "amount")
		if e != nil {
			return nil, e
		}
		if v[0] <= 0 || v[1] < math.Max(v[0], v[3]) || v[2] > math.Min(v[0], v[3]) || v[2] <= 0 || v[3] <= 0 || v[4] < 0 || v[5] < 0 {
			return nil, fmt.Errorf("invalid adjusted OHLC")
		}
		ratio := f / anchor
		bar := foundation.KLine{Symbol: symbol, Time: at, Open: v[0] * ratio, High: v[1] * ratio, Low: v[2] * ratio, Close: v[3] * ratio, Volume: v[4], Amount: v[5] * 1000, Meta: meta}
		bars = append(bars, bar)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no completed adjusted bars")
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Time.Before(bars[j].Time) })
	for i := range bars {
		bars[i].Meta.TradeDate = bars[len(bars)-1].Time.Format("2006-01-02")
		if i > 0 {
			bars[i].PreviousClose = bars[i-1].Close
			bars[i].ChangePercent = (bars[i].Close/bars[i-1].Close - 1) * 100
		}
	}
	return bars, nil
}
func aggregateAdjusted(bars []foundation.KLine, period string) []foundation.KLine {
	out := []foundation.KLine{}
	last := ""
	for _, b := range bars {
		key := b.Time.Format("2006-01")
		if period == "week" {
			y, w := b.Time.ISOWeek()
			key = fmt.Sprintf("%d-%d", y, w)
		}
		if key != last {
			out = append(out, b)
			last = key
			continue
		}
		p := &out[len(out)-1]
		p.Time = b.Time
		p.Close = b.Close
		p.High = math.Max(p.High, b.High)
		p.Low = math.Min(p.Low, b.Low)
		p.Volume += b.Volume
		p.Amount += b.Amount
	}
	for i := range out {
		out[i].PreviousClose = 0
		out[i].ChangePercent = 0
		if i > 0 {
			out[i].PreviousClose = out[i-1].Close
			out[i].ChangePercent = (out[i].Close/out[i-1].Close - 1) * 100
		}
	}
	return out
}
