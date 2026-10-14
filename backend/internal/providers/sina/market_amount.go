package sina

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
)

type amountCacheEntry struct {
	bars []foundation.KLine
	at   time.Time
}

// MarketAmountBars uses the broad A-share indexes, never Shenzhen Component.
// Sina's ordinary daily endpoint has no amount; only complete 48-bucket days
// from its real 5-minute amount history may form a daily observation.
func (c *Client) MarketAmountBars(ctx context.Context, symbol, period string, limit int) ([]foundation.KLine, error) {
	if (symbol != "000002.SH" && symbol != "399107.SZ") || (period != "day" && period != "5") || limit < 1 || limit > 1023 {
		return nil, fmt.Errorf("unsupported Sina market amount request")
	}
	c.amountMu.Lock()
	entry, ok := c.amountCache[symbol]
	c.amountMu.Unlock()
	if !ok || time.Since(entry.at) >= 30*time.Second {
		bars, err := c.fetchMarketAmounts(ctx, symbol)
		if err != nil {
			return nil, err
		}
		entry = amountCacheEntry{bars: bars, at: time.Now()}
		c.amountMu.Lock()
		if c.amountCache == nil {
			c.amountCache = map[string]amountCacheEntry{}
		}
		c.amountCache[symbol] = entry
		c.amountMu.Unlock()
	}
	bars := entry.bars
	if period == "day" {
		bars = completeAmountDays(bars)
	}
	if len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("Sina market amount has no complete %s bars", period)
	}
	return append([]foundation.KLine(nil), bars...), nil
}

func (c *Client) fetchMarketAmounts(ctx context.Context, symbol string) ([]foundation.KLine, error) {
	n, err := foundation.NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	u := c.kLineBaseURL + "?" + url.Values{"symbol": {n.Sina}, "scale": {"5"}, "ma": {"no"}, "datalen": {"1023"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 easy-stock/0.1")
	req.Header.Set("Referer", "https://finance.sina.com.cn/")
	start := time.Now()
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Sina market amount request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Sina market amount HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	meta := foundation.SourceMeta{Source: "sina:index-amount", Family: "sina", Channel: "http", SourceURL: u, FetchedAt: now, LatencyMS: time.Since(start).Milliseconds(), AmountUnit: "CNY", Adjustment: "none", AvailableFields: []string{"amount"}}
	return parseMarketAmounts(b, symbol, meta, now)
}

func parseMarketAmounts(body []byte, symbol string, meta foundation.SourceMeta, now time.Time) ([]foundation.KLine, error) {
	s := strings.TrimSpace(string(body))
	left, right := strings.Index(s, "["), strings.LastIndex(s, "]")
	if left < 0 || right < left {
		return nil, fmt.Errorf("Sina market amount has no array")
	}
	var rows []struct {
		Day    string          `json:"day"`
		Amount json.RawMessage `json:"amount"`
	}
	if err := json.Unmarshal([]byte(s[left:right+1]), &rows); err != nil {
		return nil, fmt.Errorf("invalid Sina market amount payload")
	}
	seen := map[time.Time]float64{}
	bars := make([]foundation.KLine, 0, len(rows))
	for _, row := range rows {
		at, err := time.ParseInLocation("2006-01-02 15:04:05", row.Day, foundation.AStockLocation)
		if err != nil {
			return nil, fmt.Errorf("invalid Sina amount timestamp")
		}
		// The endpoint includes the currently forming bar (e.g. 11:30 at 11:25).
		if at.After(now) {
			continue
		}
		m := at.Hour()*60 + at.Minute()
		if at.Second() != 0 || m%5 != 0 || m < 575 || m > 900 || (m > 690 && m <= 780) || !foundation.IsAStockTradingDay(at) {
			return nil, fmt.Errorf("unexpected Sina amount bucket %s", row.Day)
		}
		raw := strings.Trim(string(row.Amount), "\"")
		amount, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
			return nil, fmt.Errorf("missing or invalid Sina amount at %s", row.Day)
		}
		if value, exists := seen[at]; exists {
			if value != amount {
				return nil, fmt.Errorf("conflicting Sina amount bucket %s", row.Day)
			}
			continue
		}
		seen[at] = amount
		bars = append(bars, foundation.KLine{Symbol: symbol, Time: at, Amount: amount, Meta: meta})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Time.Before(bars[j].Time) })
	return bars, nil
}

func completeAmountDays(bars []foundation.KLine) []foundation.KLine {
	byDay := map[string][]foundation.KLine{}
	for _, b := range bars {
		day := b.Time.In(foundation.AStockLocation).Format("2006-01-02")
		byDay[day] = append(byDay[day], b)
	}
	days := []foundation.KLine{}
	for _, group := range byDay {
		// Parser guarantees unique timestamps on the regular 5-minute grid.
		if len(group) != 48 {
			continue
		}
		b := group[len(group)-1]
		b.Amount = 0
		for _, minute := range group {
			b.Amount += minute.Amount
		}
		days = append(days, b)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Time.Before(days[j].Time) })
	return days
}
