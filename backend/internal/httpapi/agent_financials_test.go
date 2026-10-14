package httpapi

import (
	"easy-stock/backend/internal/foundation"
	"testing"
	"time"
)

func TestFinancialToolPeriodIdentityAndMissingValues(t *testing.T) {
	cutoff := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	row := foundation.StockFundamentals{Symbol: "600176.SH", ReportDate: "2026-06-30 00:00:00", PublishedAt: cutoff.Add(-time.Hour), Revenue: 0, NetProfit: 99, Meta: foundation.SourceMeta{AvailableFields: []string{"revenue"}}}
	future := row
	future.ReportDate = "2026-09-30"
	future.PublishedAt = cutoff.Add(time.Hour)
	wrong := row
	wrong.Symbol = "600000.SH"
	rows, _, _ := financialToolRows([]foundation.StockFundamentals{row, row, future, wrong}, "600176.SH", cutoff)
	if len(rows) != 1 {
		t.Fatal("future, duplicate or wrong security leaked", rows)
	}
	f := rows[0]["fields"].(map[string]any)
	if _, ok := f["net_profit"]; ok {
		t.Fatal("missing field invented")
	}
	if v, ok := f["revenue"]; !ok || v != float64(0) {
		t.Fatal("actual zero lost")
	}
	if rows[0]["kind"] != "actual" || rows[0]["report_period"] != "2026-06-30" {
		t.Fatal(rows)
	}
}

func TestSupplementalFinancialToolRejectsFutureAndDuplicatePeriods(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	row := foundation.StockFinancialEvidence{Symbol: "600176.SH", ReportDate: "2026-06-30", Fields: map[string]float64{"revenue": 0}}
	future := row
	future.ReportDate = "2027-03-31"
	wrong := row
	wrong.Symbol = "600000.SH"
	rows, _, gaps := supplementalFinancialRows([]foundation.StockFinancialEvidence{row, row, future, wrong}, row.Symbol, now)
	if len(rows) != 1 || len(gaps) != 1 || rows[0]["amount_unit"] != "CNY" {
		t.Fatal(rows, gaps)
	}
	if _, ok := rows[0]["published_at"]; ok {
		t.Fatal("unknown publication time invented")
	}
}
