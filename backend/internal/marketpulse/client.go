package marketpulse

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Row map[string]any
type cacheEntry struct {
	Rows []Row     `json:"rows"`
	At   time.Time `json:"at"`
}
type Client struct {
	Fallback          IndexAmountProvider
	SecondaryFallback IndexAmountProvider
	BreadthFallback   BreadthProvider
	BreadthSecondary  BreadthProvider
	HTTP              *http.Client
	URL               string
	Token             func() string
	CacheDir          string
	mu                sync.Mutex
	cache             map[string]cacheEntry
	validationOnly    bool // One isolated capture retains its first successful response per key.
}

func New(token func() string, cacheDir string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 18 * time.Second}, URL: "https://api.tushare.pro", Token: token, CacheDir: cacheDir, cache: map[string]cacheEntry{}}
}
func (c *Client) query(ctx context.Context, api string, params map[string]any, fields string, ttl time.Duration) ([]Row, error) {
	token := strings.TrimSpace(c.Token())
	if token == "" {
		return nil, fmt.Errorf("请在系统设置的数据源中配置 Tushare Token")
	}
	keyData, _ := json.Marshal([]any{api, params, fields})
	key := fmt.Sprintf("%x", sha256.Sum256(keyData))
	c.mu.Lock()
	entry, ok := c.cache[key]
	c.mu.Unlock()
	if !ok && c.CacheDir != "" {
		b, e := os.ReadFile(filepath.Join(c.CacheDir, key+".json"))
		if e == nil && json.Unmarshal(b, &entry) == nil {
			ok = true
		}
	}
	if ok && (c.validationOnly || time.Since(entry.At) < ttl) {
		return entry.Rows, nil
	}
	body, _ := json.Marshal(map[string]any{"api_name": api, "token": token, "params": params, "fields": fields})
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := c.HTTP.Do(req)
	if e != nil {
		return nil, fmt.Errorf("%s 请求失败", api)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s HTTP %d", api, res.StatusCode)
	}
	var p struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Fields  []string `json:"fields"`
			Items   [][]any  `json:"items"`
			HasMore bool     `json:"has_more"`
		} `json:"data"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 12<<20)).Decode(&p); e != nil {
		return nil, fmt.Errorf("%s 响应无法解析", api)
	}
	if p.Code != 0 {
		return nil, fmt.Errorf("%s: %s", api, strings.ReplaceAll(p.Msg, token, "[redacted]"))
	}
	if p.Data.HasMore {
		return nil, fmt.Errorf("%s 返回不完整，请缩小查询范围", api)
	}
	out := make([]Row, 0, len(p.Data.Items))
	for _, item := range p.Data.Items {
		r := Row{}
		for i, f := range p.Data.Fields {
			if i < len(item) {
				r[f] = item[i]
			}
		}
		out = append(out, r)
	}
	if len(out) > 0 && ttl > 0 {
		entry = cacheEntry{Rows: out, At: time.Now()}
		c.mu.Lock()
		c.cache[key] = entry
		c.mu.Unlock()
		if c.CacheDir != "" {
			if os.MkdirAll(c.CacheDir, 0700) == nil {
				if b, e := json.Marshal(entry); e == nil {
					f, e := os.CreateTemp(c.CacheDir, ".pulse-")
					if e == nil {
						_, e = f.Write(b)
						f.Close()
						if e == nil {
							_ = os.Rename(f.Name(), filepath.Join(c.CacheDir, key+".json"))
						}
						_ = os.Remove(f.Name())
					}
				}
			}
		}
	}
	return out, nil
}
func num(r Row, k string) float64 { v, _ := r[k].(float64); return v }
func str(r Row, k string) string  { v, _ := r[k].(string); return v }
func (c *Client) daily(ctx context.Context, api, code, start, end string) ([]Row, error) {
	return c.query(ctx, api, map[string]any{"ts_code": code, "start_date": start, "end_date": end}, "", 30*time.Minute)
}
