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

var marketCodes = []string{"000002.SH", "399107.SZ"}

func Target(now time.Time) (time.Time, string) {
	now = now.In(foundation.AStockLocation)
	m := now.Hour()*60 + now.Minute()
	if !foundation.IsAStockTradingDay(now) || m < 570 {
		return foundation.LatestCompletedAStockSession(now), "closed"
	}
	if m >= 900 {
		return now, "closed"
	}
	if m >= 690 && m < 780 {
		return now, "lunch"
	}
	return now, "intraday"
}
func (c *Client) loadPrimary(ctx context.Context, now time.Time) Snapshot {
	target, session := Target(now)
	date := target.Format("2006-01-02")
	v := Snapshot{Version: "market-pulse-v1", Date: date, Session: session, FetchedAt: now, Source: "Tushare · 沪深A指成交额", History: []Day{}, Gaps: []string{}}
	if c.Token() == "" {
		v.Gaps = append(v.Gaps, "请在系统设置的数据源中配置 Tushare Token")
		return v
	}
	var histories [2][]Row
	var errs [2]error
	var wg sync.WaitGroup
	for i, code := range marketCodes {
		wg.Add(1)
		go func(i int, code string) {
			defer wg.Done()
			histories[i], errs[i] = c.daily(ctx, "index_daily", code, target.AddDate(0, 0, -100).Format("20060102"), target.Format("20060102"))
		}(i, code)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			v.Gaps = append(v.Gaps, e.Error())
		}
	}
	days := mergeHistory(histories[0], histories[1])
	for _, d := range days {
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
	if len(v.History) > 0 {
		v.Previous = ptr(v.History[len(v.History)-1].Amount)
	}
	if completeHistory(v.History, target) {
		sum := 0.0
		for _, d := range v.History {
			sum += d.Amount
		}
		v.Mean20 = ptr(sum / 20)
	} else {
		v.Gaps = append(v.Gaps, "前20个完整交易日成交额有缺口，暂不计算均额")
	}
	if session != "closed" || v.Actual == nil {
		rows, e := c.query(ctx, "rt_idx_k", map[string]any{"ts_code": strings.Join(marketCodes, ",")}, "ts_code,trade_time,amount", 30*time.Second)
		if e != nil {
			v.Gaps = append(v.Gaps, e.Error())
		} else if a, at, ok := realtimeAmount(rows, date, now, session); ok {
			v.Actual = ptr(a)
			v.At = at
		} else {
			v.Gaps = append(v.Gaps, "沪深实时成交额缺失、跨日或已过期")
		}
	}
	wg.Add(2)
	var b *Breadth
	var be error
	var em *Emotion
	var ee error
	go func() { defer wg.Done(); b, be = c.breadth(ctx, date, session, now) }()
	go func() { defer wg.Done(); em, ee = c.emotion(ctx, date, session) }()
	if session != "closed" && v.Actual != nil {
		if len(v.History) > 0 {
			prev := v.History[len(v.History)-1]
			estimate, change, at, e := c.estimate(ctx, date, prev, now, session)
			if e != nil {
				v.Gaps = append(v.Gaps, e.Error())
			} else {
				v.SameTimeChange = change
				if now.In(foundation.AStockLocation).Hour()*60+now.In(foundation.AStockLocation).Minute() >= 585 {
					if estimate != nil && *estimate >= *v.Actual {
						v.Forecast = estimate
					} else {
						v.Gaps = append(v.Gaps, "预估低于已实现成交额，等待下一次完整分钟更新")
					}
					v.ForecastMethod = "同刻比例法 · " + at
				} else {
					v.Gaps = append(v.Gaps, "开盘不足15分钟，暂不外推全天成交额")
				}
			}
		}
	}
	wg.Wait()
	v.Breadth = b
	v.Emotion = em
	for _, e := range []error{be, ee} {
		if e != nil {
			v.Gaps = append(v.Gaps, e.Error())
		}
	}
	if v.Actual == nil {
		v.Gaps = append(v.Gaps, "本交易日量能尚未就绪")
	}
	return v
}
func mergeHistory(a, b []Row) []Day {
	m := map[string]float64{}
	for _, r := range a {
		if num(r, "amount") > 0 {
			m[str(r, "trade_date")] = num(r, "amount") / 1e5
		}
	}
	out := []Day{}
	seen := map[string]bool{}
	for _, r := range b {
		d := str(r, "trade_date")
		x, ok := m[d]
		if ok && len(d) == 8 && num(r, "amount") > 0 && !seen[d] {
			out = append(out, Day{d[:4] + "-" + d[4:6] + "-" + d[6:], x + num(r, "amount")/1e5})
			seen[d] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}
func completeHistory(days []Day, target time.Time) bool {
	if len(days) != 20 {
		return false
	}
	d := target
	for i := 19; i >= 0; i-- {
		d = d.AddDate(0, 0, -1)
		for !foundation.IsAStockTradingDay(d) {
			d = d.AddDate(0, 0, -1)
		}
		if days[i].Date != d.Format("2006-01-02") {
			return false
		}
	}
	return true
}
func parseTime(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04:05", s, foundation.AStockLocation)
	return t
}
func fresh(t, now time.Time, session string) bool {
	if t.IsZero() || t.After(now.Add(time.Minute)) {
		return false
	}
	n := now.In(foundation.AStockLocation)
	if session == "closed" {
		return t.Hour() >= 15
	}
	if session == "lunch" {
		return t.Hour() == 11 && t.Minute() >= 25
	}
	if n.Hour() == 13 && n.Minute() < 5 && t.Hour() == 11 && t.Minute() >= 25 {
		return true
	}
	return now.Sub(t) <= 7*time.Minute
}
func realtimeAmount(rows []Row, date string, now time.Time, session string) (float64, string, bool) {
	seen := map[string]bool{}
	total := 0.0
	at := ""
	var first, latest time.Time
	for _, r := range rows {
		code := str(r, "ts_code")
		if code != marketCodes[0] && code != marketCodes[1] {
			continue
		}
		t := parseTime(str(r, "trade_time"))
		amount := num(r, "amount")
		if t.Format("2006-01-02") != date || !fresh(t, now, session) || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || seen[code] {
			return 0, "", false
		}
		seen[code] = true
		total += num(r, "amount") / 1e8
		if first.IsZero() || t.Before(first) {
			first = t
			at = str(r, "trade_time")
		}
		if t.After(latest) {
			latest = t
		}
	}
	return total, at, len(seen) == 2 && latest.Sub(first) <= time.Minute
}
func (c *Client) breadth(ctx context.Context, date, session string, now time.Time) (*Breadth, error) {
	var rows []Row
	if session == "closed" {
		r, e := c.query(ctx, "daily", map[string]any{"trade_date": strings.ReplaceAll(date, "-", "")}, "ts_code,trade_date,pct_chg,amount", 12*time.Hour)
		if e != nil {
			return nil, e
		}
		rows = r
	} else {
		for _, code := range []string{"6*.SH", "0*.SZ,3*.SZ"} {
			r, e := c.query(ctx, "rt_k", map[string]any{"ts_code": code}, "ts_code,close,pre_close,amount,trade_time", time.Minute)
			if e != nil {
				return nil, e
			}
			rows = append(rows, r...)
		}
	}
	b := &Breadth{Date: date, Source: "Tushare · 沪深A股有效交易样本", At: date + " 15:00:00", Meta: foundation.SourceMeta{Family: "tushare", Channel: "http", Source: "tushare:breadth", TradeDate: date, FetchedAt: now}}
	members := []foundation.BreadthMember{}
	var earliest, latest time.Time
	counts := map[string]int{}
	seen := map[string]bool{}
	missing := 0
	for _, r := range rows {
		code := str(r, "ts_code")
		if !foundation.IsSHSZAShare(code) {
			continue
		}
		if seen[code] {
			return nil, fmt.Errorf("Tushare宽度重复证券")
		}
		seen[code] = true
		amount, exists := r["amount"].(float64)
		if !exists || math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
			missing++
			continue
		}
		member := foundation.BreadthMember{Symbol: code, Amount: amount, ChangeUnit: "CNY"}
		if session == "closed" {
			member.Amount *= 1000
			member.ChangeUnit = "percent"
		}
		if amount == 0 {
			members = append(members, member)
			continue
		}
		change := num(r, "pct_chg")
		if session == "closed" {
			if _, ok := r["pct_chg"].(float64); !ok {
				missing++
				continue
			}
		}
		if session != "closed" {
			t := parseTime(str(r, "trade_time"))
			if t.Format("2006-01-02") != date || !fresh(t, now, session) || num(r, "pre_close") <= 0 || num(r, "close") <= 0 {
				missing++
				continue
			}
			change = num(r, "close") - num(r, "pre_close")
			member.At = str(r, "trade_time")
			if earliest.IsZero() || t.Before(earliest) {
				earliest = t
			}
			if t.After(latest) {
				latest = t
			}
		} else if str(r, "trade_date") != strings.ReplaceAll(date, "-", "") {
			missing++
			continue
		}
		if session == "closed" {
			member.At = date + " 15:00:00"
		}
		if math.IsNaN(change) || math.IsInf(change, 0) {
			missing++
			continue
		}
		member.Change = change
		members = append(members, member)
		counts[code[len(code)-2:]]++
		if change > 0 {
			b.Up++
		} else if change < 0 {
			b.Down++
		} else {
			b.Flat++
		}
	}
	// Refuse a truncated/one-exchange response instead of presenting sample breadth as full-market breadth.
	if missing > 0 || counts["SH"] < 1500 || counts["SZ"] < 2000 {
		return nil, fmt.Errorf("沪深广度覆盖不足或行情过期，暂不用于判断")
	}
	if session != "closed" {
		if latest.Sub(earliest) > 2*time.Minute {
			return nil, fmt.Errorf("Tushare宽度行情不同步")
		}
		b.At = earliest.Format("2006-01-02 15:04:05")
		b.LatestAt = latest.Format("2006-01-02 15:04:05")
	}
	foundation.AttachBreadthMembers(b, members)
	return b, nil
}

func (c *Client) MarketBreadth(ctx context.Context, date, session string, now time.Time) (*foundation.MarketBreadth, error) {
	return c.breadth(ctx, date, session, now)
}
func (c *Client) emotion(ctx context.Context, date, session string) (*Emotion, error) {
	// Daily limit data is published after close; do not label yesterday's sample as today's intraday emotion.
	if session != "closed" {
		return nil, nil
	}
	rows, e := c.query(ctx, "limit_list_d", map[string]any{"trade_date": strings.ReplaceAll(date, "-", "")}, "ts_code,name,trade_date,limit,limit_times", 10*time.Minute)
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("涨跌停日统计尚未发布")
	}
	v := &Emotion{Date: date, Source: "Tushare · 沪深非ST涨跌停日统计", Levels: []Level{}}
	levels := map[int]int{}
	broken := 0
	for _, r := range rows {
		if strings.Contains(str(r, "name"), "ST") || strings.HasSuffix(str(r, "ts_code"), ".BJ") {
			continue
		}
		switch str(r, "limit") {
		case "U":
			v.Up++
			n := int(num(r, "limit_times"))
			if n > 1 {
				v.Boards++
				levels[n]++
			}
			if n > v.Height {
				v.Height = n
			}
		case "D":
			v.Down++
		case "Z":
			broken++
		}
	}
	if v.Up+broken > 0 {
		v.BrokenRate = float64(broken) / float64(v.Up+broken) * 100
	}
	for h, n := range levels {
		v.Levels = append(v.Levels, Level{h, n})
	}
	sort.Slice(v.Levels, func(i, j int) bool { return v.Levels[i].Height > v.Levels[j].Height })
	return v, nil
}

// cumulative aligns complete five-minute buckets on all four streams. No clock-linear fallback.
func cumulative(rows []Row, date string) map[string]float64 {
	rows = append([]Row(nil), rows...)
	sort.Slice(rows, func(i, j int) bool {
		return str(rows[i], "trade_time")+str(rows[i], "time") < str(rows[j], "trade_time")+str(rows[j], "time")
	})
	out := map[string]float64{}
	total := 0.0
	seen := map[string]bool{}
	for _, r := range rows {
		s := str(r, "trade_time")
		if s == "" {
			s = str(r, "time")
		}
		if len(s) < 19 || s[:10] != date || num(r, "amount") < 0 || seen[s] {
			continue
		}
		seen[s] = true
		total += num(r, "amount") / 1e8
		out[s[11:16]] = total
	}
	return out
}
func (c *Client) estimate(ctx context.Context, date string, prev Day, now time.Time, session string) (*float64, *float64, string, error) {
	var data [4][]Row
	var errs [4]error
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := marketCodes[i%2]
			if i < 2 {
				data[i], errs[i] = c.query(ctx, "rt_idx_min_daily", map[string]any{"ts_code": code, "freq": "5MIN"}, "", time.Minute)
			} else {
				data[i], errs[i] = c.query(ctx, "idx_mins", map[string]any{"ts_code": code, "freq": "5min", "start_date": prev.Date + " 09:00:00", "end_date": prev.Date + " 15:00:00"}, "", 24*time.Hour)
			}
		}(i)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return nil, nil, "", e
		}
	}
	return estimateRows(data, date, prev, now, session)
}

func estimateRows(data [4][]Row, date string, prev Day, now time.Time, session string) (*float64, *float64, string, error) {
	a, b := cumulative(data[0], date), cumulative(data[1], date)
	y, z := cumulative(data[2], prev.Date), cumulative(data[3], prev.Date)
	last := ""
	for t := range a {
		if parseTime(date + " " + t + ":00").After(now) {
			continue
		}
		if _, ok := b[t]; ok && t > last {
			if _, ok = y[t]; ok {
				if _, ok = z[t]; ok {
					last = t
				}
			}
		}
	}
	at := parseTime(date + " " + last + ":00")
	if last == "" || !fresh(at, now, session) {
		return nil, nil, "", fmt.Errorf("缺少有效的昨日/今日同刻分钟数据，暂不预估")
	}
	den := y[last] + z[last]
	actual := a[last] + b[last]
	if den <= 0 || actual <= 0 {
		return nil, nil, "", fmt.Errorf("同刻成交额不足")
	}
	// Gapped minute streams cannot safely be treated as cumulative full sessions.
	for i, rows := range data {
		d := date
		if i >= 2 {
			d = prev.Date
		}
		if !completeMinutes(rows, d, last) {
			return nil, nil, "", fmt.Errorf("分钟序列有缺口，暂不预估")
		}
	}
	estimated := actual / den * prev.Amount
	if math.IsNaN(estimated) || estimated < actual || den > prev.Amount*1.02 {
		return nil, nil, "", fmt.Errorf("分钟与日线成交额口径不一致")
	}
	return ptr(estimated), ptr((actual/den - 1) * 100), last, nil
}
func completeMinutes(rows []Row, date, last string) bool {
	seen := map[string]float64{}
	for _, row := range rows {
		stamp := str(row, "trade_time")
		if stamp == "" {
			stamp = str(row, "time")
		}
		if len(stamp) < 19 || stamp[:10] != date || stamp[11:16] > last {
			continue
		}
		amount, hasAmount := row["amount"].(float64)
		if !hasAmount || amount < 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
			return false
		}
		if old, exists := seen[stamp]; exists && old != amount {
			return false // Conflicting duplicates cannot select an arbitrary amount.
		}
		seen[stamp] = amount
	}
	m := cumulative(rows, date)
	for minute := 575; minute <= 900; minute += 5 {
		if minute > 690 && minute <= 780 {
			continue
		}
		key := fmt.Sprintf("%02d:%02d", minute/60, minute%60)
		if key > last {
			break
		}
		if _, ok := m[key]; !ok {
			return false
		}
	}
	return len(m) > 0
}
