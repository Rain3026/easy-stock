package marketpulse

import (
	"easy-stock/backend/internal/foundation"
	"time"
)

const RuleVersion = "market-reference-v2"

type Judgement struct {
	Version         string    `json:"version"`
	Label           string    `json:"label"`
	Range           *[2]int   `json:"range"`
	Tone            string    `json:"tone"`
	Reason          string    `json:"reason"`
	ObservationLine float64   `json:"observation_line"`
	CalculatedAt    time.Time `json:"calculated_at"`
}

// Judge computes a versioned reference from facts only. It never invokes an AI
// runtime; the same output is frozen for research and sent to the overview.
func Judge(p Snapshot, indexes []foundation.MarketIndexSnapshot, themes []foundation.ThemeOverview, themeMeta foundation.SourceMeta, now time.Time) Judgement {
	out := Judgement{Version: RuleVersion, Label: "待判断", Tone: "neutral", Reason: "等待同日量能、市场广度与指数数据", ObservationLine: p.ObservationLine, CalculatedAt: now}
	if p.Session == "intraday" && now.Sub(p.FetchedAt) > 7*time.Minute {
		out.Reason = "盘中数据已过期，请刷新"
		return out
	}
	if p.Session != "closed" {
		at := parseTime(p.At)
		if at.IsZero() || at.Hour()*60+at.Minute() < 630 {
			out.Label = "早盘观察"
			out.Reason = "早盘预估波动较大，先观察量能与广度"
			return out
		}
	}
	value := p.Forecast
	if p.Session == "closed" {
		value = p.Actual
	}
	if value == nil || p.Mean20 == nil || p.Breadth == nil || p.Breadth.Date != p.Date || p.Stale || p.ObservationLine <= 0 {
		return out
	}
	count := 0
	trend := 0.0
	for _, i := range indexes {
		if i.Market != "CN" || i.Meta.Stale || i.TradeTime.In(foundation.AStockLocation).Format("2006-01-02") != p.Date {
			continue
		}
		switch i.ID {
		case "sse", "szse", "chinext", "csi300", "csi1000":
			count++
			trend += i.ChangePercent
		}
	}
	if count < 3 {
		return out
	}
	trend /= float64(count)
	b := p.Breadth
	total := b.Up + b.Down + b.Flat
	if total <= 0 {
		return out
	}
	up := float64(b.Up) / float64(total)
	if up < .4 || trend < -0.8 {
		out.Label = "偏空"
		out.Range = &[2]int{0, 20}
		out.Tone = "bear"
		out.Reason = "市场广度或指数趋势偏弱，控制参与规模"
		return out
	}
	if *value >= p.ObservationLine && *value >= *p.Mean20 && up >= .6 && trend > 0 {
		mainline := false
		for _, t := range themes {
			if t.TradeDate == p.Date && !t.CarryForward && !themeMeta.Stale && t.ActiveDays >= 2 && t.DailyStrengthScore >= 70 {
				mainline = true
			}
		}
		healthy := mainline && p.Emotion != nil && !p.Emotion.Partial && p.Emotion.Date == p.Date && p.Emotion.Height >= 3 && p.Emotion.BrokenRate < 30
		out.Label = "中性偏多"
		out.Tone = "bull"
		out.Range = &[2]int{30, 40}
		out.Reason = "量能与广度改善，主线及情绪仍需确认"
		if healthy {
			out.Range = &[2]int{30, 50}
			out.Reason = "量能、广度与短线情绪共同支持"
		}
		return out
	}
	out.Label = "中性观望"
	out.Range = &[2]int{10, 30}
	out.Reason = "量价信号尚未形成一致方向"
	if *value < p.ObservationLine {
		out.Reason = "量能未达观察线，优先观察持续性"
	}
	return out
}
