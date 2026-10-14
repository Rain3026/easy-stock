package httpapi

import (
	"context"
	"easy-stock/backend/internal/datahub"
	"easy-stock/backend/internal/foundation"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Only published actual statements qualify. Forecast announcements stay in
// the announcement tool and cannot silently become financial statements.
func financialToolRows(rows []foundation.StockFundamentals, symbol string, cutoff time.Time) ([]map[string]any, []datahub.EvidenceSource, []string) {
	out := []map[string]any{}
	refs := []datahub.EvidenceSource{}
	gaps := []string{}
	seen := map[string]bool{}
	for _, v := range rows {
		n, e := foundation.NormalizeSymbol(v.Symbol)
		period := strings.Split(v.ReportDate, " ")[0]
		date, de := time.ParseInLocation("2006-01-02", period, foundation.AStockLocation)
		if e != nil || n.Canonical != symbol || de != nil || date.After(cutoff) || v.PublishedAt.After(cutoff) || seen[period] {
			continue
		}
		seen[period] = true
		raw, _ := json.Marshal(v)
		var all map[string]any
		_ = json.Unmarshal(raw, &all)
		fields := map[string]any{}
		for _, k := range v.Meta.AvailableFields {
			if val, ok := all[k]; ok {
				fields[k] = val
			}
		}
		row := map[string]any{"symbol": symbol, "report_period": period, "kind": "actual", "period_basis": "year_to_date", "amount_unit": "CNY", "fields": fields, "source": v.Meta}
		if v.PublishedAt.IsZero() {
			gaps = append(gaps, period+" 披露日未知，不能用于严格历史回测")
		} else {
			row["published_at"] = v.PublishedAt
		}
		if len(fields) == 0 {
			gaps = append(gaps, period+" 未提供字段可用性，暂不输出数值")
		}
		out = append(out, row)
		ref := sourceRef("financial:"+period, v.Meta, nil)
		if !v.PublishedAt.IsZero() {
			at := v.PublishedAt
			ref.PublishedAt = &at
		}
		refs = append(refs, ref)
	}
	if len(out) == 0 {
		gaps = append(gaps, "未取得截止时点可用的实际财报")
	}
	return out, refs, gaps
}
func (s *Server) agentFinancialFacts(ctx context.Context, symbol string) (any, []datahub.EvidenceSource, []string, error) {
	p, ok := s.stockBusiness.(StockFinancialHistoryProvider)
	if !ok {
		return nil, nil, nil, errors.New("financial history unavailable")
	}
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	rows, e := p.StockFinancialHistory(c, symbol, 8)
	if e != nil && s.stockFinancialSupplement != nil {
		backup, stop := context.WithTimeout(ctx, 8*time.Second)
		defer stop()
		values, be := s.stockFinancialSupplement.StockFinancialEvidence(backup, symbol, 8)
		if be != nil {
			return nil, nil, nil, e
		}
		out, refs, gaps := supplementalFinancialRows(values, symbol, time.Now())
		return out, refs, append(gaps, "财务主源失败，新浪仅提供返回字段；缺字段不补零"), nil
	}
	if e != nil {
		return nil, nil, nil, e
	}
	out, refs, gaps := financialToolRows(rows, symbol, time.Now())
	return out, refs, gaps, nil
}

func supplementalFinancialRows(values []foundation.StockFinancialEvidence, symbol string, cutoff time.Time) ([]map[string]any, []datahub.EvidenceSource, []string) {
	out := []map[string]any{}
	refs := []datahub.EvidenceSource{}
	gaps := []string{}
	seen := map[string]bool{}
	for _, v := range values {
		n, ne := foundation.NormalizeSymbol(v.Symbol)
		period := strings.Split(v.ReportDate, " ")[0]
		date, de := time.ParseInLocation("2006-01-02", period, foundation.AStockLocation)
		if ne != nil || n.Canonical != symbol || de != nil || date.After(cutoff) || v.PublishedAt.After(cutoff) || seen[period] {
			continue
		}
		seen[period] = true
		row := map[string]any{"symbol": symbol, "report_period": period, "kind": "actual", "period_basis": "year_to_date", "amount_unit": "CNY", "fields": v.Fields, "source": v.Meta}
		ref := sourceRef("financial:"+period, v.Meta, nil)
		if v.PublishedAt.IsZero() {
			gaps = append(gaps, period+" 披露日未知，不能用于严格历史回测")
		} else {
			row["published_at"] = v.PublishedAt
			at := v.PublishedAt
			ref.PublishedAt = &at
		}
		out = append(out, row)
		refs = append(refs, ref)
	}
	if len(out) == 0 {
		gaps = append(gaps, "未取得截止时点可用的实际财报")
	}
	return out, refs, gaps
}
