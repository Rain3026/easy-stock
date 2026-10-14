package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"sort"
	"strings"
	"time"
)

func billboardMeta(date, source string) foundation.SourceMeta {
	return foundation.SourceMeta{Family: "tushare", Channel: "http", Source: source, SourceURL: "https://tushare.pro/document/2?doc_id=106", TradeDate: date, AmountUnit: "CNY", FetchedAt: time.Now(), AvailableFields: []string{"close_price", "change_percent", "turnover_rate", "reason", "buy_amount", "sell_amount", "net_amount"}, FallbackReason: "日频披露；不同来源上榜原因条数可能不同，不声明全榜等价；机构/席位仅在同日同原因明细齐备时提供"}
}
func (c *Client) MarketBillboard(ctx context.Context, tradeDate string, limit int) ([]foundation.MarketBillboardItem, foundation.SourceMeta, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	explicit := strings.TrimSpace(tradeDate) != ""
	date := foundation.LatestCompletedAStockSession(time.Now())
	var e error
	if explicit {
		date, e = time.ParseInLocation("2006-01-02", tradeDate, foundation.AStockLocation)
		if e != nil {
			return nil, foundation.SourceMeta{}, e
		}
	}
	for attempt := 0; attempt < 8; attempt++ {
		day := date.Format("2006-01-02")
		meta := billboardMeta(day, "tushare:top_list")
		rows, err := c.query(ctx, "top_list", map[string]any{"trade_date": date.Format("20060102")}, "", 5*time.Minute)
		if err != nil {
			return nil, meta, err
		}
		if len(rows) >= 10000 {
			return nil, meta, fmt.Errorf("top_list may be truncated")
		}
		if len(rows) == 0 {
			if explicit {
				return []foundation.MarketBillboardItem{}, meta, nil
			}
			date = foundation.LatestCompletedAStockSession(date.Add(-time.Minute))
			continue
		}
		items, err := parseBillboardRows(rows, day, meta)
		if err != nil {
			return nil, meta, err
		}
		// One date-wide request, never a per-stock fan-out. Metadata marks optional failure.
		seats, seatErr := c.query(ctx, "top_inst", map[string]any{"trade_date": date.Format("20060102")}, "", 5*time.Minute)
		if seatErr == nil && len(seats) < 10000 {
			for i := range items {
				d, _, de := parseBillboardSeats(seats, items[i].Symbol, day, items[i].Reason)
				if de != nil {
					continue
				}
				items[i].BuySeats = len(d.BuySeats)
				items[i].SellSeats = len(d.SellSeats)
				for _, s := range d.BuySeats {
					if s.Institution {
						items[i].InstitutionBuyers++
					}
				}
				items[i].Meta.AvailableFields = append(append([]string{}, items[i].Meta.AvailableFields...), "buy_seats", "sell_seats", "institution_buyers")
			}
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].NetAmount > items[j].NetAmount })
		if len(items) > limit {
			meta.FallbackReason += fmt.Sprintf("；当前按净买入展示%d/%d条，不是完整榜", limit, len(items))
			items = items[:limit]
		}
		for i := range items {
			items[i].Meta.FallbackReason = meta.FallbackReason
		}
		return items, meta, nil
	}
	return nil, foundation.SourceMeta{}, fmt.Errorf("Tushare近8个交易日无龙虎榜数据")
}
func parseBillboardRows(rows []Row, date string, meta foundation.SourceMeta) ([]foundation.MarketBillboardItem, error) {
	out := []foundation.MarketBillboardItem{}
	seen := map[string]bool{}
	for _, r := range rows {
		symbol := str(r, "ts_code")
		n, e := foundation.NormalizeSymbol(symbol)
		if e != nil || str(r, "trade_date") != strings.ReplaceAll(date, "-", "") {
			return nil, fmt.Errorf("龙虎榜日期或证券不符")
		}
		reason := strings.TrimSpace(str(r, "reason"))
		key := n.Canonical + "|" + reason
		if reason == "" || seen[key] {
			return nil, fmt.Errorf("龙虎榜原因缺失或重复")
		}
		seen[key] = true
		v, e := requiredNumbers(r, "close", "pct_change", "l_buy", "l_sell", "net_amount")
		if e != nil {
			return nil, e
		}
		if v[0] <= 0 || v[2] < 0 || v[3] < 0 {
			return nil, fmt.Errorf("invalid billboard values")
		}
		itemMeta := meta
		turnover, turnoverErr := requiredNumber(r, "turnover_rate")
		if turnoverErr != nil {
			itemMeta.AvailableFields = []string{"close_price", "change_percent", "reason", "buy_amount", "sell_amount", "net_amount"}
		}
		out = append(out, foundation.MarketBillboardItem{TradeDate: date, Symbol: n.Canonical, Name: str(r, "name"), ClosePrice: v[0], ChangePercent: v[1], TurnoverRate: turnover, Reason: reason, BuyAmount: v[2], SellAmount: v[3], NetAmount: v[4], Meta: itemMeta})
	}
	return out, nil
}
func (c *Client) MarketBillboardDetail(ctx context.Context, symbol, date, reason string) (foundation.MarketBillboardDetail, foundation.SourceMeta, error) {
	n, e := foundation.NormalizeSymbol(symbol)
	if e != nil {
		return foundation.MarketBillboardDetail{}, foundation.SourceMeta{}, e
	}
	d, e := time.Parse("2006-01-02", date)
	if e != nil || strings.TrimSpace(reason) == "" {
		return foundation.MarketBillboardDetail{}, foundation.SourceMeta{}, fmt.Errorf("invalid date/reason")
	}
	rows, e := c.query(ctx, "top_inst", map[string]any{"trade_date": d.Format("20060102"), "ts_code": n.Canonical}, "", 5*time.Minute)
	if e != nil {
		return foundation.MarketBillboardDetail{}, foundation.SourceMeta{}, e
	}
	if len(rows) >= 10000 {
		return foundation.MarketBillboardDetail{}, foundation.SourceMeta{}, fmt.Errorf("top_inst may be truncated")
	}
	return parseBillboardSeats(rows, n.Canonical, date, reason)
}
func parseBillboardSeats(rows []Row, symbol, date, reason string) (foundation.MarketBillboardDetail, foundation.SourceMeta, error) {
	meta := billboardMeta(date, "tushare:top_inst")
	meta.SourceURL = "https://tushare.pro/document/2?doc_id=107"
	meta.AvailableFields = []string{"buy_seats", "sell_seats"}
	meta.FallbackReason = "同日同原因买卖席位；上游未披露的对侧金额/比例保留缺失，不计为零"
	d := foundation.MarketBillboardDetail{Symbol: symbol, TradeDate: date, Reason: reason, BuySeats: []foundation.MarketBillboardSeat{}, SellSeats: []foundation.MarketBillboardSeat{}, Meta: meta}
	for _, r := range rows {
		if str(r, "ts_code") != symbol || str(r, "trade_date") != strings.ReplaceAll(date, "-", "") || strings.TrimSpace(str(r, "reason")) != strings.TrimSpace(reason) {
			continue
		}

		name := strings.TrimSpace(str(r, "exalter"))
		side := str(r, "side")
		if name == "" || (side != "0" && side != "1") {
			return d, meta, fmt.Errorf("invalid billboard seat identity")
		}
		values := make([]float64, 5)
		available := []string{"name", "institution"}
		upstream := []string{"buy", "sell", "buy_rate", "sell_rate", "net_buy"}
		keys := []string{"buy_amount", "sell_amount", "buy_ratio", "sell_ratio", "net_amount"}
		for i, k := range upstream {
			if r[k] == nil {
				continue
			}
			v, e := requiredNumber(r, k)
			if e != nil || i < 4 && v < 0 {
				return d, meta, fmt.Errorf("invalid seat amount")
			}
			values[i] = v
			available = append(available, keys[i])
		}
		if side == "0" && r["buy"] == nil || side == "1" && r["sell"] == nil {
			return d, meta, fmt.Errorf("missing ranked side amount")
		}
		seat := foundation.MarketBillboardSeat{Name: name, BuyAmount: values[0], SellAmount: values[1], BuyRatio: values[2], SellRatio: values[3], NetAmount: values[4], Institution: strings.Contains(name, "机构专用"), Source: "tushare:top_inst", Direction: "buy", AvailableFields: available}

		if side == "0" {
			d.BuySeats = append(d.BuySeats, seat)
		} else {
			seat.Direction = "sell"
			d.SellSeats = append(d.SellSeats, seat)
		}
	}
	if len(d.BuySeats) == 0 || len(d.SellSeats) == 0 || len(d.BuySeats) > 5 || len(d.SellSeats) > 5 {
		return d, meta, fmt.Errorf("同日同原因买卖席位未齐备或有重复；不混用其他上榜原因")
	}
	sort.SliceStable(d.BuySeats, func(i, j int) bool { return d.BuySeats[i].BuyAmount > d.BuySeats[j].BuyAmount })
	sort.SliceStable(d.SellSeats, func(i, j int) bool { return d.SellSeats[i].SellAmount > d.SellSeats[j].SellAmount })
	for i := range d.BuySeats {
		d.BuySeats[i].Rank = i + 1
	}
	for i := range d.SellSeats {
		d.SellSeats[i].Rank = i + 1
	}
	return d, meta, nil
}
