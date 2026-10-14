package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"easy-stock/backend/internal/datahub"
	"easy-stock/backend/internal/foundation"
	"easy-stock/backend/internal/providers/cls"
	"easy-stock/backend/internal/providers/cninfo"
	"easy-stock/backend/internal/providers/duanxianxia"
	"easy-stock/backend/internal/providers/eastmoney"
	"easy-stock/backend/internal/providers/futuresposition"
	"easy-stock/backend/internal/providers/hotstock"
	"easy-stock/backend/internal/providers/sina"
	"easy-stock/backend/internal/providers/tencent"
)

func (s *Server) initSourceMonitor(cfg Config) {
	if cfg.SourceMonitor != nil {
		s.sourceMonitor = cfg.SourceMonitor
		return
	}
	em, sn, tx, cl := eastmoney.NewClient(), sina.NewClient(), tencent.NewClient(), cls.NewClient()
	dx := duanxianxia.NewClient(duanxianxia.ClientConfig{BaseURL: cfg.DuanxianxiaBaseURL})
	hot, futures := hotstock.NewClient(), futuresposition.NewClient()
	probes := []datahub.Probe{
		{SourceID: "cninfo", Capability: "announcements", Run: func(ctx context.Context) (datahub.Observation, error) {
			rows, _, err := cninfo.NewClient().MarketAnnouncements(ctx, "", "", "", 3)
			if err != nil {
				return datahub.Observation{}, err
			}
			var at time.Time
			for _, r := range rows {
				if r.PublishedAt.After(at) {
					at = r.PublishedAt
				}
			}
			o := datahub.Observation{Count: len(rows), DataAt: at, Freshness: "unverified", Detail: "官方公告目录样本；不代表 PDF 正文已提取"}
			if !at.IsZero() {
				o.Freshness = "verified"
				o.Stale = time.Since(at) > 96*time.Hour
			}
			return o, nil
		}},
		{SourceID: "tushare", Capability: "market_amount", Run: s.dailyOverview.client.Probe},
		{SourceID: "eastmoney", Capability: "kline", Run: func(ctx context.Context) (datahub.Observation, error) {
			v, e := em.KLine(ctx, "600176.SH", "day", 2)
			if e != nil {
				return datahub.Observation{}, e
			}
			var at time.Time
			valid := 0
			for _, r := range v {
				if r.Close > 0 {
					valid++
					if r.Time.After(at) {
						at = r.Time
					}
				}
			}
			return datahub.SessionObservation(valid, at, time.Now(), false), nil
		}},
		{SourceID: "sina", Capability: "quote", Run: func(ctx context.Context) (datahub.Observation, error) {
			v, e := sn.Realtime(ctx, []string{"600176.SH"})
			if e != nil {
				return datahub.Observation{}, e
			}
			if len(v) == 0 || v[0].Price <= 0 {
				return datahub.Observation{}, nil
			}
			return datahub.SessionObservation(1, v[0].TradeTime, time.Now(), true), nil
		}},
		{SourceID: "tencent", Capability: "financials", Run: func(ctx context.Context) (datahub.Observation, error) {
			v, e := tx.StockValuations(ctx, []string{"600176.SH"})
			if e != nil {
				return datahub.Observation{}, e
			}
			if len(v) == 0 {
				return datahub.Observation{}, nil
			}
			o := datahub.SessionObservation(len(v), v[0].TradeTime, time.Now(), true)
			o.Detail = "PE/PB 样本，不代表财报或海外指数覆盖"
			return o, nil
		}},
		{SourceID: "cls", Capability: "news", Run: func(ctx context.Context) (datahub.Observation, error) {
			v, e := cl.LatestNews(ctx, 3)
			if e != nil {
				return datahub.Observation{}, e
			}
			var at time.Time
			for _, r := range v {
				if r.PublishedAt.After(at) {
					at = r.PublishedAt
				}
			}
			o := datahub.Observation{Count: len(v), DataAt: at, Freshness: "unverified"}
			if !at.IsZero() {
				o.Freshness = "verified"
				o.Stale = time.Since(at) > 24*time.Hour || at.After(time.Now().Add(2*time.Minute))
			}
			return o, nil
		}},
		{SourceID: "duanxianxia", Capability: "limit_up", Run: func(ctx context.Context) (datahub.Observation, error) {
			v, e := dx.FetchLimitUpPool(ctx)
			if e != nil {
				return datahub.Observation{}, e
			}
			at, _ := time.ParseInLocation("2006-01-02", v.TradeDate, foundation.AStockLocation)
			o := datahub.SessionObservation(len(v.Events), at, time.Now(), false)
			o.Detail = "日内更新时效需结合源端时间另验"
			return o, nil
		}},
		{SourceID: "ths", Capability: "hot_rank", Run: func(ctx context.Context) (datahub.Observation, error) {
			for _, v := range hot.HotStockRanks(ctx, 3) {
				if v.Source == "ths" {
					if v.Error != "" {
						return datahub.Observation{}, errors.New(v.Error)
					}
					return datahub.Observation{Count: len(v.Items), Freshness: "unverified", Detail: "来源未提供榜单业务时间，抓取时间不代替榜单时间"}, nil
				}
			}
			return datahub.Observation{}, nil
		}},
		{SourceID: "cffex", Capability: "futures", Run: func(ctx context.Context) (datahub.Observation, error) {
			v, e := futures.Consensus(ctx, "")
			if e != nil {
				return datahub.Observation{}, e
			}
			at, _ := time.ParseInLocation("2006-01-02", v.TradeDate, foundation.AStockLocation)
			o := datahub.SessionObservation(len(v.Varieties), at, time.Now(), false)
			o.Detail = "官方日持仓披露；不包含全部历史/基差"
			return o, nil
		}},
	}
	path := ""
	if cfg.SettingsPath != "" {
		path = filepath.Join(filepath.Dir(cfg.SettingsPath), "data-source-health.json")
	}
	s.sourceMonitor = datahub.NewMonitor(probes, path)
}

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	v, refreshing := s.sourceMonitor.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"sources": v, "refreshing": refreshing, "scope": "sampled_capability", "primary_families": []string{"tushare", "eastmoney"}})
}
func (s *Server) sourcesProbe(w http.ResponseWriter, r *http.Request) {
	started := s.sourceMonitor.Refresh()
	v, refreshing := s.sourceMonitor.Snapshot()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": started, "refreshing": refreshing, "sources": v})
}
func (s *Server) dataCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": "data-contract-v1", "sources": datahub.Sources(), "content_channels": datahub.ContentChannels(), "capabilities": datahub.Contracts(), "note": "能力登记不代表全部分支已通过实测；主备是否可替代由字段口径决定"})
}
