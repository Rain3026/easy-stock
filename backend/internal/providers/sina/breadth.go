package sina

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy-stock/backend/internal/foundation"
)

func WithBreadthBaseURL(u string) Option { return func(c *Client) { c.breadthBaseURL = u } }

func (c *Client) breadthBody(ctx context.Context, u string) (string, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("Referer", "https://finance.sina.com.cn/")
	req.Header.Set("User-Agent", "Mozilla/5.0 easy-stock/0.1")
	res, e := c.httpClient.Do(req)
	if e != nil {
		return "", e
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Sina breadth HTTP %d at %s page=%s", res.StatusCode, req.URL.Path, req.URL.Query().Get("page"))
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if e != nil {
		return "", e
	}
	return decodeSinaBody(b, res.Header.Get("Content-Type")), nil
}

// A bounded four-worker traversal, never retries partial or failed pages.
func breadthParallel(ctx context.Context, n int, fetch func(int) error) error {
	errs := make([]error, n)
	jobs := make(chan int, n)
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					errs[i] = ctx.Err()
				} else {
					errs[i] = fetch(i)
				}
			}
		}()
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func (c *Client) breadthSymbols(ctx context.Context) ([]string, int, error) {
	// The general list omits STAR-market CDRs. Union with the complete STAR
	// list, not a hard-coded security or a quote from another provider.
	all, excluded, e := c.breadthNodeSymbols(ctx, "hs_a")
	if e != nil {
		return nil, 0, e
	}
	star, _, e := c.breadthNodeSymbols(ctx, "kcb")
	if e != nil {
		return nil, 0, e
	}
	seen := map[string]bool{}
	for _, s := range all {
		seen[s] = true
	}
	for _, s := range star {
		if !seen[s] {
			all = append(all, s)
			seen[s] = true
		}
	}
	return all, excluded, nil
}

func (c *Client) breadthNodeSymbols(ctx context.Context, node string) ([]string, int, error) {
	count := func() (int, error) {
		body, e := c.breadthBody(ctx, c.breadthBaseURL+"getHQNodeStockCount?node="+node)
		if e != nil {
			return 0, e
		}
		n, e := strconv.Atoi(strings.Trim(strings.TrimSpace(body), "\""))
		if e != nil || n < 1 || n > 10000 {
			return 0, fmt.Errorf("invalid Sina breadth universe count")
		}
		return n, nil
	}
	total, e := count()
	if e != nil {
		return nil, 0, e
	}
	pages := make([][]struct {
		Symbol string `json:"symbol"`
	}, (total+79)/80)
	e = breadthParallel(ctx, len(pages), func(i int) error {
		p := url.Values{"page": {strconv.Itoa(i + 1)}, "num": {"80"}, "sort": {"symbol"}, "asc": {"1"}, "node": {node}, "symbol": {""}, "_s_r_a": {"page"}}
		body, e := c.breadthBody(ctx, c.breadthBaseURL+"getHQNodeData?"+p.Encode())
		if e != nil {
			return e
		}
		if e = json.Unmarshal([]byte(body), &pages[i]); e != nil {
			return fmt.Errorf("invalid Sina breadth list page")
		}
		expected := 80
		if i == len(pages)-1 {
			expected = total - i*80
		}
		if len(pages[i]) != expected {
			return fmt.Errorf("Sina breadth %s page %d incomplete: expected %d got %d", node, i+1, expected, len(pages[i]))
		}
		return nil
	})
	if e != nil {
		return nil, 0, e
	}
	end, e := count()
	if e != nil {
		return nil, 0, e
	}
	if end != total {
		return nil, 0, fmt.Errorf("Sina breadth universe changed during pagination")
	}
	seen := map[string]bool{}
	symbols := []string{}
	excluded := 0
	for _, page := range pages {
		for _, row := range page {
			s := row.Symbol
			if seen[s] {
				return nil, 0, fmt.Errorf("Sina breadth duplicate symbol")
			}
			seen[s] = true
			if len(s) != 8 {
				return nil, 0, fmt.Errorf("invalid Sina breadth symbol")
			}
			if strings.HasPrefix(s, "bj") {
				excluded++
				continue
			}
			canonical := s[2:] + "." + strings.ToUpper(s[:2])
			if !foundation.IsSHSZAShare(canonical) {
				return nil, 0, fmt.Errorf("unexpected Sina breadth universe")
			}
			symbols = append(symbols, canonical)
		}
	}
	return symbols, excluded, nil
}

func parseBreadthQuotes(body string, symbols []string) ([]foundation.BreadthMember, error) {
	want := map[string]bool{}
	for _, s := range symbols {
		want[s] = true
	}
	seen := map[string]bool{}
	out := []foundation.BreadthMember{}
	for _, line := range strings.Split(body, ";") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		left, payload, ok := strings.Cut(line, "=\"")
		if !ok {
			return nil, fmt.Errorf("invalid Sina breadth quote payload")
		}
		code := strings.TrimPrefix(left, "var hq_str_")
		if len(code) != 8 {
			return nil, fmt.Errorf("invalid Sina breadth quote symbol")
		}
		symbol := code[2:] + "." + strings.ToUpper(code[:2])
		if !want[symbol] || seen[symbol] {
			return nil, fmt.Errorf("unexpected or duplicate Sina breadth quote")
		}
		seen[symbol] = true
		fields := strings.Split(strings.TrimSuffix(payload, "\""), ",")
		if len(fields) < 32 || fields[0] == "" {
			return nil, fmt.Errorf("missing Sina breadth quote %s", symbol)
		}
		amount, e := strconv.ParseFloat(fields[9], 64)
		if e != nil || !finiteNonnegative(amount) {
			return nil, fmt.Errorf("missing or invalid Sina breadth amount")
		}
		m := foundation.BreadthMember{Symbol: symbol, Amount: amount, ChangeUnit: "CNY"}
		if amount > 0 {
			price, pe := strconv.ParseFloat(fields[3], 64)
			prior, qe := strconv.ParseFloat(fields[2], 64)
			if pe != nil || qe != nil || !finiteNonnegative(price) || !finiteNonnegative(prior) || price == 0 || prior == 0 {
				return nil, fmt.Errorf("invalid Sina breadth traded price")
			}
			m.Change = price - prior
			m.At = fields[30] + " " + fields[31]
		}
		out = append(out, m)
	}
	if len(out) != len(symbols) {
		return nil, fmt.Errorf("Sina breadth quotes incomplete")
	}
	return out, nil
}

func finiteNonnegative(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }

func aggregateSinaBreadth(members []foundation.BreadthMember, date, session string, now time.Time) (*foundation.MarketBreadth, error) {
	now = now.In(foundation.AStockLocation)
	b := &foundation.MarketBreadth{Date: date, Source: "新浪 · 沪深A股完整名单（含科创板CDR）逐批行情", Meta: foundation.SourceMeta{Source: "sina:breadth", Family: "sina", Channel: "http", FetchedAt: now, TradeDate: date, AvailableFields: []string{"up", "down", "flat", "coverage"}}}
	counts := map[string]int{}
	seen := map[string]bool{}
	var earliest, latest time.Time
	for _, m := range members {
		if !foundation.IsSHSZAShare(m.Symbol) || seen[m.Symbol] || !finiteNonnegative(m.Amount) {
			return nil, fmt.Errorf("invalid or duplicate Sina breadth member")
		}
		seen[m.Symbol] = true
		if m.Amount == 0 {
			continue
		}
		at, e := time.ParseInLocation("2006-01-02 15:04:05", m.At, foundation.AStockLocation)
		fresh := now.Sub(at) <= 7*time.Minute
		if session == "closed" {
			fresh = at.Hour() >= 15
		} else if session == "lunch" || now.Hour() == 13 && now.Minute() < 5 {
			fresh = fresh || at.Hour() == 11 && at.Minute() >= 25
		}
		if e != nil || at.Format("2006-01-02") != date || at.After(now.Add(time.Minute)) || !fresh || math.IsNaN(m.Change) || math.IsInf(m.Change, 0) {
			return nil, fmt.Errorf("Sina breadth stale, cross-date or invalid quote")
		}
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
		if at.After(latest) {
			latest = at
		}
		counts[m.Symbol[7:]]++
		if m.Change > 0 {
			b.Up++
		} else if m.Change < 0 {
			b.Down++
		} else {
			b.Flat++
		}
	}
	if counts["SH"] < 1500 || counts["SZ"] < 2000 {
		return nil, fmt.Errorf("Sina breadth exchange coverage insufficient")
	}
	if session != "closed" && latest.Sub(earliest) > 2*time.Minute {
		return nil, fmt.Errorf("Sina breadth quote batches not synchronized")
	}
	b.At = earliest.Format("2006-01-02 15:04:05")
	b.LatestAt = latest.Format("2006-01-02 15:04:05")
	foundation.AttachBreadthMembers(b, members)
	return b, nil
}

func (c *Client) MarketBreadth(ctx context.Context, date, session string, now time.Time) (*foundation.MarketBreadth, error) {
	start := time.Now()
	symbols, excluded, e := c.breadthSymbols(ctx)
	if e != nil {
		return nil, e
	}
	batches := make([][]foundation.BreadthMember, (len(symbols)+79)/80)
	e = breadthParallel(ctx, len(batches), func(i int) error {
		end := min((i+1)*80, len(symbols))
		wanted := symbols[i*80 : end]
		codes := []string{}
		for _, s := range wanted {
			codes = append(codes, strings.ToLower(s[7:])+s[:6])
		}
		body, e := c.breadthBody(ctx, c.requestURL(url.Values{"rn": {strconv.FormatInt(time.Now().UnixMilli(), 10)}, "list": {strings.Join(codes, ",")}}))
		if e != nil {
			return e
		}
		batches[i], e = parseBreadthQuotes(body, wanted)
		return e
	})
	if e != nil {
		return nil, e
	}
	members := []foundation.BreadthMember{}
	for _, batch := range batches {
		members = append(members, batch...)
	}
	b, e := aggregateSinaBreadth(members, date, session, now)
	if e != nil {
		return nil, e
	}
	b.OtherMarketsExcluded = excluded
	b.Meta.SourceURL = "https://vip.stock.finance.sina.com.cn/mkt/"
	b.Meta.LatencyMS = time.Since(start).Milliseconds()
	return b, nil
}
