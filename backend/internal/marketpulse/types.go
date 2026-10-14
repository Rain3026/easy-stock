// Package marketpulse keeps live market observations separate from frozen research.
package marketpulse

import (
	"easy-stock/backend/internal/foundation"
	"time"
)

type Day struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
}
type Breadth = foundation.MarketBreadth

type Level struct {
	Height int `json:"height"`
	Count  int `json:"count"`
}
type Emotion struct {
	Partial    bool    `json:"partial"`
	Date       string  `json:"date"`
	Up         int     `json:"up"`
	Down       int     `json:"down"`
	Boards     int     `json:"boards"`
	Height     int     `json:"height"`
	BrokenRate float64 `json:"broken_rate"`
	Levels     []Level `json:"levels"`
	Source     string  `json:"source"`
}
type Snapshot struct {
	FallbackReason  string    `json:"fallback_reason,omitempty"`
	Version         string    `json:"version"`
	ObservationLine float64   `json:"observation_line"`
	Date            string    `json:"date"`
	Session         string    `json:"session"`
	At              string    `json:"at"`
	FetchedAt       time.Time `json:"fetched_at"`
	Source          string    `json:"source"`
	History         []Day     `json:"history"`
	Actual          *float64  `json:"actual"`
	Forecast        *float64  `json:"forecast"`
	Mean20          *float64  `json:"mean20"`
	Previous        *float64  `json:"previous"`
	SameTimeChange  *float64  `json:"same_time_change"`
	ForecastMethod  string    `json:"forecast_method"`
	Breadth         *Breadth  `json:"breadth"`
	Emotion         *Emotion  `json:"emotion"`
	Gaps            []string  `json:"gaps"`
	Stale           bool      `json:"stale"`
}

func ptr(v float64) *float64 { return &v }
