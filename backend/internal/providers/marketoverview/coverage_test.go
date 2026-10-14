package marketoverview

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"testing"
)

type coverageSource struct {
	failingPrimary
	bad    bool
	calls  int
	source string
}

func (p *coverageSource) MarketMarginSeries(context.Context, int) ([]foundation.MarketMarginPoint, foundation.SourceMeta, error) {
	p.calls++
	if p.bad {
		return nil, foundation.SourceMeta{}, fmt.Errorf("HTTP 429")
	}
	m := foundation.SourceMeta{Source: p.source}
	return []foundation.MarketMarginPoint{{MarginBalance: 30, Meta: m}}, m, nil
}
func (p *coverageSource) MarketBillboard(context.Context, string, int) ([]foundation.MarketBillboardItem, foundation.SourceMeta, error) {
	p.calls++
	if p.bad {
		return nil, foundation.SourceMeta{}, fmt.Errorf("HTTP 403")
	}
	m := foundation.SourceMeta{Source: p.source}
	return []foundation.MarketBillboardItem{{Symbol: "600000.SH", Meta: m}}, m, nil
}
func (p *coverageSource) MarketBillboardDetail(context.Context, string, string, string) (foundation.MarketBillboardDetail, foundation.SourceMeta, error) {
	p.calls++
	if p.bad {
		return foundation.MarketBillboardDetail{}, foundation.SourceMeta{}, fmt.Errorf("timeout")
	}
	m := foundation.SourceMeta{Source: p.source}
	return foundation.MarketBillboardDetail{BuySeats: []foundation.MarketBillboardSeat{{Name: "seat"}}, Meta: m}, m, nil
}
func TestCoverageFallbackAndPrimaryRecovery(t *testing.T) {
	ctx := context.Background()
	primary := &coverageSource{bad: true, source: "eastmoney"}
	backup := &coverageSource{source: "tushare"}
	p := New(primary, nil, nil, nil).WithMargin(backup).WithBillboard(backup)
	_, m, e := p.MarketMarginSeries(ctx, 30)
	if e != nil || m.Source != "tushare" {
		t.Fatal(m, e)
	}
	_, m, e = p.MarketBillboard(ctx, "2026-10-13", 50)
	if e != nil || m.Source != "tushare" {
		t.Fatal(m, e)
	}
	_, m, e = p.MarketBillboardDetail(ctx, "600000.SH", "2026-10-13", "reason")
	if e != nil || m.Source != "tushare" {
		t.Fatal(m, e)
	}
	primary.bad = false
	_, m, e = p.MarketMarginSeries(ctx, 30)
	if e != nil || m.Source != "eastmoney" {
		t.Fatal(m, e)
	}
	_, m, e = p.MarketBillboard(ctx, "2026-10-13", 50)
	if e != nil || m.Source != "eastmoney" || backup.calls != 3 {
		t.Fatal(m, e, backup.calls)
	}
	primary.bad = true
	backup.bad = true
	if _, _, e = p.MarketMarginSeries(ctx, 30); e == nil {
		t.Fatal("all failed success")
	}
	if _, _, e = p.MarketBillboard(ctx, "2026-10-13", 50); e == nil {
		t.Fatal("all failed success")
	}
}
