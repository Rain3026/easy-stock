package dailyresearch

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"math"
	"sort"
	"time"
)

// Outcomes are observations on fixed exchange sessions, never simulated trades.
func Verify(ctx context.Context, s Snapshot, now time.Time, load func(context.Context, string) ([]foundation.KLine, error)) Verification {
	v := Verification{SnapshotID: s.ID, CheckedAt: now, Outcomes: []Outcome{}, Note: "1/3/5个交易日的收盘观察变化，以冻结的已完成日线为基准；不是可交易收益。智能体文字条件需人工核验。"}
	date, _ := time.ParseInLocation("2006-01-02", s.TradeDate, foundation.AStockLocation)
	latest := foundation.LatestCompletedAStockSession(now.Add(-5 * time.Minute))
	seen := map[string]bool{}
	themes := append([]Theme(nil), s.Themes...)
	if s.EntityID != "" {
		names := []string{}
		for symbol := range s.Baselines {
			if _, e := foundation.NormalizeSymbol(symbol); e == nil {
				names = append(names, symbol)
			}
		}
		sort.Strings(names)
		extra := Theme{}
		for _, symbol := range names {
			extra.Candidates = append(extra.Candidates, foundation.BoardStock{Symbol: symbol, Name: s.ContextLabel})
		}
		themes = append(themes, extra)
	}
	for _, t := range themes {
		for _, stock := range t.Candidates {
			if seen[stock.Symbol] {
				continue
			}
			seen[stock.Symbol] = true
			base, hasBase := s.Baselines[stock.Symbol]
			var lines []foundation.KLine
			var loadErr error
			if date.Before(latest) {
				lines, loadErr = load(ctx, stock.Symbol)
			}
			byDate := map[string]foundation.KLine{}
			for _, b := range lines {
				byDate[b.Time.In(foundation.AStockLocation).Format("2006-01-02")] = b
			}
			for _, days := range []int{1, 3, 5} {
				due := date
				for n := 0; n < days; {
					due = due.AddDate(0, 0, 1)
					if foundation.IsAStockTradingDay(due) {
						n++
					}
				}
				o := Outcome{Symbol: stock.Symbol, Name: stock.Name, Days: days, DueDate: due.Format("2006-01-02"), Status: "pending", Reason: "观察窗口尚未收盘"}
				if due.Year() > 2026 || due.Year() < 2025 {
					o.Status = "unavailable"
					o.Reason = "本地交易日历未覆盖该年份"
				} else if !due.After(latest) {
					o.Status = "unavailable"
					o.Reason = "缺少冻结基准或到期日K线；停牌不顺延窗口"
					freshBase, ok := byDate[base.Time.In(foundation.AStockLocation).Format("2006-01-02")]
					end, exists := byDate[o.DueDate]
					if loadErr == nil && hasBase && ok && exists && base.Close > 0 && end.Close > 0 {
						if math.Abs(freshBase.Close-base.Close) > math.Max(.0001, base.Close*.00001) || base.Meta.Source != freshBase.Meta.Source || end.Meta.Source != base.Meta.Source {
							o.Status = "revised"
							o.Reason = "基准价格或来源变化，可能发生复权修订；不合并计算"
						} else {
							pct := (end.Close/base.Close - 1) * 100
							o.Status = "observed"
							o.Reason = ""
							o.ChangePercent = &pct
							o.Baseline = &base
							o.Observation = &end
						}
					}
				}
				v.Outcomes = append(v.Outcomes, o)
			}
		}
	}
	if len(v.Outcomes) == 0 {
		v.Note = "没有可核验的证券收盘基准。组合尚未建立净值序列，不能用个股涨跌代替组合收益；文字条件保留人工核验。"
	}
	return v
}
