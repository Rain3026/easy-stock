package marketoverview

import (
	"context"
	"fmt"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
)

type Primary interface {
	MarketIndexes(ctx context.Context, scope string) ([]foundation.MarketIndexSnapshot, foundation.SourceMeta, error)
	MarketIndexSeries(ctx context.Context, id string, period string, limit int) (foundation.MarketIndexSeries, error)
	IndustryMomentum(ctx context.Context, limit int) ([]foundation.MarketIndustryMomentum, foundation.SourceMeta, error)
	MarketFundFlows(ctx context.Context, dimension string, sortKey string, limit int) ([]foundation.MarketFundFlow, foundation.SourceMeta, error)
	MarketMarginSeries(ctx context.Context, limit int) ([]foundation.MarketMarginPoint, foundation.SourceMeta, error)
	MarketBillboard(ctx context.Context, tradeDate string, limit int) ([]foundation.MarketBillboardItem, foundation.SourceMeta, error)
	MarketBillboardDetail(ctx context.Context, symbol string, tradeDate string, reason string) (foundation.MarketBillboardDetail, foundation.SourceMeta, error)
	MarketAnnouncements(ctx context.Context, query string, symbol string, category string, limit int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error)
	MarketReports(ctx context.Context, kind string, query string, symbol string, industry string, limit int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error)
}

type IndustryMomentumProvider interface {
	IndustryMomentum(ctx context.Context, limit int) ([]foundation.MarketIndustryMomentum, foundation.SourceMeta, error)
}

type FundFlowProvider interface {
	MarketFundFlows(ctx context.Context, dimension string, sortKey string, limit int) ([]foundation.MarketFundFlow, foundation.SourceMeta, error)
}

type USSectorMomentumProvider interface {
	USSectorMomentum(ctx context.Context, limit int) ([]foundation.MarketUSSectorMomentum, foundation.SourceMeta, error)
}

type IndexFallback interface {
	MarketIndexes(ctx context.Context, scope string) ([]foundation.MarketIndexSnapshot, foundation.SourceMeta, error)
	MarketIndexSeries(ctx context.Context, id string, period string, limit int) (foundation.MarketIndexSeries, error)
}

type AnnouncementProvider interface {
	MarketAnnouncements(context.Context, string, string, string, int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error)
}

type ReportProvider interface {
	MarketReports(context.Context, string, string, string, string, int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error)
}
type IndexQuoteProvider interface {
	MarketIndexes(context.Context, string) ([]foundation.MarketIndexSnapshot, foundation.SourceMeta, error)
}
type IndexSeriesProvider interface {
	MarketIndexSeries(context.Context, string, string, int) (foundation.MarketIndexSeries, error)
}

func (p *Provider) WithIndexSeries(v ...IndexSeriesProvider) *Provider { p.indexSeries = v; return p }

func (p *Provider) WithReports(v ...ReportProvider) *Provider         { p.reports = v; return p }
func (p *Provider) WithIndexQuotes(v ...IndexQuoteProvider) *Provider { p.indexQuotes = v; return p }

func (p *Provider) WithAnnouncements(v AnnouncementProvider) *Provider { p.announcements = v; return p }

type BillboardProvider interface {
	MarketBillboard(context.Context, string, int) ([]foundation.MarketBillboardItem, foundation.SourceMeta, error)
	MarketBillboardDetail(context.Context, string, string, string) (foundation.MarketBillboardDetail, foundation.SourceMeta, error)
}

func (p *Provider) WithBillboard(v BillboardProvider) *Provider { p.billboard = v; return p }

type MarginProvider interface {
	MarketMarginSeries(context.Context, int) ([]foundation.MarketMarginPoint, foundation.SourceMeta, error)
}

func (p *Provider) WithMargin(v MarginProvider) *Provider { p.margin = v; return p }

type Provider struct {
	margin           MarginProvider
	billboard        BillboardProvider
	announcements    AnnouncementProvider
	reports          []ReportProvider
	indexQuotes      []IndexQuoteProvider
	indexSeries      []IndexSeriesProvider
	primary          Primary
	indexFallback    IndexFallback
	industryProvider IndustryMomentumProvider
	fundFlowProvider FundFlowProvider
}

func New(primary Primary, indexFallback IndexFallback, industryProvider IndustryMomentumProvider, fundFlowProvider FundFlowProvider) *Provider {
	return &Provider{primary: primary, indexFallback: indexFallback, industryProvider: industryProvider, fundFlowProvider: fundFlowProvider}
}

func (p *Provider) MarketIndexes(ctx context.Context, scope string) ([]foundation.MarketIndexSnapshot, foundation.SourceMeta, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	items, meta, err := p.primary.MarketIndexes(c, scope)
	cancel()
	expected := []string{"sse", "szse", "chinext", "csi300", "sse50", "csi1000", "star50", "hsi", "dow", "sp500", "nasdaq", "nasdaq100"}
	if scope != "core" {
		expected = append(expected, "nikkei", "kospi", "taiwan", "ftse", "dax", "cac")
	}
	byID := map[string]foundation.MarketIndexSnapshot{}
	for _, v := range items {
		if v.Price > 0 && !v.Meta.Stale {
			byID[v.ID] = v
		}
	}
	used := false
	if len(byID) < len(expected) && p.indexFallback != nil {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		back, _, be := p.indexFallback.MarketIndexes(c, scope)
		cancel()
		if be == nil {
			for _, v := range back {
				if _, exists := byID[v.ID]; !exists && v.Price > 0 && !v.Meta.Stale {
					v.Meta.FallbackReason = "主来源缺少本指数，已采用同一指数的腾讯行情"
					byID[v.ID] = v
					used = true
				}
			}
		}
	}
	additionalSources := []string{}
	for _, provider := range p.indexQuotes {
		if len(byID) >= len(expected) {
			break
		}
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		back, _, be := provider.MarketIndexes(c, scope)
		cancel()
		if be != nil {
			continue
		}
		for _, v := range back {
			if _, exists := byID[v.ID]; !exists && v.Price > 0 && !v.Meta.Stale {
				v.Meta.FallbackReason = joinFallbackReason("前序来源缺少有效的同一指数，已切换独立备用", v.Meta.FallbackReason)
				byID[v.ID] = v
				additionalSources = append(additionalSources, v.Meta.Source)
			}
		}
	}
	out := []foundation.MarketIndexSnapshot{}
	missing := []string{}
	for _, id := range expected {
		if v, ok := byID[id]; ok {
			out = append(out, v)
		} else {
			missing = append(missing, id)
		}
	}
	if len(out) == 0 {
		return nil, foundation.SourceMeta{}, fmt.Errorf("指数主备均无有效数据")
	}
	if used {
		meta.FallbackReason = "主来源缺项已逐指数切换腾讯；每项保留独立来源与交易时间"
		if err != nil || meta.Source == "" {
			meta.Source = "tencent:index"
		} else {
			meta.Source += " + tencent:index"
		}
	}
	for _, source := range additionalSources {
		if meta.Source == "" {
			meta.Source = source
		} else if !strings.Contains(meta.Source, source) {
			meta.Source += " + " + source
		}
	}
	if len(additionalSources) > 0 {
		meta.FallbackReason = joinFallbackReason(meta.FallbackReason, "逐指数补入独立备用；延迟/覆盖限制见每项来源")
		meta.FetchedAt = time.Now()
	}
	if len(missing) > 0 {
		meta.MissingSymbols = missing
		meta.FallbackReason = joinFallbackReason(meta.FallbackReason, "部分指数缺少等价备用："+strings.Join(missing, ","))
	}
	return out, meta, nil
}

func (p *Provider) MarketIndexSeries(ctx context.Context, id string, period string, limit int) (foundation.MarketIndexSeries, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	series, err := p.primary.MarketIndexSeries(c, id, period, limit)
	cancel()
	complete := func(v foundation.MarketIndexSeries) bool {
		if len(v.Lines) == 0 {
			return false
		}
		if id != "nasdaq100" {
			return true
		}
		want := limit
		if want <= 0 {
			want = 120
		}
		return v.Index.ID == id && len(v.Lines) >= min(want, 30)
	}
	if err == nil && complete(series) {
		return series, err
	}
	var partial *foundation.MarketIndexSeries
	if err == nil && len(series.Lines) > 0 && series.Index.ID == id {
		partial = &series
	}
	routes := []IndexSeriesProvider{}
	if p.indexFallback != nil {
		routes = append(routes, p.indexFallback)
	}
	routes = append(routes, p.indexSeries...)
	for _, route := range routes {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		fallback, e := route.MarketIndexSeries(c, id, period, limit)
		cancel()
		if e != nil || len(fallback.Lines) == 0 {
			continue
		}
		if !complete(fallback) {
			if fallback.Index.ID == id && (partial == nil || len(fallback.Lines) > len(partial.Lines)) {
				copy := fallback
				partial = &copy
			}
			continue
		}
		label := fallback.Meta.Source
		if strings.HasPrefix(label, "tencent:") {
			label = "腾讯 K 线"
		}
		if strings.HasPrefix(label, "sina:") {
			label = "新浪 K 线"
		}
		fallback.Meta.FallbackReason = joinFallbackReason("前序指数走势不可用，已切换 "+label, fallback.Meta.FallbackReason)
		fallback.Index.Meta = fallback.Meta
		for i := range fallback.Lines {
			fallback.Lines[i].Meta = fallback.Meta
		}
		return fallback, nil
	}
	if partial != nil {
		partial.Meta.FallbackReason = joinFallbackReason(fmt.Sprintf("历史范围不足，当前仅 %d 条；备用未恢复", len(partial.Lines)), partial.Meta.FallbackReason)
		partial.Index.Meta = partial.Meta
		for i := range partial.Lines {
			partial.Lines[i].Meta = partial.Meta
		}
		return *partial, nil
	}
	return foundation.MarketIndexSeries{}, fmt.Errorf("index series unavailable for %s", id)
}

func (p *Provider) IndustryMomentum(ctx context.Context, limit int) ([]foundation.MarketIndustryMomentum, foundation.SourceMeta, error) {
	if p.industryProvider == nil {
		return p.primary.IndustryMomentum(ctx, limit)
	}
	items, meta, err := p.industryProvider.IndustryMomentum(ctx, limit)
	if err == nil {
		return items, meta, nil
	}
	fallbackItems, fallbackMeta, fallbackErr := p.primary.IndustryMomentum(ctx, limit)
	if fallbackErr != nil {
		return nil, foundation.SourceMeta{}, fmt.Errorf("tencent industry momentum failed: %v; eastmoney fallback failed: %w", err, fallbackErr)
	}
	fallbackMeta.FallbackReason = joinFallbackReason("腾讯行业强度不可用，已回退东方财富可用字段", fallbackMeta.FallbackReason)
	for index := range fallbackItems {
		fallbackItems[index].Meta = fallbackMeta
	}
	return fallbackItems, fallbackMeta, nil
}

func (p *Provider) MarketFundFlows(ctx context.Context, dimension string, sortKey string, limit int) ([]foundation.MarketFundFlow, foundation.SourceMeta, error) {
	if p.fundFlowProvider == nil {
		return p.primary.MarketFundFlows(ctx, dimension, sortKey, limit)
	}
	items, meta, err := p.fundFlowProvider.MarketFundFlows(ctx, dimension, sortKey, limit)
	if err == nil {
		return items, meta, nil
	}
	fallbackItems, fallbackMeta, fallbackErr := p.primary.MarketFundFlows(ctx, dimension, sortKey, limit)
	if fallbackErr != nil {
		return nil, foundation.SourceMeta{}, fmt.Errorf("sina %s fund flow failed: %v; eastmoney fallback failed: %w", dimension, err, fallbackErr)
	}
	fallbackMeta.FallbackReason = joinFallbackReason("新浪资金榜不可用，已回退东方财富可用字段", fallbackMeta.FallbackReason)
	for index := range fallbackItems {
		fallbackItems[index].Meta = fallbackMeta
	}
	return fallbackItems, fallbackMeta, nil
}

func (p *Provider) USSectorMomentum(ctx context.Context, limit int) ([]foundation.MarketUSSectorMomentum, foundation.SourceMeta, error) {
	if provider, ok := p.primary.(USSectorMomentumProvider); ok {
		return provider.USSectorMomentum(ctx, limit)
	}
	if provider, ok := p.indexFallback.(USSectorMomentumProvider); ok {
		items, meta, err := provider.USSectorMomentum(ctx, limit)
		if err != nil {
			return nil, foundation.SourceMeta{}, err
		}
		meta.FallbackReason = joinFallbackReason("主行情源不提供美股板块ETF，已切换腾讯行情", meta.FallbackReason)
		for index := range items {
			items[index].Meta = meta
		}
		return items, meta, nil
	}
	return nil, foundation.SourceMeta{}, fmt.Errorf("US sector momentum provider unavailable")
}

func (p *Provider) MarketMarginSeries(ctx context.Context, limit int) ([]foundation.MarketMarginPoint, foundation.SourceMeta, error) {
	c, cancel := context.WithTimeout(ctx, 6*time.Second)
	items, meta, err := p.primary.MarketMarginSeries(c, limit)
	cancel()
	if err == nil && len(items) > 0 && !meta.Stale {
		return items, meta, nil
	}
	if p.margin != nil {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		back, bm, be := p.margin.MarketMarginSeries(c, limit)
		if be == nil && len(back) > 0 && !bm.Stale {
			bm.FallbackReason = "首选两融来源不可用，整段切换Tushare；" + bm.FallbackReason
			for i := range back {
				back[i].Meta = bm
			}
			return back, bm, nil
		}
	}
	if err == nil {
		err = fmt.Errorf("两融主备均无有效数据")
	}
	return nil, meta, err
}

func joinFallbackReason(primary string, secondary string) string {
	if secondary == "" {
		return primary
	}
	return primary + "；" + secondary
}

func (p *Provider) MarketBillboard(ctx context.Context, tradeDate string, limit int) ([]foundation.MarketBillboardItem, foundation.SourceMeta, error) {
	c, cancel := context.WithTimeout(ctx, 6*time.Second)
	items, meta, err := p.primary.MarketBillboard(c, tradeDate, limit)
	cancel()
	if err == nil && len(items) > 0 && !meta.Stale {
		return items, meta, nil
	}
	if p.billboard != nil {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		back, bm, be := p.billboard.MarketBillboard(c, tradeDate, limit)
		if be == nil && !bm.Stale {
			bm.FallbackReason = "首选龙虎榜不可用或无记录，使用Tushare同日榜单；" + bm.FallbackReason
			for i := range back {
				back[i].Meta.FallbackReason = bm.FallbackReason
			}
			return back, bm, nil
		}
		if err != nil {
			return nil, meta, fmt.Errorf("龙虎榜主备均失败：%w", be)
		}
	}
	return items, meta, err
}

func (p *Provider) MarketBillboardDetail(ctx context.Context, symbol string, tradeDate string, reason string) (foundation.MarketBillboardDetail, foundation.SourceMeta, error) {
	c, cancel := context.WithTimeout(ctx, 6*time.Second)
	d, meta, err := p.primary.MarketBillboardDetail(c, symbol, tradeDate, reason)
	cancel()
	if err == nil && (len(d.BuySeats) > 0 || len(d.SellSeats) > 0) && !meta.Stale {
		return d, meta, nil
	}
	if p.billboard != nil {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		back, bm, be := p.billboard.MarketBillboardDetail(c, symbol, tradeDate, reason)
		if be == nil {
			bm.FallbackReason = "首选席位未就绪，使用Tushare同日同原因席位；" + bm.FallbackReason
			back.Meta = bm
			return back, bm, nil
		}
		return d, meta, be
	}
	return d, meta, err
}

func (p *Provider) MarketAnnouncements(ctx context.Context, query string, symbol string, category string, limit int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error) {
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	items, meta, err := p.primary.MarketAnnouncements(c, query, symbol, category, limit)
	cancel()
	if (err == nil && len(items) > 0) || p.announcements == nil {
		return items, meta, err
	}
	fallback, fm, fe := p.announcements.MarketAnnouncements(ctx, query, symbol, category, limit)
	if fe != nil {
		if err == nil {
			return items, meta, nil
		}
		return nil, foundation.SourceMeta{}, fmt.Errorf("公告主源与官方备用均暂不可用")
	}
	fm.FallbackReason = "东方财富未返回匹配公告，已查询巨潮官方目录；备用提供原始 PDF 链接，正文需另取"
	for i := range fallback {
		fallback[i].Meta = fm
	}
	return fallback, fm, nil
}

func (p *Provider) MarketAnnouncementContent(ctx context.Context, id string) (string, error) {
	if strings.HasPrefix(id, "cninfo:") {
		return "", fmt.Errorf("巨潮目录仅包含公告原文链接，尚未提取 PDF 正文")
	}
	if provider, ok := p.primary.(interface {
		MarketAnnouncementContent(context.Context, string) (string, error)
	}); ok {
		return provider.MarketAnnouncementContent(ctx, id)
	}
	return "", fmt.Errorf("announcement body provider unavailable")
}

func (p *Provider) MarketReports(ctx context.Context, kind string, query string, symbol string, industry string, limit int) ([]foundation.MarketResearchItem, foundation.SourceMeta, error) {
	c, cancel := context.WithTimeout(ctx, 6*time.Second)
	items, meta, err := p.primary.MarketReports(c, kind, query, symbol, industry, limit)
	cancel()
	if len(p.reports) == 0 || (err == nil && len(items) > 0 && !meta.Stale) {
		return items, meta, err
	}
	emptyResponse := err == nil && !meta.Stale
	for _, provider := range p.reports {
		if ctx.Err() != nil {
			return nil, meta, ctx.Err()
		}
		c, cancel := context.WithTimeout(ctx, 6*time.Second)
		rows, fm, fe := provider.MarketReports(c, kind, query, symbol, industry, limit)
		cancel()
		if fe != nil || fm.Stale {
			continue
		}
		if len(rows) == 0 {
			emptyResponse = true
			continue
		}
		fm.FallbackReason = joinFallbackReason("前序研报来源无有效匹配，已切换备用目录；保留实际机构、作者与发布日期", fm.FallbackReason)
		for i := range rows {
			rows[i].Meta = fm
		}
		return rows, fm, nil
	}
	if emptyResponse {
		meta.FallbackReason = joinFallbackReason(meta.FallbackReason, "未找到匹配研报；备用目录也未取得有效匹配，不代表不存在研报")
		return []foundation.MarketResearchItem{}, meta, nil
	}
	return nil, meta, fmt.Errorf("研报主备暂不可用；未用旧文档或其他机构观点替代")
}
