package marketpulse

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type amountFixture struct {
	gap     bool
	badUnit bool
	fail    bool
	family  string
}

type amountFunc func(context.Context, string, string, int) ([]foundation.KLine, error)

func (f amountFunc) MarketAmountBars(c context.Context, s, p string, n int) ([]foundation.KLine, error) {
	return f(c, s, p, n)
}

type breadthFunc func(context.Context, string, string, time.Time) (*foundation.MarketBreadth, error)

func (f breadthFunc) MarketBreadth(c context.Context, d, s string, n time.Time) (*foundation.MarketBreadth, error) {
	return f(c, d, s, n)
}

func TestIndependentBreadthDoesNotExhaustAmountFallbackBudget(t *testing.T) {
	c := New(func() string { return "" }, "")
	started := make(chan struct{})
	var once sync.Once
	c.SecondaryFallback = amountFunc(func(ctx context.Context, code, period string, limit int) ([]foundation.KLine, error) {
		once.Do(func() { close(started) })
		return (amountFixture{family: "sina"}).MarketAmountBars(ctx, code, period, limit)
	})
	c.BreadthFallback = breadthFunc(func(ctx context.Context, _, _ string, _ time.Time) (*foundation.MarketBreadth, error) {
		select {
		case <-started:
			return &foundation.MarketBreadth{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v := c.Load(ctx, parseTime("2026-10-13 11:00:00"))
	if ctx.Err() != nil || v.Forecast == nil || v.Breadth == nil {
		t.Fatalf("breadth blocked amount fallback: %+v", v)
	}
}

func TestBreadthSecondaryPreservesFailureAndRecoversPrimary(t *testing.T) {
	c := New(func() string { return "" }, "")
	failed := true
	c.BreadthFallback = breadthFunc(func(context.Context, string, string, time.Time) (*foundation.MarketBreadth, error) {
		if failed {
			return nil, errors.New("EOF")
		}
		return &foundation.MarketBreadth{Source: "eastmoney", Up: 4000}, nil
	})
	c.BreadthSecondary = breadthFunc(func(context.Context, string, string, time.Time) (*foundation.MarketBreadth, error) {
		return &foundation.MarketBreadth{Source: "sina", Down: 4000, Meta: foundation.SourceMeta{Family: "sina"}}, nil
	})
	ctx := context.Background()
	now := parseTime("2026-10-14 14:35:00")
	b, e := c.loadBreadthFallback(ctx, "2026-10-14", "intraday", now)
	if e != nil || b.Source != "sina" || b.Down != 4000 || b.Up != 0 || !strings.Contains(b.Meta.FallbackReason, "东方财富：EOF") {
		t.Fatal("did not switch complete breadth", b, e)
	}
	failed = false
	b, e = c.loadBreadthFallback(ctx, "2026-10-14", "intraday", now)
	if e != nil || b.Source != "eastmoney" || b.Up != 4000 || b.Down != 0 {
		t.Fatal("did not return to preferred breadth", b, e)
	}
	c.BreadthSecondary = breadthFunc(func(context.Context, string, string, time.Time) (*foundation.MarketBreadth, error) {
		return nil, errors.New("missing page")
	})
	failed = true
	if b, e = c.loadBreadthFallback(ctx, "2026-10-14", "intraday", now); b != nil || e == nil {
		t.Fatal("all failures yielded fabricated breadth")
	}
}

func (f amountFixture) MarketAmountBars(ctx context.Context, code, period string, limit int) ([]foundation.KLine, error) {
	if f.fail {
		return nil, errors.New("source down")
	}
	out := []foundation.KLine{}
	meta := foundation.SourceMeta{Family: "eastmoney", Adjustment: "none", AmountUnit: "CNY"}
	if f.family != "" {
		meta.Family = f.family
	}
	if f.badUnit {
		meta.AmountUnit = ""
	}
	target := parseTime("2026-10-13 15:00:00")
	if period == "day" {
		for day := target; len(out) < 24; day = day.AddDate(0, 0, -1) {
			if foundation.IsAStockTradingDay(day) {
				out = append([]foundation.KLine{{Symbol: code, Time: day, Amount: 4800e8, Meta: meta}}, out...)
			}
		}
	} else {
		for _, date := range []string{"2026-10-12", "2026-10-13"} {
			for minute := 575; minute <= 900; minute += 5 {
				if minute > 690 && minute <= 780 {
					continue
				}
				if f.gap && date == "2026-10-13" && code == "399107.SZ" && minute == 600 {
					continue
				}
				at := parseTime(date + " 00:00:00").Add(time.Duration(minute) * time.Minute)
				amount := 100e8
				if date == "2026-10-13" {
					amount = 110e8
				}
				out = append(out, foundation.KLine{Symbol: code, Time: at, Amount: amount, Meta: meta})
			}
		}
	}
	return out, nil
}

func TestSecondaryFallbackUsesWholeGroupAndPreservesFailedSource(t *testing.T) {
	c := New(func() string { return "" }, "")
	c.Fallback = amountFixture{fail: true}
	c.SecondaryFallback = amountFixture{family: "sina"}
	v := c.Load(context.Background(), parseTime("2026-10-13 11:00:00"))
	if v.Actual == nil || v.Forecast == nil || *v.Forecast != 10560 || v.Mean20 == nil || !strings.Contains(v.Source, "新浪") || !strings.Contains(strings.Join(v.Gaps, " "), "东方财富日量能备用不可用") {
		t.Fatalf("incomplete secondary switch: %+v", v)
	}
	if v.Breadth != nil || v.Emotion != nil {
		t.Fatal("amount fallback fabricated breadth or emotion")
	}
}

func TestSecondaryDoesNotAdmitBadIdentityOrIncompleteMinutes(t *testing.T) {
	for _, f := range []amountFixture{{family: "sina", gap: true}, {family: "sina", badUnit: true}, {family: "eastmoney"}} {
		c := New(func() string { return "" }, "")
		c.Fallback = amountFixture{fail: true}
		c.SecondaryFallback = f
		v := c.Load(context.Background(), parseTime("2026-10-13 11:00:00"))
		if v.Forecast != nil {
			t.Fatal("unsafe secondary forecast admitted")
		}
	}
}

func TestPrimaryRecoveryReturnsToPrimaryWholeGroup(t *testing.T) {
	c := fixtureClient(t, func(api string, params map[string]any) ([]string, [][]any) {
		if api != "index_daily" {
			return nil, nil
		}
		bars, _ := (amountFixture{}).MarketAmountBars(context.Background(), params["ts_code"].(string), "day", 80)
		rows := [][]any{}
		for _, b := range bars {
			rows = append(rows, []any{b.Time.Format("20060102"), b.Amount / 1000})
		}
		return []string{"trade_date", "amount"}, rows
	})
	recovered := false
	c.Token = func() string {
		if recovered {
			return "fixture-token"
		}
		return ""
	}
	c.Fallback = amountFixture{fail: true}
	c.SecondaryFallback = amountFixture{family: "sina"}
	now := parseTime("2026-10-13 21:00:00")
	if v := c.Load(context.Background(), now); !strings.Contains(v.Source, "新浪") {
		t.Fatal("secondary did not take over")
	}
	recovered = true
	v := c.Load(context.Background(), now)
	if !strings.Contains(v.Source, "Tushare") || v.FallbackReason != "" || v.Actual == nil || *v.Actual != 9600 {
		t.Fatalf("primary recovery retained secondary: %+v", v)
	}
}
func TestBackupRestoresVolumeWithoutInventingBreadth(t *testing.T) {
	c := New(func() string { return "" }, "")
	c.Fallback = amountFixture{}
	v := c.Load(context.Background(), parseTime("2026-10-13 11:00:00"))
	if v.Actual == nil || v.Forecast == nil || *v.Forecast != 10560 || v.Mean20 == nil || len(v.History) != 20 || v.FallbackReason == "" {
		t.Fatalf("fallback not complete: %+v", v)
	}
	if v.Breadth != nil || v.Emotion != nil {
		t.Fatal("fallback fabricated unrelated data")
	}
	if v.At != "2026-10-13 11:00:00" {
		t.Fatal(v.At)
	}
}
func TestBackupRejectsMissingMinuteAndUnknownUnit(t *testing.T) {
	for _, f := range []amountFixture{{gap: true}, {badUnit: true}, {fail: true}} {
		c := New(func() string { return "" }, "")
		c.Fallback = f
		v := c.Load(context.Background(), parseTime("2026-10-13 11:00:00"))
		if v.Forecast != nil {
			t.Fatal("invalid backup admitted")
		}
		if len(v.Gaps) == 0 {
			t.Fatal("missing provenance")
		}
	}
}
func TestBackupClosedSessionUsesActualNotForecast(t *testing.T) {
	c := New(func() string { return "" }, "")
	c.Fallback = amountFixture{}
	v := c.Load(context.Background(), parseTime("2026-10-13 21:00:00"))
	if v.Actual == nil || *v.Actual != 9600 || v.Forecast != nil || v.Mean20 == nil {
		t.Fatalf("unexpected closed snapshot %+v", v)
	}
}
