package sina

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
	"golang.org/x/net/html"
)

func WithReportBaseURL(base string) Option {
	return func(c *Client) { c.reportBaseURL = strings.TrimRight(base, "/") }
}

// Public report metadata is a fallback catalogue, not a complete broker PDF feed.
func (c *Client) MarketReports(ctx context.Context, kind, query, symbol, industry string, limit int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error) {
	if kind == "" {
		kind = "stock"
	}
	if kind != "stock" && kind != "industry" {
		return nil, foundation.SourceMeta{}, fmt.Errorf("unsupported report kind")
	}
	if industry != "" {
		return nil, foundation.SourceMeta{}, fmt.Errorf("新浪行业分类尚未与当前行业筛选映射，不能静默替换")
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 100)
	path := "/stock/go.php/vReport_List/kind/company/index.phtml"
	if kind == "industry" {
		path = "/stock/go.php/vReport_List/kind/industry/index.phtml"
	}
	params := url.Values{}
	canonical := ""
	if symbol != "" {
		if kind != "stock" {
			return nil, foundation.SourceMeta{}, fmt.Errorf("行业研报不接受个股筛选")
		}
		n, err := foundation.NormalizeSymbol(symbol)
		if err != nil {
			return nil, foundation.SourceMeta{}, err
		}
		canonical = n.Canonical
		path = "/stock/go.php/vReport_List/kind/search/index.phtml"
		params.Set("symbol", n.Sina)
		params.Set("t1", "2")
	}
	start := time.Now()
	day := start.In(foundation.AStockLocation)
	cutoff := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, foundation.AStockLocation).AddDate(0, 0, -45)
	meta := foundation.SourceMeta{Family: "sina", Channel: "http", Source: "sina:report", AvailableFields: []string{"title", "kind", "symbol", "organization", "researchers", "published_at", "url"}, FallbackReason: "新浪公开研报目录；不提供东财评级、目标价、EPS/PE或完整PDF，单次最多扫描120条"}
	items := []foundation.MarketResearchItem{}
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		params.Set("p", strconv.Itoa(page))
		endpoint := c.reportBaseURL + path + "?" + params.Encode()
		if page == 1 {
			meta.SourceURL = endpoint
		}
		body, err := c.publicText(ctx, endpoint, 2<<20)
		if err != nil {
			return nil, meta, err
		}
		rows, err := parseReportPage(body, kind)
		if err != nil {
			return nil, meta, err
		}
		newRows := 0
		oldest := time.Now()
		for _, row := range rows {
			if seen[row.ID] {
				continue
			}
			seen[row.ID] = true
			newRows++
			if row.PublishedAt.Before(oldest) {
				oldest = row.PublishedAt
			}
			if row.PublishedAt.After(start) || row.PublishedAt.Before(cutoff) {
				continue
			}
			if canonical != "" && row.Symbol != canonical {
				continue
			}
			haystack := strings.ToLower(strings.Join([]string{row.Title, row.Symbol, row.Organization, row.Researchers}, " "))
			if q := strings.ToLower(strings.TrimSpace(query)); q != "" && !strings.Contains(haystack, q) {
				continue
			}
			items = append(items, row)
		}
		if len(items) >= limit || len(rows) < 40 || newRows == 0 || oldest.Before(cutoff) {
			break
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].PublishedAt.After(items[j].PublishedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	meta.FetchedAt = time.Now()
	meta.LatencyMS = time.Since(start).Milliseconds()
	for i := range items {
		items[i].Meta = meta
	}
	return items, meta, nil
}

func (c *Client) publicText(ctx context.Context, endpoint string, maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", "https://finance.sina.com.cn/")
	req.Header.Set("User-Agent", "Mozilla/5.0 easy-stock/0.1")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sina HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("sina response too large")
	}
	return decodeSinaBody(body, resp.Header.Get("Content-Type")), nil
}

var reportIDPattern = regexp.MustCompile(`/rptid/([0-9]+)/`)
var reportStockPattern = regexp.MustCompile(`[（(]([0-9]{6})[)）]`)

func parseReportPage(body, kind string) ([]foundation.MarketResearchItem, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	var table *html.Node
	walkHTML(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" && strings.Contains(" "+attrHTML(n, "class")+" ", " tb_01 ") {
			table = n
		}
	})
	if table == nil {
		return nil, fmt.Errorf("新浪研报目录结构不可识别")
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	out := []foundation.MarketResearchItem{}
	walkHTML(table, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "tr" {
			return
		}
		cells := []*html.Node{}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "td" {
				cells = append(cells, c)
			}
		}
		if len(cells) != 6 {
			return
		}
		var link *html.Node
		walkHTML(cells[1], func(a *html.Node) {
			if a.Data == "a" && reportIDPattern.MatchString(attrHTML(a, "href")) {
				link = a
			}
		})
		if link == nil {
			return
		}
		u, e := url.Parse(attrHTML(link, "href"))
		if e != nil || (u.Host != "" && u.Host != "stock.finance.sina.com.cn") {
			return
		}
		id := reportIDPattern.FindStringSubmatch(u.Path)[1]
		title := strings.TrimSpace(attrHTML(link, "title"))
		if title == "" {
			title = textHTML(link)
		}
		at, e := time.ParseInLocation("2006-01-02", textHTML(cells[3]), loc)
		if e != nil || title == "" {
			return
		}
		category := textHTML(cells[2])
		if kind == "stock" && category != "公司" {
			return
		}
		if kind == "industry" && category != "行业" {
			return
		}
		row := foundation.MarketResearchItem{ID: "sina:" + id, Kind: kind, Title: title, Category: category, Organization: textHTML(cells[4]), Researchers: textHTML(cells[5]), PublishedAt: at, URL: "https://stock.finance.sina.com.cn/stock/go.php/vReport_Show/kind/search/rptid/" + id + "/index.phtml"}
		if m := reportStockPattern.FindStringSubmatch(title); len(m) == 2 {
			if s, e := foundation.NormalizeSymbol(m[1]); e == nil {
				row.Symbol = s.Canonical
			}
			row.StockName = strings.TrimSpace(strings.Split(strings.Split(title, "(")[0], "（")[0])
		}
		out = append(out, row)
	})
	return out, nil
}
func attrHTML(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func textHTML(n *html.Node) string {
	var b strings.Builder
	walkHTML(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	})
	return strings.Join(strings.Fields(b.String()), " ")
}
func walkHTML(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTML(c, f)
	}
}
