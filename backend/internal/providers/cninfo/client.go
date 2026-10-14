// Package cninfo provides an independent official disclosure directory fallback.
// PDF links are original documents; an index entry is never passed off as its body.
package cninfo

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
	mu      sync.Mutex
	orgs    map[string]string
	expires time.Time
}

func NewClient() *Client {
	return &Client{BaseURL: "https://www.cninfo.com.cn", HTTP: &http.Client{Timeout: 12 * time.Second}}
}
func (c *Client) request(ctx context.Context, path string, form url.Values, target any) error {
	method := http.MethodGet
	var body io.Reader
	if form != nil {
		method = http.MethodPost
		body = strings.NewReader(form.Encode())
	}
	req, e := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if e != nil {
		return e
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 EasyStock/0.1")
	req.Header.Set("Referer", c.BaseURL+"/")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r, e := c.HTTP.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("cninfo http status %d", r.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(target)
}
func (c *Client) organization(ctx context.Context, code string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().After(c.expires) {
		var v struct {
			Stocks []struct {
				Code string `json:"code"`
				Org  string `json:"orgId"`
			} `json:"stockList"`
		}
		if e := c.request(ctx, "/new/data/szse_stock.json", nil, &v); e != nil {
			return "", e
		}
		if len(v.Stocks) == 0 {
			return "", fmt.Errorf("cninfo empty security directory")
		}
		next := map[string]string{}
		for _, s := range v.Stocks {
			if s.Code != "" && s.Org != "" {
				next[s.Code] = s.Org
			}
		}
		c.orgs = next
		c.expires = time.Now().Add(24 * time.Hour)
	}
	if id := c.orgs[code]; id != "" {
		return id, nil
	}
	return "", fmt.Errorf("cninfo security not in directory")
}

var tags = regexp.MustCompile(`<[^>]*>`)

func (c *Client) MarketAnnouncements(ctx context.Context, query, symbol, category string, limit int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error) {
	if category != "" && category != "all" {
		return nil, foundation.SourceMeta{}, fmt.Errorf("巨潮备用尚未建立此分类的等价映射")
	}
	limit = max(1, min(limit, 100))
	now := time.Now().In(foundation.AStockLocation)
	form := url.Values{"pageNum": {"1"}, "pageSize": {strconv.Itoa(limit)}, "column": {"szse"}, "tabName": {"fulltext"}, "sortName": {"time"}, "sortType": {"desc"}, "isHLtitle": {"false"}, "searchkey": {strings.TrimSpace(query)}, "seDate": {now.AddDate(-1, 0, 0).Format("2006-01-02") + "~" + now.Format("2006-01-02")}}
	canonical := ""
	if symbol != "" {
		n, e := foundation.NormalizeSymbol(symbol)
		if e != nil {
			return nil, foundation.SourceMeta{}, e
		}
		canonical = n.Canonical
		code := strings.Split(canonical, ".")[0]
		org, e := c.organization(ctx, code)
		if e != nil {
			return nil, foundation.SourceMeta{}, e
		}
		form.Set("stock", code+","+org)
		if strings.HasSuffix(canonical, ".SH") {
			form.Set("column", "sse")
		}
	}
	var payload struct {
		Items []struct {
			ID    string `json:"announcementId"`
			Code  string `json:"secCode"`
			Name  string `json:"secName"`
			Title string `json:"announcementTitle"`
			At    int64  `json:"announcementTime"`
			URL   string `json:"adjunctUrl"`
		} `json:"announcements"`
		Total int `json:"totalAnnouncement"`
	}
	start := time.Now()
	if e := c.request(ctx, "/new/hisAnnouncement/query", form, &payload); e != nil {
		return nil, foundation.SourceMeta{}, e
	}
	meta := foundation.SourceMeta{Family: "cninfo", Channel: "http", Source: "cninfo:announcements", SourceURL: c.BaseURL + "/new/hisAnnouncement/query", FetchedAt: time.Now(), LatencyMS: time.Since(start).Milliseconds(), AvailableFields: []string{"symbol", "title", "published_at", "url"}}
	out := []foundation.MarketResearchItem{}
	seen := map[string]bool{}
	for _, r := range payload.Items {
		n, e := foundation.NormalizeSymbol(r.Code)
		if e != nil || (canonical != "" && n.Canonical != canonical) || seen[r.ID] || r.ID == "" || r.At <= 0 {
			continue
		}
		title := html.UnescapeString(tags.ReplaceAllString(r.Title, ""))
		at := time.UnixMilli(r.At)
		if at.After(time.Now().Add(24*time.Hour)) || title == "" || !strings.HasPrefix(r.URL, "finalpage/") || strings.Contains(r.URL, "..") {
			continue
		}
		seen[r.ID] = true
		out = append(out, foundation.MarketResearchItem{Kind: "announcement", ID: "cninfo:" + r.ID, Symbol: n.Canonical, StockName: r.Name, Title: title, PublishedAt: at, URL: "https://static.cninfo.com.cn/" + r.URL, Meta: meta})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PublishedAt.After(out[j].PublishedAt) })
	if len(payload.Items) > 0 && len(out) == 0 {
		return nil, meta, fmt.Errorf("cninfo response has no matching valid documents")
	}
	return out, meta, nil
}
