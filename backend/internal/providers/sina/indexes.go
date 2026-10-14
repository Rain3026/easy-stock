package sina

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
)

// Nasdaq-100 quote only. The composite index and index futures are not substitutes.
func (c *Client) MarketIndexes(ctx context.Context, scope string) ([]foundation.MarketIndexSnapshot, foundation.SourceMeta, error) {
	start := time.Now()
	// Sina does not decode the percent-escaped dollar in this fixed symbol.
	endpoint := c.baseURL + "/?list=gb_$ndx"
	body, err := c.publicText(ctx, endpoint, 64<<10)
	meta := foundation.SourceMeta{Family: "sina", Channel: "http", Source: "sina:index", SourceURL: endpoint, FetchedAt: time.Now(), LatencyMS: time.Since(start).Milliseconds(), AvailableFields: []string{"nasdaq100.price", "change", "change_percent", "trade_time"}, FallbackReason: "新浪纳斯达克100标准指数行情可能延迟至少15分钟；仅报价，不提供本接口的历史K线"}
	if err != nil {
		return nil, meta, err
	}
	row, err := parseNasdaq100(body, meta, time.Now())
	if err != nil {
		return nil, meta, err
	}
	return []foundation.MarketIndexSnapshot{row}, meta, nil
}

func parseNasdaq100(body string, meta foundation.SourceMeta, now time.Time) (foundation.MarketIndexSnapshot, error) {
	bad := func() (foundation.MarketIndexSnapshot, error) {
		return foundation.MarketIndexSnapshot{}, fmt.Errorf("新浪纳斯达克100行情缺失、过期或身份不匹配")
	}
	_, rest, ok := strings.Cut(body, `var hq_str_gb_$ndx="`)
	if !ok {
		return bad()
	}
	payload, _, ok := strings.Cut(rest, `"`)
	if !ok {
		return bad()
	}
	f := strings.Split(payload, ",")
	if len(f) < 27 || !strings.Contains(f[0], "100") || !strings.Contains(f[0], "纳斯达克") {
		return bad()
	}
	nums := make([]float64, 3)
	for i, p := range []int{1, 4, 2} {
		v, e := strconv.ParseFloat(f[p], 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return bad()
		}
		nums[i] = v
	}
	if nums[0] <= 0 {
		return bad()
	}
	cn, _ := time.LoadLocation("Asia/Shanghai")
	ny, _ := time.LoadLocation("America/New_York")
	updated, e := time.ParseInLocation("2006-01-02 15:04:05", f[3], cn)
	if e != nil || updated.After(now.Add(time.Minute)) {
		return bad()
	}
	// f[3] is feed update time; f[25] is the actual exchange quote time.
	var at time.Time
	for _, year := range []int{updated.Year() - 1, updated.Year(), updated.Year() + 1} {
		t, e := time.ParseInLocation("2006 Jan 2 03:04PM MST", fmt.Sprintf("%d %s", year, strings.TrimSpace(f[25])), ny)
		if e != nil {
			continue
		}
		zone, _ := t.Zone()
		if !(strings.HasSuffix(f[25], zone) && (zone == "EDT" || zone == "EST")) {
			continue
		}
		if t.After(updated.Add(time.Minute)) {
			continue
		}
		if at.IsZero() || t.After(at) {
			at = t
		}
	}
	if at.IsZero() || now.Sub(at) > 96*time.Hour {
		return bad()
	}
	status := foundation.IndexSessionStatus("US", at, now)
	if status == "unknown" {
		return bad()
	}
	meta.Stale = at.In(ny).Format("2006-01-02") < expectedUSCloseDate(now)
	return foundation.MarketIndexSnapshot{ID: "nasdaq100", SecID: "gb_$ndx", Code: ".NDX", Name: "纳斯达克100", Region: "美洲", Market: "US", Currency: "USD", Price: nums[0], Change: nums[1], ChangePercent: nums[2], TradeTime: at, Status: status, Meta: meta}, nil
}
