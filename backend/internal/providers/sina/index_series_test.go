package sina

import (
	"testing"
	"time"

	"easy-stock/backend/internal/foundation"
)

func TestNasdaqHistoryPreservesCutoffAndAggregates(t *testing.T) {
	const body = `/*not executable*/ IO.XSRV2.CallbackList([{"d":"2026-10-08","o":"30000","h":"30200","l":"29900","c":"30100"},{"d":"2026-10-09","o":"30100","h":"30400","l":"30000","c":"30300"},{"d":"2026-10-12","o":"30200","h":"30600","l":"30100","c":"30500"}]);`
	now, _ := time.Parse(time.RFC3339, "2026-10-14T07:00:00+08:00")
	for _, period := range []string{"day", "week", "month"} {
		v, e := parseNasdaqHistory(body, period, 120, foundation.SourceMeta{Source: "sina:index-kline"}, now)
		if e != nil || v.Index.ID != "nasdaq100" || v.Meta.TradeDate != "2026-10-12" || !v.Meta.Stale {
			t.Fatal(v, e)
		}
		if period == "day" && len(v.Lines) != 3 {
			t.Fatal(v.Lines)
		}
		if period == "week" && (len(v.Lines) != 2 || v.Lines[0].Open != 30000 || v.Lines[0].High != 30400 || v.Lines[0].Close != 30300) {
			t.Fatal(v.Lines)
		}
		if period == "month" && (len(v.Lines) != 1 || v.Lines[0].Low != 29900 || v.Lines[0].Close != 30500) {
			t.Fatal(v.Lines)
		}
		for _, line := range v.Lines {
			if line.Amount != 0 || line.Volume != 0 {
				t.Fatal("unverified quantity invented")
			}
		}
	}
	if _, e := parseNasdaqHistory(body, "day", 120, foundation.SourceMeta{}, now.AddDate(0, 0, 7)); e == nil {
		t.Fatal("old history accepted")
	}
}

func TestNasdaqHistoryRejectsMalformedAndConflictingData(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T07:00:00+08:00")
	for _, body := range []string{`null`, `IO.XSRV2.CallbackList([])`, `IO.XSRV2.CallbackList([{"d":"2026-10-12","o":"NaN","h":"3","l":"1","c":"2"}])`, `IO.XSRV2.CallbackList([{"d":"2026-10-12","o":"2","h":"3","l":"1","c":"2"},{"d":"2026-10-12","o":"2","h":"3","l":"1","c":"3"}])`} {
		if _, e := parseNasdaqHistory(body, "day", 120, foundation.SourceMeta{}, now); e == nil {
			t.Fatal("invalid payload accepted")
		}
	}
}
