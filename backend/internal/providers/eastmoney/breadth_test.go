package eastmoney

import (
	"easy-stock/backend/internal/foundation"
	"fmt"
	"testing"
	"time"
)

func breadthRows() ([]map[string]any, time.Time) {
	now := time.Date(2026, 8, 11, 10, 30, 0, 0, foundation.AStockLocation)
	rows := []map[string]any{}
	for i := 0; i < 4000; i++ {
		market := 0
		code := fmt.Sprintf("%06d", i)
		if i < 1800 {
			market = 1
			code = fmt.Sprintf("%06d", 600000+i)
		}
		rows = append(rows, map[string]any{"f12": code, "f13": float64(market), "f2": 11.0, "f18": 10.0, "f6": 100.0, "f124": float64(now.Unix())})
	}
	return rows, now
}
func TestBreadthRejectsPartialDuplicateStaleAndForeignUniverse(t *testing.T) {
	for _, name := range []string{"complete", "missing-page", "duplicate", "stale", "missing-price", "foreign"} {
		t.Run(name, func(t *testing.T) {
			rows, now := breadthRows()
			switch name {
			case "missing-page":
				rows = rows[:3999]
			case "duplicate":
				rows[1] = rows[0]
			case "stale":
				rows[0]["f124"] = float64(now.Add(-24 * time.Hour).Unix())
			case "missing-price":
				rows[0]["f18"] = nil
			case "foreign":
				rows[0]["f12"] = "900001"
			}
			got, e := aggregateBreadth(rows, 4000, "2026-08-11", "intraday", now)
			if name == "complete" {
				if e != nil || got.Up != 4000 {
					t.Fatal(got, e)
				}
			} else if e == nil {
				t.Fatal("accepted invalid breadth")
			}
		})
	}
}
