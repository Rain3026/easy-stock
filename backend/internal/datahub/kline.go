package datahub

import (
	"context"
	"easy-stock/backend/internal/foundation"
	"fmt"
	"time"
)

type KLineProvider interface {
	KLine(context.Context, string, string, int) ([]foundation.KLine, error)
}
type KLineChain struct{ Providers []KLineProvider }

func (c *KLineChain) KLine(ctx context.Context, symbol, period string, limit int) ([]foundation.KLine, error) {
	var last error
	for i, p := range c.Providers {
		sc, cancel := context.WithTimeout(ctx, 5*time.Second)
		bars, e := p.KLine(sc, symbol, period, limit)
		cancel()
		if e == nil && len(bars) > 0 {
			if i > 0 {
				for j := range bars {
					bars[j].Meta.FallbackReason = "前序K线备用无有效数据，整段切换；" + bars[j].Meta.FallbackReason
				}
			}
			return bars, nil
		}
		last = e
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if last == nil {
		last = fmt.Errorf("K线备用无有效数据")
	}
	return nil, last
}
func (c *KLineChain) AdjustedKLine(ctx context.Context, symbol string, limit int) ([]foundation.KLine, error) {
	return c.KLine(ctx, symbol, "day", limit)
}
