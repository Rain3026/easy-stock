package sina

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
)

func (c *Client) MarketIndexSeries(ctx context.Context, id, period string, limit int) (foundation.MarketIndexSeries, error) {
	if id != "nasdaq100" {
		return foundation.MarketIndexSeries{}, fmt.Errorf("unsupported Sina index %q", id)
	}
	if period == "" || period == "daily" {
		period = "day"
	}
	if period != "day" && period != "week" && period != "month" {
		return foundation.MarketIndexSeries{}, fmt.Errorf("unsupported period")
	}
	if limit <= 0 {
		limit = 120
	}
	limit = min(limit, 500)
	// Fixed identity, not the Nasdaq Composite or futures. Parse JSONP as data only.
	u := c.reportBaseURL + "/usstock/api/jsonp.php/IO.XSRV2.CallbackList/US_MinKService.getDailyK?symbol=.NDX&_var=kline_dayqfq&range=30d"
	start := time.Now()
	body, err := c.publicText(ctx, u, 2<<20)
	if err != nil {
		return foundation.MarketIndexSeries{}, err
	}
	meta := foundation.SourceMeta{Family: "sina", Channel: "http", Source: "sina:index-kline", SourceURL: u, FetchedAt: time.Now(), LatencyMS: time.Since(start).Milliseconds(), Adjustment: "none", AvailableFields: []string{"open", "high", "low", "close", "trade_date"}}
	return parseNasdaqHistory(body, period, limit, meta, time.Now())
}

func parseNasdaqHistory(body, period string, limit int, meta foundation.SourceMeta, now time.Time) (foundation.MarketIndexSeries, error) {
	bad := func() (foundation.MarketIndexSeries, error) {
		return foundation.MarketIndexSeries{}, fmt.Errorf("新浪纳指100历史序列缺失、过期或格式不符")
	}
	_, payload, ok := strings.Cut(body, "IO.XSRV2.CallbackList(")
	if !ok {
		return bad()
	}
	end := strings.LastIndex(payload, ")")
	if end < 0 {
		return bad()
	}
	var raw []struct {
		Date  string `json:"d"`
		Open  string `json:"o"`
		High  string `json:"h"`
		Low   string `json:"l"`
		Close string `json:"c"`
	}
	if json.Unmarshal([]byte(payload[:end]), &raw) != nil || len(raw) == 0 {
		return bad()
	}
	ny, _ := time.LoadLocation("America/New_York")
	seen := map[string]foundation.KLine{}
	for _, r := range raw {
		day, err := time.ParseInLocation("2006-01-02", r.Date, ny)
		if err != nil || day.After(now) {
			return bad()
		}
		if day.Add(16 * time.Hour).After(now) {
			continue
		} // Daily endpoint has no intraday observation timestamp.
		prices := make([]float64, 4)
		for i, s := range []string{r.Open, r.High, r.Low, r.Close} {
			p, e := strconv.ParseFloat(s, 64)
			if e != nil || p <= 0 || math.IsNaN(p) || math.IsInf(p, 0) {
				return bad()
			}
			prices[i] = p
		}
		if prices[1] < max(prices[0], prices[3]) || prices[2] > min(prices[0], prices[3]) {
			return bad()
		}
		line := foundation.KLine{Symbol: "nasdaq100", Time: day, Open: prices[0], High: prices[1], Low: prices[2], Close: prices[3]}
		if old, exists := seen[r.Date]; exists && (old.Open != line.Open || old.High != line.High || old.Low != line.Low || old.Close != line.Close) {
			return bad()
		}
		seen[r.Date] = line
	}
	lines := make([]foundation.KLine, 0, len(seen))
	for _, r := range seen {
		lines = append(lines, r)
	}
	if len(lines) == 0 {
		return bad()
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Time.Before(lines[j].Time) })
	latest := lines[len(lines)-1]
	closeAt := latest.Time.Add(16 * time.Hour)
	if now.Sub(closeAt) > 96*time.Hour {
		return bad()
	}
	meta.TradeDate = latest.Time.Format("2006-01-02")
	meta.Stale = latest.Time.Format("2006-01-02") < expectedUSCloseDate(now)
	meta.FallbackReason = "新浪纳指100历史OHLC，截至 " + meta.TradeDate + "；可能晚于报价更新，成交量/金额未核实，不补齐缺失交易日"
	if period != "day" {
		grouped := []foundation.KLine{}
		key := ""
		for _, line := range lines {
			y, w := line.Time.ISOWeek()
			k := fmt.Sprintf("%d-W%02d", y, w)
			if period == "month" {
				k = line.Time.Format("2006-01")
			}
			if k != key {
				grouped = append(grouped, line)
				key = k
			} else {
				g := &grouped[len(grouped)-1]
				g.High = max(g.High, line.High)
				g.Low = min(g.Low, line.Low)
				g.Close = line.Close
				g.Time = line.Time
			}
		}
		lines = grouped
	}
	for i := range lines {
		lines[i].Meta = meta
		if i > 0 {
			lines[i].PreviousClose = lines[i-1].Close
			lines[i].ChangePercent = (lines[i].Close/lines[i-1].Close - 1) * 100
		}
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	last := lines[len(lines)-1]
	index := foundation.MarketIndexSnapshot{ID: "nasdaq100", SecID: "gb_$ndx", Code: ".NDX", Name: "纳斯达克100", Region: "美洲", Market: "US", Currency: "USD", Price: last.Close, TradeTime: closeAt, Status: "closed", Meta: meta}
	return foundation.MarketIndexSeries{Index: index, Lines: lines, Meta: meta}, nil
}

// Conservative weekday boundary: unverified US holidays stay disclosed as delayed.
func expectedUSCloseDate(now time.Time) string {
	ny, _ := time.LoadLocation("America/New_York")
	d := now.In(ny)
	if d.Hour() < 16 {
		d = d.AddDate(0, 0, -1)
	}
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d.Format("2006-01-02")
}
