package marketpulse

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"easy-stock/backend/internal/datahub"
	"easy-stock/backend/internal/foundation"
)

type BreadthProvider interface {
	MarketBreadth(context.Context, string, string, time.Time) (*foundation.MarketBreadth, error)
}

type IndexAmountProvider interface {
	MarketAmountBars(context.Context, string, string, int) ([]foundation.KLine, error)
}

func amountFailure(err error) string {
	var request *url.Error
	if errors.As(err, &request) {
		return request.Err.Error()
	}
	return err.Error()
}

func (c *Client) Load(ctx context.Context, now time.Time) Snapshot {
	primaryCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
	v := c.loadPrimary(primaryCtx, now)
	cancel()
	target, session := Target(now)
	var breadth *foundation.MarketBreadth
	var breadthErr error
	var wg sync.WaitGroup
	if v.Breadth == nil && (c.BreadthFallback != nil || c.BreadthSecondary != nil) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bc, done := context.WithTimeout(ctx, 15*time.Second)
			defer done()
			breadth, breadthErr = c.loadBreadthFallback(bc, target.Format("2006-01-02"), session, now)
		}()
	}
	// The overview has a 35-second budget. Independent breadth fallback must
	// not spend the amount chain's remaining time (18 + max(15, 14) <= 33).
	v = c.withAmountFallback(ctx, now, v)
	wg.Wait()
	if breadthErr == nil && breadth != nil {
		// Keep the chosen source and earlier failure from the breadth chain.
		v.Breadth = breadth
		v.Gaps = append(v.Gaps, breadth.Meta.FallbackReason)
	} else if breadthErr != nil {
		v.Gaps = append(v.Gaps, "宽度备用："+breadthErr.Error())
	}
	return v
}

func (c *Client) withAmountFallback(ctx context.Context, now time.Time, v Snapshot) Snapshot {
	_, session := Target(now)
	needForecast := session != "closed" && now.In(foundation.AStockLocation).Hour()*60+now.In(foundation.AStockLocation).Minute() >= 585
	usable := func(s Snapshot) bool {
		return s.Actual != nil && s.Mean20 != nil && (!needForecast || s.Forecast != nil)
	}
	if usable(v) {
		return v
	}
	// One bounded fallback budget, reserving time for an independent supplier.
	backupCtx, done := context.WithTimeout(ctx, 14*time.Second)
	defer done()
	for _, route := range []struct {
		provider      IndexAmountProvider
		family, label string
	}{
		{c.Fallback, "eastmoney", "东方财富"}, {c.SecondaryFallback, "sina", "新浪"},
	} {
		if route.provider == nil {
			continue
		}
		routeCtx := backupCtx
		cancel := func() {}
		if route.family == "eastmoney" && c.SecondaryFallback != nil {
			routeCtx, cancel = context.WithTimeout(backupCtx, 6*time.Second)
		}
		f := c.loadAmountFallback(routeCtx, now, route.provider, route.family, route.label)
		cancel()
		if usable(f) {
			// Switch the complete group: never splice exchanges or prior-day
			// auction buckets across suppliers. Breadth/emotion remain independent.
			v.History, v.Mean20, v.Previous = f.History, f.Mean20, f.Previous
			v.Actual, v.At, v.Forecast = f.Actual, f.At, f.Forecast
			v.SameTimeChange, v.ForecastMethod, v.Source = f.SameTimeChange, f.ForecastMethod, f.Source
			v.FallbackReason = "优先量能来源未齐备，使用" + route.label + "同口径沪深 A 指整组备用；广度和情绪保持独立来源"
			v.Gaps = append(v.Gaps, v.FallbackReason)
			return v
		}
		if v.Mean20 == nil && f.Mean20 != nil {
			v.History, v.Mean20, v.Previous = f.History, f.Mean20, f.Previous
			v.FallbackReason = "历史量能使用" + route.label + "备用；当日分钟字段尚未恢复"
		}
		v.Gaps = append(v.Gaps, f.Gaps...)
	}
	return v
}

func (c *Client) alternativeRows(ctx context.Context, code, period string) ([]Row, error) {
	return c.alternativeRowsFor(ctx, c.Fallback, "eastmoney", code, period)
}

func (c *Client) alternativeRowsFor(ctx context.Context, provider IndexAmountProvider, family, code, period string) ([]Row, error) {
	key := family + ":index-amount:" + code + ":" + period
	ttl := time.Minute
	if period == "day" {
		ttl = 10 * time.Minute
	}
	if family == "sina" {
		ttl = 30 * time.Second
	}
	c.mu.Lock()
	entry, ok := c.cache[key]
	c.mu.Unlock()
	if ok && (c.validationOnly || time.Since(entry.At) < ttl) {
		return entry.Rows, nil
	}
	limit := 80
	if period == "5" {
		limit = 240
		if family == "sina" {
			// Retain the daily aggregation evidence, not only yesterday/today.
			limit = 1023
		}
	}
	bars, e := provider.MarketAmountBars(ctx, code, period, limit)
	if e != nil {
		return nil, e
	}
	rows := make([]Row, 0, len(bars))
	for _, bar := range bars {
		if bar.Symbol != code || bar.Time.IsZero() || bar.Meta.Family != family || bar.Meta.Adjustment != "none" {
			return nil, fmt.Errorf("备用量能实体或价格口径不符")
		}
		amount, err := datahub.AmountCNY(bar.Amount, bar.Meta.AmountUnit)
		if err != nil {
			return nil, err
		}
		row := Row{"ts_code": code, "amount": amount, "trade_time": bar.Time.In(foundation.AStockLocation).Format("2006-01-02 15:04:05"), "trade_date": bar.Time.In(foundation.AStockLocation).Format("20060102")}
		if period == "day" {
			row["amount"] = amount / 1000
		} // mergeHistory's explicit thousand-yuan input.
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("备用量能返回空序列")
	}
	c.mu.Lock()
	c.cache[key] = cacheEntry{Rows: rows, At: time.Now()}
	c.mu.Unlock()
	return rows, nil
}

func (c *Client) loadFallback(ctx context.Context, now time.Time) Snapshot {
	return c.loadAmountFallback(ctx, now, c.Fallback, "eastmoney", "东方财富")
}

func (c *Client) loadSecondaryFallback(ctx context.Context, now time.Time) Snapshot {
	return c.loadAmountFallback(ctx, now, c.SecondaryFallback, "sina", "新浪")
}

func (c *Client) loadAmountFallback(ctx context.Context, now time.Time, provider IndexAmountProvider, family, label string) Snapshot {
	target, session := Target(now)
	date := target.Format("2006-01-02")
	v := Snapshot{Version: "market-pulse-v1", Date: date, Session: session, FetchedAt: now, Source: label + " · 沪深 A 指成交额（备用）", History: []Day{}, Gaps: []string{}}
	var daily [2][]Row
	var errs [2]error
	var wg sync.WaitGroup
	for i, code := range marketCodes {
		wg.Add(1)
		go func(i int, code string) {
			defer wg.Done()
			daily[i], errs[i] = c.alternativeRowsFor(ctx, provider, family, code, "day")
		}(i, code)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			v.Gaps = append(v.Gaps, label+"日量能备用不可用："+amountFailure(e))
			return v
		}
	}
	for _, d := range mergeHistory(daily[0], daily[1]) {
		if d.Date < date {
			v.History = append(v.History, d)
		}
		if d.Date == date && session == "closed" {
			v.Actual = ptr(d.Amount)
			v.At = date + " 15:00:00"
		}
	}
	if len(v.History) > 20 {
		v.History = v.History[len(v.History)-20:]
	}
	if completeHistory(v.History, target) {
		sum := 0.0
		for _, d := range v.History {
			sum += d.Amount
		}
		v.Mean20 = ptr(sum / 20)
		v.Previous = ptr(v.History[len(v.History)-1].Amount)
	} else {
		v.Gaps = append(v.Gaps, label+"备用前20日量能存在缺口")
	}
	if session == "closed" {
		return v
	}
	var rows [2][]Row
	for i, code := range marketCodes {
		wg.Add(1)
		go func(i int, code string) {
			defer wg.Done()
			rows[i], errs[i] = c.alternativeRowsFor(ctx, provider, family, code, "5")
		}(i, code)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			v.Gaps = append(v.Gaps, label+"分钟量能备用不可用："+amountFailure(e))
			return v
		}
	}
	a, b := cumulative(rows[0], date), cumulative(rows[1], date)
	last := ""
	for at := range a {
		if _, ok := b[at]; ok && at > last && !parseTime(date+" "+at+":00").After(now) {
			last = at
		}
	}
	if last == "" || !fresh(parseTime(date+" "+last+":00"), now, session) || !completeMinutes(rows[0], date, last) || !completeMinutes(rows[1], date, last) {
		v.Gaps = append(v.Gaps, label+"备用分钟数据过期、缺段或两市不同步")
		return v
	}
	v.Actual = ptr(a[last] + b[last])
	v.At = date + " " + last + ":00"
	if v.Previous != nil && last >= "09:45" {
		prev := v.History[len(v.History)-1]
		data := [4][]Row{rows[0], rows[1], rows[0], rows[1]}
		forecast, change, at, e := estimateRows(data, date, prev, now, session)
		if e == nil {
			v.Forecast = forecast
			v.SameTimeChange = change
			v.ForecastMethod = label + "两市同刻分钟比例 · " + at
		} else {
			v.Gaps = append(v.Gaps, e.Error())
		}
	}
	return v
}

func (c *Client) loadBreadthFallback(ctx context.Context, date, session string, now time.Time) (*foundation.MarketBreadth, error) {
	failures := []string{}
	for _, route := range []struct {
		provider BreadthProvider
		label    string
	}{{c.BreadthFallback, "东方财富"}, {c.BreadthSecondary, "新浪"}} {
		if route.provider == nil {
			continue
		}
		callCtx := ctx
		cancel := func() {}
		if route.label == "东方财富" && c.BreadthSecondary != nil {
			callCtx, cancel = context.WithTimeout(ctx, 2*time.Second)
		}
		b, e := route.provider.MarketBreadth(callCtx, date, session, now)
		cancel()
		if e == nil && b != nil {
			b.Meta.FallbackReason = "主来源宽度缺失，使用" + route.label + "沪深A股完整覆盖备用"
			if len(failures) > 0 {
				b.Meta.FallbackReason += "；" + strings.Join(failures, "；")
			}
			return b, nil
		}
		if e != nil {
			failures = append(failures, route.label+"："+amountFailure(e))
		} else {
			failures = append(failures, route.label+"返回空宽度")
		}
	}
	return nil, fmt.Errorf("%s", strings.Join(failures, "；"))
}
