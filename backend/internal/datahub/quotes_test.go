package datahub

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type quoteFunc func(context.Context, []string) ([]foundation.Quote, error)

func (f quoteFunc) Realtime(ctx context.Context, ids []string) ([]foundation.Quote, error) {
	return f(ctx, ids)
}
func TestQuoteRouterRejectsStalePartialAndRecovers(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-13T11:00:00+08:00")
	var primary atomic.Int32
	q := NewQuoteRouter([]QuoteRoute{
		{"tushare", quoteFunc(func(ctx context.Context, ids []string) ([]foundation.Quote, error) {
			primary.Add(1)
			return []foundation.Quote{{Symbol: ids[0], Price: 12, PreviousClose: 10, TradeTime: now.Add(-time.Hour)}}, nil
		})},
		{"sina", quoteFunc(func(ctx context.Context, ids []string) ([]foundation.Quote, error) {
			return []foundation.Quote{{Symbol: ids[0], Price: 11, PreviousClose: 10, TradeTime: now}}, nil
		})},
		{"tencent", quoteFunc(func(ctx context.Context, ids []string) ([]foundation.Quote, error) {
			out := []foundation.Quote{}
			for _, id := range ids {
				out = append(out, foundation.Quote{Symbol: id, Price: 11, PreviousClose: 10, TradeTime: now})
			}
			return out, nil
		})},
	})
	q.now = func() time.Time { return now }
	out, e := q.Realtime(context.Background(), []string{"000001.SZ", "600176.SH"})
	if e != nil || len(out) != 2 || out[0].Meta.Family != "sina" || out[1].Meta.Family != "tencent" || out[1].Meta.FallbackReason == "" {
		t.Fatal(out, e)
	}
	_, _ = q.Realtime(context.Background(), []string{"000001.SZ", "600176.SH"})
	if primary.Load() != 1 {
		t.Fatal("cache not used")
	}
	now = now.Add(4 * time.Second)
	_, _ = q.Realtime(context.Background(), []string{"000001.SZ", "600176.SH"})
	now = now.Add(4 * time.Second)
	_, _ = q.Realtime(context.Background(), []string{"000001.SZ", "600176.SH"})
	if primary.Load() != 2 {
		t.Fatal("failed primary not cooled down")
	}
	now = now.Add(35 * time.Second)
	_, _ = q.Realtime(context.Background(), []string{"000001.SZ", "600176.SH"})
	if primary.Load() != 3 {
		t.Fatal("recovery probe not attempted")
	}
}
func TestQuoteRouterJoinsConcurrentRequests(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-13T11:00:00+08:00")
	var calls atomic.Int32
	q := NewQuoteRouter([]QuoteRoute{{"sina", quoteFunc(func(ctx context.Context, ids []string) ([]foundation.Quote, error) {
		calls.Add(1)
		time.Sleep(15 * time.Millisecond)
		return []foundation.Quote{{Symbol: ids[0], Price: 1, PreviousClose: 1, TradeTime: now}}, nil
	})}})
	q.now = func() time.Time { return now }
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := q.Realtime(context.Background(), []string{"600176.SH"})
			if e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestQuoteRouterMarksMissingSymbolsAndAllSourceFailure(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-13T11:00:00+08:00")
	q := NewQuoteRouter([]QuoteRoute{{"sina", quoteFunc(func(ctx context.Context, ids []string) ([]foundation.Quote, error) {
		return []foundation.Quote{{Symbol: ids[0], Price: 1, PreviousClose: 1, TradeTime: now}}, nil
	})}})
	q.now = func() time.Time { return now }
	v, e := q.Realtime(context.Background(), []string{"000001.SZ", "600176.SH"})
	if e != nil || len(v) != 1 || len(v[0].Meta.MissingSymbols) != 1 {
		t.Fatal(v, e)
	}
	q = NewQuoteRouter([]QuoteRoute{{"sina", quoteFunc(func(ctx context.Context, ids []string) ([]foundation.Quote, error) {
		return nil, errors.New("403 secret")
	})}})
	if _, e = q.Realtime(context.Background(), []string{"600176.SH"}); e == nil || e.Error() != "全部报价来源未返回有效数据" {
		t.Fatal(e)
	}
}
