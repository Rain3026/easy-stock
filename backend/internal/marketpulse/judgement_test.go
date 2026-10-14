package marketpulse

import (
	"easy-stock/backend/internal/foundation"
	"testing"
	"time"
)

func ruleFixture() (Snapshot, []foundation.MarketIndexSnapshot, time.Time) {
	at := parseTime("2026-10-13 15:00:00")
	p := Snapshot{Date: "2026-10-13", Session: "closed", At: "2026-10-13 15:00:00", Actual: ptr(19800), Mean20: ptr(19200), ObservationLine: 18100, FetchedAt: at, Breadth: &Breadth{Date: "2026-10-13", Up: 3100, Down: 1800, Flat: 100}}
	indexes := []foundation.MarketIndexSnapshot{}
	for _, id := range []string{"sse", "szse", "chinext"} {
		indexes = append(indexes, foundation.MarketIndexSnapshot{ID: id, Market: "CN", TradeTime: at, ChangePercent: 1})
	}
	return p, indexes, at
}
func TestRuleMissingAndBroadDecline(t *testing.T) {
	p, i, at := ruleFixture()
	j := Judge(p, i, nil, foundation.SourceMeta{}, at)
	if j.Label != "中性偏多" || j.Version != RuleVersion || j.Range[1] != 40 {
		t.Fatal(j)
	}
	p.Breadth.Up = 500
	j = Judge(p, i, nil, foundation.SourceMeta{}, at)
	if j.Label != "偏空" || j.Range[0] != 0 {
		t.Fatal(j)
	}
	p.Breadth = nil
	if Judge(p, i, nil, foundation.SourceMeta{}, at).Range != nil {
		t.Fatal("missing breadth")
	}
	p, i, at = ruleFixture()
	i[0].TradeTime = at.AddDate(0, 0, -1)
	if Judge(p, i, nil, foundation.SourceMeta{}, at).Range != nil {
		t.Fatal("cross date indexes")
	}
}
func TestRuleIntradayUsesForecastNotPartialAmount(t *testing.T) {
	p, i, _ := ruleFixture()
	at := parseTime("2026-10-13 11:00:00")
	p.Session = "intraday"
	p.At = "2026-10-13 11:00:00"
	p.FetchedAt = at
	p.Actual = ptr(1000)
	p.Forecast = ptr(19800)
	if Judge(p, i, nil, foundation.SourceMeta{}, at).Label != "中性偏多" {
		t.Fatal("used partial amount")
	}
	p.Forecast = nil
	if Judge(p, i, nil, foundation.SourceMeta{}, at).Range != nil {
		t.Fatal("missing forecast")
	}
	p.Forecast = ptr(19800)
	p.At = "2026-10-13 09:45:00"
	if Judge(p, i, nil, foundation.SourceMeta{}, at).Label != "早盘观察" {
		t.Fatal("early session")
	}
	p.At = "2026-10-13 11:00:00"
	p.FetchedAt = at.Add(-8 * time.Minute)
	if Judge(p, i, nil, foundation.SourceMeta{}, at).Range != nil {
		t.Fatal("expired observation")
	}
}
func TestRuleObservationLineAndPartialEmotion(t *testing.T) {
	p, i, at := ruleFixture()
	p.ObservationLine = 25000
	if Judge(p, i, nil, foundation.SourceMeta{}, at).Label != "中性观望" {
		t.Fatal("ignored line")
	}
	p.ObservationLine = 18100
	p.Emotion = &Emotion{Date: p.Date, Height: 5, BrokenRate: 10, Partial: true}
	themes := []foundation.ThemeOverview{{TradeDate: p.Date, ActiveDays: 3, DailyStrengthScore: 90}}
	if Judge(p, i, themes, foundation.SourceMeta{}, at).Range[1] != 40 {
		t.Fatal("partial emotion used")
	}
	p.Emotion.Partial = false
	if Judge(p, i, themes, foundation.SourceMeta{}, at).Range[1] != 50 {
		t.Fatal("complete signals ignored")
	}
}
