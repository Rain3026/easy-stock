package datahub

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type QuoteProvider interface {
	Realtime(context.Context, []string) ([]foundation.Quote, error)
}
type QuoteRoute struct {
	Family   string
	Provider QuoteProvider
}
type quoteCache struct {
	values []foundation.Quote
	at     time.Time
}
type quoteFlight struct {
	done   chan struct{}
	values []foundation.Quote
	err    error
}
type circuit struct {
	failures int
	until    time.Time
}
type QuoteRouter struct {
	mu       sync.Mutex
	routes   []QuoteRoute
	cache    map[string]quoteCache
	flights  map[string]*quoteFlight
	circuits map[string]circuit
	now      func() time.Time
}

func NewQuoteRouter(routes []QuoteRoute) *QuoteRouter {
	return &QuoteRouter{routes: routes, cache: map[string]quoteCache{}, flights: map[string]*quoteFlight{}, circuits: map[string]circuit{}, now: time.Now}
}
func (q *QuoteRouter) Realtime(ctx context.Context, symbols []string) ([]foundation.Quote, error) {
	keys := []string{}
	seen := map[string]bool{}
	for _, s := range symbols {
		n, e := foundation.NormalizeSymbol(s)
		if e != nil {
			return nil, e
		}
		if !seen[n.Canonical] {
			keys = append(keys, n.Canonical)
			seen[n.Canonical] = true
		}
	}
	if len(keys) == 0 || len(keys) > 80 {
		return nil, fmt.Errorf("quote request must contain 1 to 80 symbols")
	}
	sort.Strings(keys)
	key := strings.Join(keys, ",")
	q.mu.Lock()
	cached, ok := q.cache[key]
	if ok && q.now().Sub(cached.at) < 3*time.Second {
		q.mu.Unlock()
		return append([]foundation.Quote(nil), cached.values...), nil
	}
	if f, ok := q.flights[key]; ok {
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.done:
			return append([]foundation.Quote(nil), f.values...), f.err
		}
	}
	f := &quoteFlight{done: make(chan struct{})}
	q.flights[key] = f
	q.mu.Unlock()
	values, err := q.load(ctx, keys)
	q.mu.Lock()
	f.values, f.err = values, err
	if err == nil {
		if len(q.cache) > 512 {
			q.cache = map[string]quoteCache{}
		}
		q.cache[key] = quoteCache{values: append([]foundation.Quote(nil), values...), at: q.now()}
	}
	delete(q.flights, key)
	close(f.done)
	q.mu.Unlock()
	return append([]foundation.Quote(nil), values...), err
}
func (q *QuoteRouter) load(ctx context.Context, keys []string) ([]foundation.Quote, error) {
	byID := map[string]foundation.Quote{}
	failed := []string{}
	for _, route := range q.routes {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		q.mu.Lock()
		state := q.circuits[route.Family]
		q.mu.Unlock()
		if q.now().Before(state.until) {
			failed = append(failed, route.Family+" 冷却中")
			continue
		}
		missing := []string{}
		for _, k := range keys {
			if _, ok := byID[k]; !ok {
				missing = append(missing, k)
			}
		}
		if len(missing) == 0 {
			break
		}
		c, cancel := context.WithTimeout(ctx, 4*time.Second)
		values, err := route.Provider.Realtime(c, missing)
		cancel()
		accepted := 0
		wanted := map[string]bool{}
		for _, k := range missing {
			wanted[k] = true
		}
		if err == nil {
			for _, v := range values {
				if !wanted[v.Symbol] || v.Price <= 0 || v.PreviousClose <= 0 || math.IsNaN(v.Price) || math.IsInf(v.Price, 0) || math.IsNaN(v.PreviousClose) || math.IsInf(v.PreviousClose, 0) || v.Meta.Stale {
					continue
				}
				o := SessionObservation(1, v.TradeTime, q.now(), true)
				if o.Freshness != "verified" || o.Stale {
					continue
				}
				v.Meta.Family = route.Family
				v.Meta.Channel = "http"
				v.Meta.AmountUnit = "CNY"
				v.Meta.VolumeUnit = "shares"
				if len(failed) > 0 {
					v.Meta.FallbackReason = strings.Join(failed, "；") + "，已切换到 " + route.Family
				}
				byID[v.Symbol] = v
				accepted++
			}
		}
		q.mu.Lock()
		if accepted == len(missing) {
			q.circuits[route.Family] = circuit{}
		} else {
			state.failures++
			if state.failures >= 2 {
				state.until = q.now().Add(30 * time.Second)
			}
			q.circuits[route.Family] = state
		}
		q.mu.Unlock()
		if accepted < len(missing) {
			failed = append(failed, route.Family+" 缺少有效同时点报价")
		}
	}
	out := make([]foundation.Quote, 0, len(keys))
	missing := []string{}
	for _, k := range keys {
		if v, ok := byID[k]; ok {
			out = append(out, v)
		} else {
			missing = append(missing, k)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("全部报价来源未返回有效数据")
	}
	if len(missing) > 0 {
		for i := range out {
			out[i].Meta.MissingSymbols = append([]string(nil), missing...)
		}
	}
	return out, nil
}
