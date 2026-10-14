package marketpulse

import (
	"context"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
)

// Validation captures public evidence without a persistent cache or business writes.
// Its timestamps are wall-clock observations, never a historical replay cutoff.
type Validation struct {
	BreadthBackup         *foundation.MarketBreadth             `json:"breadth_backup,omitempty"`
	BreadthBackupError    string                                `json:"breadth_backup_error,omitempty"`
	BreadthSecondary      *foundation.MarketBreadth             `json:"breadth_secondary,omitempty"`
	BreadthSecondaryError string                                `json:"breadth_secondary_error,omitempty"`
	BreadthEvidence       map[string][]foundation.BreadthMember `json:"breadth_evidence,omitempty"`
	Mode                  string                                `json:"mode"`
	StartedAt             time.Time                             `json:"started_at"`
	FinishedAt            time.Time                             `json:"finished_at"`
	Window                string                                `json:"window"`
	Eligible              bool                                  `json:"eligible_live_window"`
	Primary               *Snapshot                             `json:"primary,omitempty"`
	Backup                *Snapshot                             `json:"backup,omitempty"`
	Secondary             *Snapshot                             `json:"secondary,omitempty"`
	Streams               []ValidationStream                    `json:"streams,omitempty"`
	Checks                map[string]bool                       `json:"checks,omitempty"`
}

type ValidationStream struct {
	Source      string    `json:"source"`
	API         string    `json:"api"`
	Symbol      string    `json:"symbol"`
	Date        string    `json:"date"`
	Unit        string    `json:"amount_unit"`
	CollectedAt time.Time `json:"collected_at"`
	Rows        []Row     `json:"rows"`
	Error       string    `json:"error,omitempty"`
}

func ValidationWindow(now time.Time) string {
	now = now.In(foundation.AStockLocation)
	if !foundation.IsAStockTradingDay(now) {
		return "outside"
	}
	m := now.Hour()*60 + now.Minute()
	switch {
	case m >= 575 && m < 585:
		return "opening"
	case m >= 630 && m < 660:
		return "morning"
	case m >= 870 && m < 895:
		return "afternoon"
	case m >= 930 && m < 1020:
		return "close"
	default:
		return "outside"
	}
}

func (c *Client) CaptureValidation(ctx context.Context) Validation {
	// Capture uses a private, non-persistent cache. Retain the first response
	// even if other providers consume its TTL before raw evidence is serialized.
	// Production cache policy and concurrent callers are unaffected.
	isolated := New(c.Token, "")
	isolated.HTTP, isolated.URL = c.HTTP, c.URL
	isolated.Fallback, isolated.SecondaryFallback = c.Fallback, c.SecondaryFallback
	isolated.BreadthFallback, isolated.BreadthSecondary = c.BreadthFallback, c.BreadthSecondary
	isolated.validationOnly = true
	return isolated.captureValidation(ctx, time.Now())
}

func (c *Client) captureValidation(ctx context.Context, now time.Time) Validation {
	v := Validation{Mode: "live_read_only", StartedAt: now, Window: ValidationWindow(now)}
	v.Eligible = v.Window != "outside"
	if !v.Eligible {
		v.FinishedAt = time.Now()
		return v
	}
	primaryCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
	p := c.loadPrimary(primaryCtx, now)
	cancel()
	v.Primary = &p
	if c.Fallback != nil {
		backupCtx, cancel := context.WithTimeout(ctx, 14*time.Second)
		b := c.loadFallback(backupCtx, now)
		cancel()
		v.Backup = &b
	}
	if c.SecondaryFallback != nil {
		secondaryCtx, cancel := context.WithTimeout(ctx, 14*time.Second)
		b := c.loadSecondaryFallback(secondaryCtx, now)
		cancel()
		v.Secondary = &b
	}
	target, session := Target(now)
	if c.BreadthFallback != nil {
		bc, done := context.WithTimeout(ctx, 15*time.Second)
		b, e := c.BreadthFallback.MarketBreadth(bc, target.Format("2006-01-02"), session, now)
		done()
		v.BreadthBackup = b
		if e != nil {
			v.BreadthBackupError = e.Error()
		}
	}
	if c.BreadthSecondary != nil {
		bc, done := context.WithTimeout(ctx, 15*time.Second)
		b, e := c.BreadthSecondary.MarketBreadth(bc, target.Format("2006-01-02"), session, now)
		done()
		v.BreadthSecondary = b
		if e != nil {
			v.BreadthSecondaryError = e.Error()
		}
	}
	v.BreadthEvidence = map[string][]foundation.BreadthMember{}
	for source, b := range map[string]*foundation.MarketBreadth{"tushare": p.Breadth, "eastmoney": v.BreadthBackup, "sina": v.BreadthSecondary} {
		if b != nil && len(b.Members) > 0 {
			v.BreadthEvidence[source] = b.Members
		}
	}
	date := target.Format("2006-01-02")
	previous := foundation.LatestCompletedAStockSession(parseTime(date + " 09:00:00")).Format("2006-01-02")
	add := func(source, api, code, day, unit string, rows []Row, err error) {
		// Retain only fields required for independent amount/time checks.
		safe := make([]Row, 0, len(rows))
		for _, r := range rows {
			n := Row{}
			for _, key := range []string{"ts_code", "trade_date", "trade_time", "time", "amount"} {
				if value, exists := r[key]; exists {
					n[key] = value
				}
			}
			safe = append(safe, n)
		}
		s := ValidationStream{Source: source, API: api, Symbol: code, Date: day, Unit: unit, CollectedAt: time.Now(), Rows: safe}
		if err != nil {
			s.Error = err.Error()
		}
		v.Streams = append(v.Streams, s)
	}
	for _, code := range marketCodes {
		rows, err := c.daily(ctx, "index_daily", code, target.AddDate(0, 0, -100).Format("20060102"), target.Format("20060102"))
		add("tushare", "index_daily", code, date, "thousand_CNY", rows, err)
		if c.Fallback != nil {
			rows, err = c.alternativeRows(ctx, code, "day")
			add("eastmoney", "day", code, date, "thousand_CNY", rows, err)
		}
		if c.SecondaryFallback != nil {
			rows, err = c.alternativeRowsFor(ctx, c.SecondaryFallback, "sina", code, "day")
			add("sina", "day", code, date, "thousand_CNY", rows, err)
			if session == "closed" {
				// Sina daily amounts are derived: retain all original minute
				// amounts even at close so the 48-bucket sum can be audited.
				rows, err = c.alternativeRowsFor(ctx, c.SecondaryFallback, "sina", code, "5")
				add("sina", "5min", code, date, "CNY", rows, err)
			}
		}
		if session != "closed" {
			rows, err = c.query(ctx, "rt_idx_min_daily", map[string]any{"ts_code": code, "freq": "5MIN"}, "", time.Minute)
			add("tushare", "rt_idx_min_daily", code, date, "CNY", rows, err)
			rows, err = c.query(ctx, "idx_mins", map[string]any{"ts_code": code, "freq": "5min", "start_date": previous + " 09:00:00", "end_date": previous + " 15:00:00"}, "", 24*time.Hour)
			add("tushare", "idx_mins", code, previous, "CNY", rows, err)
			if c.Fallback != nil {
				rows, err = c.alternativeRows(ctx, code, "5")
				add("eastmoney", "5min", code, date, "CNY", rows, err)
			}
			if c.SecondaryFallback != nil {
				rows, err = c.alternativeRowsFor(ctx, c.SecondaryFallback, "sina", code, "5")
				add("sina", "5min", code, date, "CNY", rows, err)
			}
		}
	}
	if session != "closed" {
		rows, err := c.query(ctx, "rt_idx_k", map[string]any{"ts_code": strings.Join(marketCodes, ",")}, "ts_code,trade_time,amount", 30*time.Second)
		add("tushare", "rt_idx_k", strings.Join(marketCodes, ","), date, "CNY", rows, err)
	}
	v.Checks = map[string]bool{}
	for name, s := range map[string]*Snapshot{"primary": v.Primary, "backup": v.Backup, "secondary": v.Secondary} {
		if s == nil {
			continue
		}
		v.Checks[name+"_actual"] = s.Actual != nil && *s.Actual > 0 && s.Date == date
		v.Checks[name+"_mean20"] = s.Mean20 != nil && completeHistory(s.History, target)
		if v.Window == "opening" || v.Window == "close" {
			v.Checks[name+"_forecast_guard"] = s.Forecast == nil
		} else {
			v.Checks[name+"_forecast"] = s.Forecast != nil && s.Actual != nil && *s.Forecast >= *s.Actual
		}
	}
	v.FinishedAt = time.Now()
	return v
}
