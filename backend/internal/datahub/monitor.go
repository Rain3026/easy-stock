package datahub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Observation struct {
	Count     int       `json:"count"`
	DataAt    time.Time `json:"data_at,omitempty"`
	TradeDate string    `json:"trade_date,omitempty"`
	Freshness string    `json:"freshness"`
	Stale     bool      `json:"stale"`
	Truncated bool      `json:"truncated"`
	Detail    string    `json:"detail,omitempty"`
}
type Probe struct {
	SourceID   string
	Capability string
	Run        func(context.Context) (Observation, error)
}
type Health struct {
	Source
	OK            bool         `json:"ok"`
	Status        string       `json:"status"`
	Message       string       `json:"message"`
	CheckedAt     *time.Time   `json:"checked_at,omitempty"`
	LastSuccessAt *time.Time   `json:"last_success_at,omitempty"`
	LatencyMS     int64        `json:"latency_ms"`
	Capability    string       `json:"capability,omitempty"`
	Observation   *Observation `json:"observation,omitempty"`
}

type Monitor struct {
	mu          sync.Mutex
	probes      []Probe
	values      map[string]Health
	refreshing  bool
	lastStarted time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	path        string
	now         func() time.Time
}

func NewMonitor(probes []Probe, path string) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Monitor{probes: probes, values: map[string]Health{}, ctx: ctx, cancel: cancel, path: path, now: time.Now}
	for _, s := range Sources() {
		m.values[s.ID] = Health{Source: s, Status: "unknown", Message: "尚未进行样本检测"}
	}
	var saved []Health
	if b, e := os.ReadFile(path); e == nil && json.Unmarshal(b, &saved) == nil {
		for _, h := range saved {
			if s, ok := m.values[h.ID]; ok {
				h.Source = s.Source
				m.values[h.ID] = h
			}
		}
	}
	return m
}
func (m *Monitor) Close() { m.mu.Lock(); m.cancel(); m.mu.Unlock(); m.wg.Wait() }
func (m *Monitor) Snapshot() ([]Health, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Health, 0, len(m.values))
	for _, s := range Sources() {
		h := m.values[s.ID]
		if h.CheckedAt != nil && m.now().Sub(*h.CheckedAt) > 10*time.Minute {
			h.OK = false
			h.Status = "expired"
			h.Message = "检测记录已过期，请重新检测"
		}
		out = append(out, h)
	}
	return out, m.refreshing
}

// Refresh is bounded and deduplicated. Merely viewing health never makes remote calls.
func (m *Monitor) Refresh() bool {
	m.mu.Lock()
	if m.ctx.Err() != nil || m.refreshing || (!m.lastStarted.IsZero() && m.now().Sub(m.lastStarted) < 15*time.Second) {
		m.mu.Unlock()
		return false
	}
	m.refreshing = true
	m.lastStarted = m.now()
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		sem := make(chan struct{}, 3)
		var group sync.WaitGroup
		for _, p := range m.probes {
			p := p
			group.Add(1)
			go func() {
				defer group.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}
				start := m.now()
				c, done := context.WithTimeout(ctx, 10*time.Second)
				ob, err := p.Run(c)
				if err != nil && c.Err() != nil {
					err = c.Err()
				}
				done()
				end := m.now()
				status, msg := classify(ob, err)
				m.mu.Lock()
				h, ok := m.values[p.SourceID]
				if ok {
					h.Status = status
					h.OK = status == "available"
					h.Message = msg
					h.CheckedAt = &end
					h.LatencyMS = end.Sub(start).Milliseconds()
					h.Capability = p.Capability
					h.Observation = nil
					if err == nil {
						h.Observation = &ob
					}
					if h.OK {
						h.LastSuccessAt = &end
					}
					m.values[p.SourceID] = h
				}
				m.mu.Unlock()
			}()
		}
		group.Wait()
		values, _ := m.Snapshot()
		if m.path != "" {
			if b, e := json.MarshalIndent(values, "", "  "); e == nil {
				if os.MkdirAll(filepath.Dir(m.path), 0700) == nil {
					tmp := m.path + ".tmp"
					if os.WriteFile(tmp, b, 0600) == nil {
						_ = os.Rename(tmp, m.path)
					}
				}
			}
		}
		m.mu.Lock()
		m.refreshing = false
		m.mu.Unlock()
	}()
	return true
}

func classify(ob Observation, err error) (string, string) {
	if err != nil {
		s := strings.ToLower(err.Error())
		switch {
		case errors.Is(err, context.DeadlineExceeded) || strings.Contains(s, "timeout") || strings.Contains(s, "deadline"):
			return "timeout", "样本请求超时"
		case strings.Contains(s, "未配置") || strings.Contains(s, "请在系统设置"):
			return "unconfigured", "尚未配置所需凭据"
		case strings.Contains(s, "429") || strings.Contains(s, "频次") || strings.Contains(s, "限流") || strings.Contains(s, "rate limit"):
			return "rate_limited", "达到来源调用频率限制"
		case strings.Contains(s, "401") || strings.Contains(s, "403") || strings.Contains(s, "权限") || strings.Contains(s, "token") || strings.Contains(s, "积分"):
			return "permission_denied", "凭据或接口权限不满足"
		case strings.Contains(s, "不完整") || strings.Contains(s, "truncat"):
			return "partial", "样本响应不完整"
		default:
			return "error", "样本请求或数据解析失败" // Never return upstream bodies or credential-bearing URLs.
		}
	}
	if ob.Count <= 0 {
		return "empty", "未返回有效样本，不能判定来源可用"
	}
	if ob.Stale {
		return "stale", "返回了过期数据"
	}
	if ob.Truncated {
		return "partial", "返回数据被截断"
	}
	if ob.Freshness != "verified" {
		return "partial", "样本可读取，数据时效尚未核实"
	}
	return "available", "当前检测能力样本可用；不代表全部接口已验收"
}
