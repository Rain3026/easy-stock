package eastmoney

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type breadthPage struct {
	RC   int `json:"rc"`
	Data struct {
		Total int              `json:"total"`
		Diff  []map[string]any `json:"diff"`
	} `json:"data"`
}

func (c *Client) MarketBreadth(ctx context.Context, date, session string, now time.Time) (*foundation.MarketBreadth, error) {
	started := time.Now()
	params := url.Values{"pz": {"100"}, "po": {"1"}, "np": {"1"}, "fltt": {"2"}, "invt": {"2"}, "fid": {"f12"}, "fs": {"m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23"}, "fields": {"f2,f6,f12,f13,f18,f124"}}
	endpoint := c.quoteBaseURL + "/api/qt/clist/get?"
	fetch := func(page int) (breadthPage, error) {
		p := url.Values{}
		for k, v := range params {
			p[k] = v
		}
		p.Set("pn", strconv.Itoa(page))
		var v breadthPage
		e := c.getJSONWithRetry(ctx, endpoint+p.Encode(), &v)
		if e == nil && (v.RC != 0 || v.Data.Total < 1 || len(v.Data.Diff) == 0) {
			e = fmt.Errorf("empty breadth page")
		}
		return v, e
	}
	first, e := fetch(1)
	if e != nil {
		return nil, e
	}
	total := first.Data.Total
	if total > 10000 {
		return nil, fmt.Errorf("unexpected breadth universe size")
	}
	pages := (total + 99) / 100
	data := make([]breadthPage, pages)
	data[0] = first
	errors := make([]error, pages)
	jobs := make(chan int, pages)
	for i := 1; i < pages; i++ {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range jobs {
				data[page], errors[page] = fetch(page + 1)
			}
		}()
	}
	wg.Wait()
	rows := make([]map[string]any, 0, total)
	for i, p := range data {
		if errors[i] != nil {
			return nil, errors[i]
		}
		if p.Data.Total != total {
			return nil, fmt.Errorf("breadth universe changed during pagination")
		}
		rows = append(rows, p.Data.Diff...)
	}
	result, e := aggregateBreadth(rows, total, date, session, now)
	if e != nil {
		return nil, e
	}
	result.Meta.SourceURL = endpoint + params.Encode()
	result.Meta.LatencyMS = time.Since(started).Milliseconds()
	return result, nil
}
func aggregateBreadth(rows []map[string]any, total int, date, session string, now time.Time) (*foundation.MarketBreadth, error) {
	if len(rows) != total {
		return nil, fmt.Errorf("市场宽度分页不完整")
	}
	b := &foundation.MarketBreadth{Date: date, Source: "东方财富 · 沪深A股完整分页", Coverage: total, Meta: foundation.SourceMeta{Family: "eastmoney", Channel: "http", Source: "eastmoney:breadth", TradeDate: date, FetchedAt: now, AvailableFields: []string{"up", "down", "flat", "coverage"}}}
	seen := map[string]bool{}
	counts := map[int]int{}
	var earliest, latest time.Time
	members := []foundation.BreadthMember{}
	for _, r := range rows {
		code := asString(r["f12"])
		market := int(asFloat(r["f13"]))
		key := fmt.Sprintf("%d.%s", market, code)
		if seen[key] {
			return nil, fmt.Errorf("市场宽度重复证券，拒绝截断样本")
		}
		seen[key] = true
		if len(code) != 6 || !(market == 1 && strings.HasPrefix(code, "6") || market == 0 && (strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3"))) {
			return nil, fmt.Errorf("unexpected breadth security %s", key)
		}
		symbol := code + ".SZ"
		if market == 1 {
			symbol = code + ".SH"
		}
		member := foundation.BreadthMember{Symbol: symbol, ChangeUnit: "CNY"}
		amount, ok := r["f6"].(float64)
		price, pok := r["f2"].(float64)
		if !ok || !pok {
			if r["f6"] == "-" && (r["f2"] == "-" || pok) {
				b.Excluded++
				members = append(members, member)
				continue
			}
			return nil, fmt.Errorf("宽度成交字段缺失")
		}
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
			return nil, fmt.Errorf("invalid breadth amount")
		}
		if amount == 0 {
			b.Excluded++
			members = append(members, member)
			continue
		}
		previous, ok := r["f18"].(float64)
		stamp, tok := r["f124"].(float64)
		if !ok || !tok || price <= 0 || previous <= 0 || math.IsNaN(price) || math.IsInf(price, 0) || math.IsNaN(previous) || math.IsInf(previous, 0) {
			return nil, fmt.Errorf("invalid breadth price/time")
		}
		at := time.Unix(int64(stamp), 0).In(foundation.AStockLocation)
		fresh := false
		switch session {
		case "closed":
			fresh = at.Hour() >= 15
		case "lunch":
			fresh = at.Hour() == 11 && at.Minute() >= 25
		default:
			fresh = now.Sub(at) <= 7*time.Minute
			if now.Hour() == 13 && now.Minute() < 5 {
				fresh = fresh || at.Hour() == 11 && at.Minute() >= 25
			}
		}
		if at.Format("2006-01-02") != date || at.After(now.Add(time.Minute)) || !fresh {
			return nil, fmt.Errorf("市场宽度含跨日或过期行情")
		}
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
		if at.After(latest) {
			latest = at
		}
		counts[market]++
		member.Amount = amount
		member.Change = price - previous
		member.At = at.Format("2006-01-02 15:04:05")
		members = append(members, member)
		if price > previous {
			b.Up++
		} else if price < previous {
			b.Down++
		} else {
			b.Flat++
		}
	}
	if counts[1] < 1500 || counts[0] < 2000 {
		return nil, fmt.Errorf("沪深宽度覆盖不足")
	}
	if session != "closed" && latest.Sub(earliest) > 2*time.Minute {
		return nil, fmt.Errorf("宽度分页时间不同步")
	}
	b.At = earliest.Format("2006-01-02 15:04:05")
	b.LatestAt = latest.Format("2006-01-02 15:04:05")
	foundation.AttachBreadthMembers(b, members)
	return b, nil
}
