package marketpulse

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
)

// A separately authorized Tushare API; some PDF URLs share EastMoney's upstream.
// Such a result is an access-path fallback, not another independent document.
func (c *Client) MarketReports(ctx context.Context, kind, query, symbol, industry string, limit int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error) {
	if kind == "" {
		kind = "stock"
	}
	if kind != "stock" && kind != "industry" {
		return nil, foundation.SourceMeta{}, fmt.Errorf("unsupported report kind")
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 100)
	now := time.Now()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now = now.In(loc)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -45)
	params := map[string]any{"start_date": now.AddDate(0, 0, -45).Format("20060102"), "end_date": now.Format("20060102"), "limit": min(1000, max(100, limit*3)), "report_type": "个股研报"}
	if kind == "industry" {
		params["report_type"] = "行业研报"
	}
	canonical := ""
	if symbol != "" {
		n, e := foundation.NormalizeSymbol(symbol)
		if e != nil {
			return nil, foundation.SourceMeta{}, e
		}
		canonical = n.Canonical
		params["ts_code"] = canonical
	}
	if industry != "" {
		return nil, foundation.SourceMeta{}, fmt.Errorf("Tushare 行业名称尚未与当前行业代码映射")
	}
	rows, err := c.query(ctx, "research_report", params, "trade_date,title,abstr,report_type,author,name,ts_code,inst_csname,ind_name,url", 0)
	meta := foundation.SourceMeta{Family: "tushare", Channel: "http", Source: "tushare:report", SourceURL: "https://tushare.pro/document/2?doc_id=415", FetchedAt: time.Now(), LatencyMS: time.Since(now).Milliseconds(), AvailableFields: []string{"title", "kind", "symbol", "organization", "researchers", "published_at", "url", "content"}, FallbackReason: "Tushare 研报元数据/摘要；需独立权限，PDF可能仍来自东方财富，不计作独立原文证据；未提供评级/目标价/EPS/PE"}
	if err != nil {
		return nil, meta, err
	}
	text := func(r Row, k string) string { v, _ := r[k].(string); return strings.TrimSpace(v) }
	items := []foundation.MarketResearchItem{}
	seen := map[string]bool{}
	for _, r := range rows {
		at, e := time.ParseInLocation("20060102", text(r, "trade_date"), loc)
		if e != nil || at.After(now) || at.Before(cutoff) {
			continue
		}
		title, link := text(r, "title"), text(r, "url")
		u, e := url.Parse(link)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || title == "" {
			continue
		}
		if text(r, "report_type") != params["report_type"] {
			continue
		}
		sym := text(r, "ts_code")
		if sym != "" {
			if n, e := foundation.NormalizeSymbol(sym); e == nil {
				sym = n.Canonical
			} else {
				sym = ""
			}
		}
		if canonical != "" && sym != canonical {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{title, sym, text(r, "name"), text(r, "author"), text(r, "inst_csname")}, " "))
		if q := strings.ToLower(strings.TrimSpace(query)); q != "" && !strings.Contains(haystack, q) {
			continue
		}
		identity := u.Host + u.Path + at.Format("2006-01-02")
		if seen[identity] {
			continue
		}
		seen[identity] = true
		hash := sha256.Sum256([]byte(identity))
		items = append(items, foundation.MarketResearchItem{ID: fmt.Sprintf("tushare:%x", hash[:12]), Kind: kind, Title: title, Content: text(r, "abstr"), Symbol: sym, StockName: text(r, "name"), Organization: text(r, "inst_csname"), Researchers: text(r, "author"), IndustryName: text(r, "ind_name"), PublishedAt: at, URL: link, Meta: meta})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].PublishedAt.After(items[j].PublishedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, meta, nil
}
