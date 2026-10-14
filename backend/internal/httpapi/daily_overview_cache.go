package httpapi

import (
	"context"
	"easy-stock/backend/internal/marketpulse"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Only public observations are cached. An expired response retains its original
// timestamps and cannot silently become a fresh market judgement.
type overviewCache struct {
	mu      sync.Mutex
	data    *dailyOverviewData
	fetched time.Time
	running bool
	done    chan struct{}
	file    string
}
type cachedOverview struct {
	Data    dailyOverviewData `json:"data"`
	Fetched time.Time         `json:"fetched"`
}

func (c *overviewCache) restore(dir string) {
	if dir == "" {
		return
	}
	c.file = filepath.Join(dir, "overview-v1.json")
	raw, e := os.ReadFile(c.file)
	if e != nil {
		return
	}
	var v cachedOverview
	if json.Unmarshal(raw, &v) == nil && !v.Fetched.IsZero() {
		c.data = &v.Data
		c.fetched = v.Fetched
	}
}
func (c *overviewCache) read(now time.Time, force bool, load func(context.Context) dailyOverviewData) (*dailyOverviewData, bool, time.Time, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running && (force || c.data == nil || now.Sub(c.fetched) >= time.Minute) {
		c.running = true
		c.done = make(chan struct{})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			v := load(ctx)
			at := time.Now().UTC()
			// This file is a replaceable cache, not an immutable research record.
			if c.file != "" {
				if b, e := json.Marshal(cachedOverview{v, at}); e == nil {
					if os.MkdirAll(filepath.Dir(c.file), 0700) == nil {
						tmp, e := os.CreateTemp(filepath.Dir(c.file), ".overview-*")
						if e == nil {
							name := tmp.Name()
							_, e = tmp.Write(b)
							_ = tmp.Close()
							if e == nil {
								_ = os.Rename(name, c.file)
							}
							_ = os.Remove(name)
						}
					}
				}
			}
			c.mu.Lock()
			c.data = &v
			c.fetched = at
			c.running = false
			close(c.done)
			c.mu.Unlock()
		}()
	}
	var copy *dailyOverviewData
	if c.data != nil {
		v := *c.data
		copy = &v
	}
	return copy, c.running, c.fetched, c.done
}
func (s *Server) dailyOverviewGet(w http.ResponseWriter, r *http.Request) {
	c := &s.dailyOverview.overview
	v, running, at, done := c.read(time.Now(), r.URL.Query().Get("refresh") == "1", s.loadDailyOverview)
	if v == nil {
		// Fast sources still complete in one request; slow sources don't block the
		// first screen or duplicate collection when another page joins.
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-done:
			v, running, at, _ = c.read(time.Now(), false, s.loadDailyOverview)
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}
	if v == nil {
		writeJSON(w, http.StatusAccepted, map[string]any{"data": nil, "refreshing": true})
		return
	}
	now := time.Now()
	target, _ := marketpulse.Target(now)
	if v.Pulse.Date != target.Format("2006-01-02") || (v.Pulse.Session != "closed" && now.Sub(v.Pulse.FetchedAt) > 7*time.Minute) {
		v.Pulse.Stale = true
	}
	if line, e := s.dailyResearch.store.ObservationLine(r.Context()); e == nil {
		v.Pulse.ObservationLine = line
	}
	v.Judgement = marketpulse.Judge(v.Pulse, v.Indexes, v.Themes, v.ThemeMeta, now)
	writeJSON(w, 200, map[string]any{"data": v, "refreshing": running, "cached_at": at})
}
