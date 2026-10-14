package marketpulse

import (
	"fmt"
	"testing"
)

func TestBillboardSeatsDoNotMixReasonOrDateAndAllowAnonymousInstitutions(t *testing.T) {
	rows := []Row{}
	for side := 0; side < 2; side++ {
		for i := 0; i < 5; i++ {
			rows = append(rows, Row{"trade_date": "20260811", "ts_code": "600000.SH", "reason": "连续三日", "side": fmt.Sprint(side), "exalter": "机构专用", "buy": float64(100 + i), "sell": float64(10 + i), "buy_rate": 1.0, "sell_rate": 1.0, "net_buy": 90.0})
		}
	}
	rows = append(rows, Row{"trade_date": "20260811", "ts_code": "600000.SH", "reason": "单日", "side": "0", "exalter": "不应混用"})
	d, _, e := parseBillboardSeats(rows, "600000.SH", "2026-08-11", "连续三日")
	if e != nil || len(d.BuySeats) != 5 || d.BuySeats[0].BuyAmount != 104 || !d.BuySeats[0].Institution {
		t.Fatal(d, e)
	}
	if _, _, e = parseBillboardSeats(rows, "600000.SH", "2026-08-10", "连续三日"); e == nil {
		t.Fatal("cross date accepted")
	}
	if _, _, e = parseBillboardSeats(append(rows, rows[0]), "600000.SH", "2026-08-11", "连续三日"); e == nil {
		t.Fatal("duplicate overflow accepted")
	}
}
func TestBillboardRejectsMissingNumericAndWrongDate(t *testing.T) {
	r := Row{"trade_date": "20260811", "ts_code": "600000.SH", "reason": "日涨幅", "name": "样本", "close": 10.0, "pct_change": 1.0, "turnover_rate": 2.0, "l_buy": 100.0, "l_sell": 10.0, "net_amount": 90.0}
	meta := billboardMeta("2026-08-11", "test")
	if _, e := parseBillboardRows([]Row{r}, "2026-08-11", meta); e != nil {
		t.Fatal(e)
	}
	if _, e := parseBillboardRows([]Row{r}, "2026-08-10", meta); e == nil {
		t.Fatal("wrong date accepted")
	}
	delete(r, "l_buy")
	if _, e := parseBillboardRows([]Row{r}, "2026-08-11", meta); e == nil {
		t.Fatal("missing amount became zero")
	}
}
func TestBillboardNullOppositeSideIsMissingNotZero(t *testing.T) {
	rows := []Row{{"trade_date": "20260811", "ts_code": "600000.SH", "reason": "日涨幅", "side": "0", "exalter": "买方", "buy": 100.0, "buy_rate": 1.0, "sell": nil, "sell_rate": nil, "net_buy": 100.0}, {"trade_date": "20260811", "ts_code": "600000.SH", "reason": "日涨幅", "side": "1", "exalter": "卖方", "buy": nil, "buy_rate": nil, "sell": 90.0, "sell_rate": 1.0, "net_buy": -90.0}}
	d, _, e := parseBillboardSeats(rows, "600000.SH", "2026-08-11", "日涨幅")
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range d.BuySeats[0].AvailableFields {
		if key == "sell_amount" || key == "sell_ratio" {
			t.Fatal("unavailable field declared available")
		}
	}
}
