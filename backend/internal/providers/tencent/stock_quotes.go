package tencent

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (c *Client) Realtime(ctx context.Context, symbols []string) ([]foundation.Quote, error) {
	keys := []string{}
	canonical := map[string]string{}
	for _, symbol := range symbols {
		n, e := foundation.NormalizeSymbol(symbol)
		if e != nil {
			return nil, e
		}
		if _, ok := canonical[n.Sina]; !ok {
			keys = append(keys, n.Sina)
			canonical[n.Sina] = n.Canonical
		}
	}
	if len(keys) == 0 || len(keys) > 80 {
		return nil, fmt.Errorf("quote request must contain 1 to 80 symbols")
	}
	u := c.quoteBaseURL + "?" + url.Values{"q": {strings.Join(keys, ",")}}.Encode()
	start := time.Now()
	body, e := c.get(ctx, u)
	if e != nil {
		return nil, e
	}
	parsed := parseTencentQuoteLines(body)
	out := []foundation.Quote{}
	for _, key := range keys {
		f := parsed[key]
		if len(f) < 38 || fieldAt(f, 2) != strings.Split(canonical[key], ".")[0] {
			continue
		}
		at := parseTencentTradeTime(fieldAt(f, 30))
		if at.IsZero() {
			continue
		}
		out = append(out, foundation.Quote{Symbol: canonical[key], Name: fieldAt(f, 1), Price: parseFloat(fieldAt(f, 3)), PreviousClose: parseFloat(fieldAt(f, 4)), Open: parseFloat(fieldAt(f, 5)), Volume: parseFloat(fieldAt(f, 6)) * 100, Amount: parseFloat(fieldAt(f, 37)) * 10000, TradeTime: at, Change: parseFloat(fieldAt(f, 31)), ChangePercent: parseFloat(fieldAt(f, 32)), High: parseFloat(fieldAt(f, 33)), Low: parseFloat(fieldAt(f, 34)), Meta: foundation.SourceMeta{Family: "tencent", Channel: "http", Source: "tencent:quote", SourceURL: u, FetchedAt: time.Now(), LatencyMS: time.Since(start).Milliseconds(), AmountUnit: "CNY", VolumeUnit: "shares", Adjustment: "none"}})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tencent returned no quotes")
	}
	return out, nil
}

// KLine accepts only explicit forward-adjusted stock bars. Unadjusted index
// bars are allowed only for known index identifiers, never as a stock fallback.
func (c *Client) KLine(ctx context.Context, symbol, period string, limit int) ([]foundation.KLine, error) {
	n, e := foundation.NormalizeSymbol(symbol)
	if e != nil {
		return nil, e
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
		return nil, fmt.Errorf("Tencent adjusted fallback supports day/week/month only")
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 640 {
		limit = 640
	}
	knownIndex := false
	for _, idx := range indexCatalog {
		if idx.KLineKey == n.Sina {
			knownIndex = true
			break
		}
	}
	if period == "day" && !knownIndex {
		return c.AdjustedKLine(ctx, n.Canonical, limit)
	}
	u := c.klineBaseURL + "?" + url.Values{"param": {fmt.Sprintf("%s,%s,,,%d,qfq", n.Sina, period, limit)}}.Encode()
	var payload struct {
		Code int                       `json:"code"`
		Data map[string]map[string]any `json:"data"`
	}
	if e = c.getJSON(ctx, u, &payload); e != nil {
		return nil, e
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("Tencent kline rejected")
	}
	raw := payload.Data[n.Sina]
	values, ok := raw["qfq"+period].([]any)
	adjustment := "qfq"
	if !ok {
		for _, idx := range indexCatalog {
			if idx.KLineKey == n.Sina {
				values, ok = raw[period].([]any)
				adjustment = "none"
				break
			}
		}
	}
	if !ok || len(values) == 0 {
		return nil, fmt.Errorf("missing explicit adjusted kline; refusing unadjusted substitution")
	}
	out := []foundation.KLine{}
	meta := foundation.SourceMeta{Family: "tencent", Channel: "http", Source: "tencent:adjusted-kline", SourceURL: u, FetchedAt: time.Now(), VolumeUnit: "lots", Adjustment: adjustment, AvailableFields: []string{"open", "high", "low", "close", "volume"}}
	for _, v := range values {
		r, ok := v.([]any)
		if !ok || len(r) < 6 {
			continue
		}
		at, e := time.ParseInLocation("2006-01-02", anyString(r[0]), foundation.AStockLocation)
		if e != nil {
			continue
		}
		close := parseFloat(anyString(r[2]))
		if close <= 0 {
			continue
		}
		out = append(out, foundation.KLine{Symbol: n.Canonical, Time: at, Open: parseFloat(anyString(r[1])), Close: close, High: parseFloat(anyString(r[3])), Low: parseFloat(anyString(r[4])), Volume: parseFloat(anyString(r[5])), Meta: meta})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Tencent adjusted kline has no valid bars")
	}
	return out, nil
}
