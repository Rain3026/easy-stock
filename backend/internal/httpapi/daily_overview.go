package httpapi

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"easy-stock/backend/internal/marketpulse"
	"easy-stock/backend/internal/providers/eastmoney"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

type dailyOverviewEngine struct {
	client   *marketpulse.Client
	mu       sync.Mutex
	latest   *marketpulse.Snapshot
	expires  time.Time
	overview overviewCache
}

func (s *Server) initDailyOverview(cfg Config, client *marketpulse.Client) {
	dir := ""
	if cfg.SettingsPath != "" {
		dir = filepath.Join(filepath.Dir(cfg.SettingsPath), "market-pulse-cache")
	}
	client.CacheDir = dir
	s.dailyOverview = &dailyOverviewEngine{client: client}
	s.dailyOverview.client.Fallback = eastmoney.NewClient()
	s.dailyOverview.overview.restore(dir)
}
func (s *Server) loadDailyPulse(ctx context.Context) marketpulse.Snapshot {
	e := s.dailyOverview
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	var v marketpulse.Snapshot
	if e.latest != nil && now.Before(e.expires) {
		v = *e.latest
	} else {
		v = e.client.Load(ctx, now)
		if ctx.Err() == nil {
			e.latest = &v
			e.expires = now.Add(time.Minute)
		}
	}
	line, err := s.dailyResearch.store.ObservationLine(ctx)
	if err != nil {
		v.Gaps = append(append([]string{}, v.Gaps...), "观察线设置读取失败")
	}
	v.ObservationLine = line
	return v
}

type dailyOverviewData struct {
	Judgement marketpulse.Judgement            `json:"judgement"`
	Pulse     marketpulse.Snapshot             `json:"pulse"`
	Indexes   []foundation.MarketIndexSnapshot `json:"indexes"`
	Themes    []foundation.ThemeOverview       `json:"themes"`
	ThemeMeta foundation.SourceMeta            `json:"theme_meta"`
	IndexMeta foundation.SourceMeta            `json:"index_meta"`
}

func (s *Server) loadDailyOverview(ctx context.Context) dailyOverviewData {
	var v dailyOverviewData
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); v.Pulse = s.loadDailyPulse(ctx) }()
	go func() {
		defer wg.Done()
		v.Indexes, v.IndexMeta, _ = loadMarketOverview(ctx, s.marketSnapshots, "indexes:core", func(c context.Context) ([]foundation.MarketIndexSnapshot, foundation.SourceMeta, error) {
			return s.marketOverview.MarketIndexes(c, "core")
		})
	}()
	go func() {
		defer wg.Done()
		target, session := marketpulse.Target(time.Now())
		if session == "closed" {
			v.Themes = s.archivedOverviewThemes(ctx, target.Format("2006-01-02"))
			if len(v.Themes) > 0 {
				v.ThemeMeta = foundation.SourceMeta{Source: "daily-research:archived-themes", TradeDate: target.Format("2006-01-02")}
				return
			}
		}
		c, done := context.WithTimeout(ctx, 15*time.Second)
		defer done()
		v.Themes, v.ThemeMeta, _ = s.themeOverview.Overviews(c)
	}()
	wg.Wait()
	timed, _ := s.dailyOverview.client.Indexes(ctx)
	for _, idx := range timed {
		found := false
		for i := range v.Indexes {
			if v.Indexes[i].ID == idx.ID {
				v.Indexes[i] = idx
				found = true
				break
			}
		}
		if !found {
			v.Indexes = append(v.Indexes, idx)
		}
	}
	// The same existing intraday ladder powers the live short-term reference; never borrow a different date.
	if v.Pulse.Session != "closed" && s.limitUpSnapshots != nil {
		c, done := context.WithTimeout(ctx, 5*time.Second)
		defer done()
		ladder, e := s.limitUpSnapshots.load(c, s.limitUpProvider, s.stockConcepts, s.realtimeProvider)
		if e == nil && ladder.Current.TradeDate == v.Pulse.Date && !ladder.Meta.Stale {
			d := ladder.Current
			em := &marketpulse.Emotion{Date: d.TradeDate, Up: d.LimitUpCount, Boards: d.BoardCount, Height: d.MaxStreak, Source: "连板雷达 · 盘中涨停池", Levels: []marketpulse.Level{}}
			for _, l := range d.Levels {
				if l.Level >= 2 {
					em.Levels = append(em.Levels, marketpulse.Level{Height: l.Level, Count: l.Count})
				}
			}
			em.Partial = true
			v.Pulse.Emotion = em
		}
	}
	v.Judgement = marketpulse.Judge(v.Pulse, v.Indexes, v.Themes, v.ThemeMeta, time.Now())
	return v
}
func (s *Server) dailyOverviewSettings(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Line float64 `json:"observation_line"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&v) != nil || v.Line < 100 || v.Line > 1000000 {
		writeError(w, 400, "观察线须为100至1,000,000亿元")
		return
	}
	if err := s.dailyResearch.store.SaveObservationLine(r.Context(), v.Line); err != nil {
		writeError(w, 500, "观察线保存失败")
		return
	}
	writeJSON(w, 200, map[string]any{"data": v})
}

func (s *Server) archivedOverviewThemes(ctx context.Context, date string) []foundation.ThemeOverview {
	records, _ := s.dailyResearch.store.Records(ctx, "snapshot", "")
	for _, raw := range records {
		var v struct {
			TradeDate string `json:"trade_date"`
			Themes    []struct {
				Overview foundation.ThemeOverview `json:"overview"`
			} `json:"themes"`
		}
		if json.Unmarshal(raw, &v) != nil || v.TradeDate != date {
			continue
		}
		out := []foundation.ThemeOverview{}
		for _, t := range v.Themes {
			if t.Overview.TradeDate == date {
				out = append(out, t.Overview)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}
