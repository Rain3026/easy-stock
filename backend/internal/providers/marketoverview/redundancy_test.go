package marketoverview

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"easy-stock/backend/internal/foundation"
)

type reportRoute struct {
	err   error
	rows  []foundation.MarketResearchItem
	meta  foundation.SourceMeta
	calls int
}

func (r *reportRoute) MarketReports(context.Context, string, string, string, string, int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error) {
	r.calls++
	return r.rows, r.meta, r.err
}

type reportPrimary struct {
	failingPrimary
	route *reportRoute
}

func (r reportPrimary) MarketReports(c context.Context, k, q, s, i string, l int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error) {
	return r.route.MarketReports(c, k, q, s, i, l)
}

type quoteRoute struct {
	rows  []foundation.MarketIndexSnapshot
	calls int
}

func (r *quoteRoute) MarketIndexes(context.Context, string) ([]foundation.MarketIndexSnapshot, foundation.SourceMeta, error) {
	r.calls++
	return r.rows, foundation.SourceMeta{}, nil
}

func TestReportFallbackAndPrimaryRecovery(t *testing.T) {
	for _, failure := range []string{"HTTP 403", "HTTP 429", "timeout", "empty", "stale"} {
		t.Run(failure, func(t *testing.T) {
			primary := &reportRoute{err: fmt.Errorf("%s", failure)}
			if failure == "empty" {
				primary.err = nil
			}
			if failure == "stale" {
				primary.err = nil
				primary.meta.Stale = true
				primary.rows = []foundation.MarketResearchItem{{ID: "old"}}
			}
			backup := &reportRoute{rows: []foundation.MarketResearchItem{{ID: "sina:123", Organization: "Actual Broker", Researchers: "Actual Author"}}, meta: foundation.SourceMeta{Source: "sina:report", Family: "sina", FallbackReason: "未提供目标价"}}
			third := &reportRoute{err: fmt.Errorf("should not run")}
			p := New(reportPrimary{route: primary}, nil, nil, nil).WithReports(backup, third)
			rows, meta, err := p.MarketReports(context.Background(), "stock", "", "600176.SH", "", 2)
			if err != nil || len(rows) != 1 || rows[0].Organization != "Actual Broker" || rows[0].Meta.Source != "sina:report" || !strings.Contains(meta.FallbackReason, "未提供") || third.calls != 0 {
				t.Fatal(rows, meta, err)
			}
			primary.err = nil
			primary.meta = foundation.SourceMeta{Source: "eastmoney:report"}
			primary.rows = []foundation.MarketResearchItem{{ID: "recovered"}}
			rows, _, err = p.MarketReports(context.Background(), "stock", "", "600176.SH", "", 2)
			if err != nil || rows[0].ID != "recovered" || backup.calls != 1 {
				t.Fatal("recovery not preferred", err)
			}
		})
	}
	bad := &reportRoute{err: fmt.Errorf("down")}
	p := New(reportPrimary{route: bad}, nil, nil, nil).WithReports(&reportRoute{err: fmt.Errorf("down")})
	if _, _, err := p.MarketReports(context.Background(), "stock", "", "", "", 2); err == nil {
		t.Fatal("all failed but reported success")
	}
}

func TestExtraIndexOnlyFillsMissingAndRejectsStale(t *testing.T) {
	extra := &quoteRoute{rows: []foundation.MarketIndexSnapshot{{ID: "nasdaq100", Price: 30000, Meta: foundation.SourceMeta{Source: "sina:index", FallbackReason: "延迟15分钟"}}, {ID: "sse", Price: 1}, {ID: "nasdaq", Price: 1, Meta: foundation.SourceMeta{Stale: true}}}}
	p := New(failingPrimary{}, indexFallback{}, nil, nil).WithIndexQuotes(extra)
	rows, meta, err := p.MarketIndexes(context.Background(), "core")
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	if rows[0].ID != "sse" || rows[0].Price != 3800 || rows[1].ID != "nasdaq100" || !strings.Contains(rows[1].Meta.FallbackReason, "延迟15分钟") || !strings.Contains(meta.Source, "sina:index") {
		t.Fatal(rows, meta)
	}
}

type seriesRoute struct {
	indexFallback
	series foundation.MarketIndexSeries
}

func (r seriesRoute) MarketIndexSeries(context.Context, string, string, int) (foundation.MarketIndexSeries, error) {
	return r.series, nil
}

func TestNasdaqOneRowDoesNotPreventHistoryFallback(t *testing.T) {
	short := foundation.MarketIndexSeries{Index: foundation.MarketIndexSnapshot{ID: "nasdaq100"}, Lines: []foundation.KLine{{Close: 30000}}, Meta: foundation.SourceMeta{Source: "tencent:index-kline"}}
	long := short
	long.Lines = make([]foundation.KLine, 30)
	long.Meta = foundation.SourceMeta{Source: "sina:index-kline", TradeDate: "2026-10-12", Stale: true, FallbackReason: "截至2026-10-12"}
	p := New(failingPrimary{}, seriesRoute{series: short}, nil, nil).WithIndexSeries(seriesRoute{series: long})
	v, e := p.MarketIndexSeries(context.Background(), "nasdaq100", "day", 120)
	if e != nil || len(v.Lines) != 30 || !v.Meta.Stale || v.Meta.TradeDate != "2026-10-12" {
		t.Fatal(v, e)
	}
	p.WithIndexSeries()
	v, e = p.MarketIndexSeries(context.Background(), "nasdaq100", "day", 120)
	if e != nil || len(v.Lines) != 1 || !strings.Contains(v.Meta.FallbackReason, "历史范围不足") {
		t.Fatal(v, e)
	}
}
