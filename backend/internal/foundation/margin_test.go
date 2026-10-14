package foundation

import (
	"strings"
	"testing"
)

func TestMarginRequiresSameDateCompleteUniverse(t *testing.T) {
	rows := []MarginExchangePoint{}
	for i, date := range []string{"2026-08-10", "2026-08-11", "2026-08-12"} {
		for _, exchange := range []string{"SSE", "SZSE", "BSE"} {
			if i == 2 && exchange == "SZSE" {
				continue
			}
			rows = append(rows, MarginExchangePoint{exchange, MarketMarginPoint{TradeDate: date, FinancingBalance: float64(100 + i*10), SecuritiesLendingBalance: 10, MarginBalance: float64(110 + i*10)}})
		}
	}
	got, meta, e := CompleteMarginSeries(rows, 10, SourceMeta{})
	if e != nil || len(got) != 1 || got[0].MarginBalance != 360 || got[0].MarginBalanceChange != 30 || meta.TradeDate != "2026-08-11" || !strings.Contains(meta.FallbackReason, "未齐") {
		t.Fatalf("%+v %+v %v", got, meta, e)
	}
	if _, _, e = CompleteMarginSeries(append(rows, rows[0]), 10, SourceMeta{}); e == nil {
		t.Fatal("accepted duplicate exchange")
	}
	rows[0].Point.MarginBalance = 999
	if _, _, e = CompleteMarginSeries(rows, 10, SourceMeta{}); e == nil {
		t.Fatal("accepted inconsistent units")
	}
}
