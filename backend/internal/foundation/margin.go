package foundation

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type MarginExchangePoint struct {
	Exchange string
	Point    MarketMarginPoint
}

// CompleteMarginSeries only aggregates a date after all three exchanges have
// published. Missing today's Shenzhen row must never halve the market balance.
func CompleteMarginSeries(rows []MarginExchangePoint, limit int, meta SourceMeta) ([]MarketMarginPoint, SourceMeta, error) {
	groups := map[string]map[string]MarketMarginPoint{}
	for _, row := range rows {
		p := row.Point
		if row.Exchange != "SSE" && row.Exchange != "SZSE" && row.Exchange != "BSE" {
			continue
		}
		day, err := time.Parse("2006-01-02", p.TradeDate)
		if err != nil || day.Format("2006-01-02") > time.Now().In(AStockLocation).Format("2006-01-02") {
			return nil, meta, fmt.Errorf("invalid margin trade date")
		}
		for _, v := range []float64{p.FinancingBalance, p.SecuritiesLendingBalance, p.MarginBalance, p.FinancingBuyAmount, p.FinancingRepayAmount} {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				return nil, meta, fmt.Errorf("invalid margin amount")
			}
		}
		if math.Abs(p.MarginBalance-p.FinancingBalance-p.SecuritiesLendingBalance) > math.Max(1, p.MarginBalance*1e-7) {
			return nil, meta, fmt.Errorf("margin balance identity mismatch")
		}
		if groups[p.TradeDate] == nil {
			groups[p.TradeDate] = map[string]MarketMarginPoint{}
		}
		if _, exists := groups[p.TradeDate][row.Exchange]; exists {
			return nil, meta, fmt.Errorf("duplicate margin exchange/date")
		}
		groups[p.TradeDate][row.Exchange] = p
	}
	dates := []string{}
	partial := 0
	for day, g := range groups {
		if len(g) == 3 {
			dates = append(dates, day)
		} else {
			partial++
		}
	}
	sort.Strings(dates)
	if len(dates) < 2 {
		return nil, meta, fmt.Errorf("两融同日三市完整数据不足，不能生成余额变化")
	}
	meta.TradeDate = dates[len(dates)-1]
	if lag, ok := AStockSessionLag(meta.TradeDate, time.Now()); !ok || lag > 2 {
		meta.Stale = true
		meta.FallbackReason += "；最近完整日期超过两个已完成交易日，数据待更新"
	}
	if partial > 0 {
		meta.FallbackReason += fmt.Sprintf("；%d个日期披露未齐，不纳入全市场合计", partial)
	}
	result := []MarketMarginPoint{}
	for _, date := range dates {
		p := MarketMarginPoint{TradeDate: date, Meta: meta}
		for _, exchange := range []string{"SSE", "SZSE", "BSE"} {
			v := groups[date][exchange]
			p.FinancingBalance += v.FinancingBalance
			p.SecuritiesLendingBalance += v.SecuritiesLendingBalance
			p.MarginBalance += v.MarginBalance
			p.FinancingBuyAmount += v.FinancingBuyAmount
			p.FinancingRepayAmount += v.FinancingRepayAmount
			p.SecuritiesLendingSellVolume += v.SecuritiesLendingSellVolume
			p.SecuritiesLendingRepayVolume += v.SecuritiesLendingRepayVolume
		}
		p.FinancingNetBuyAmount = p.FinancingBuyAmount - p.FinancingRepayAmount
		if len(result) > 0 {
			previous := result[len(result)-1]
			expected := LatestCompletedAStockSession(timeMustParseMarginDate(date)).Format("2006-01-02")
			if previous.TradeDate == expected {
				p.MarginBalanceChange = p.MarginBalance - previous.MarginBalance
			} else {
				p.Meta.AvailableFields = []string{}
				for _, field := range meta.AvailableFields {
					if field != "margin_balance_change" {
						p.Meta.AvailableFields = append(p.Meta.AvailableFields, field)
					}
				}
				p.Meta.FallbackReason += "；前一交易日有缺口，余额变化未提供"
			}
		}
		result = append(result, p)
	}
	// The first row has no predecessor and is never exposed with a fake zero change.
	result = result[1:]
	if len(result) > limit {
		result = result[len(result)-limit:]
	}
	if len(result) > 0 {
		meta = result[len(result)-1].Meta
	}
	return result, meta, nil
}

func timeMustParseMarginDate(date string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", date, AStockLocation)
	return t
}
