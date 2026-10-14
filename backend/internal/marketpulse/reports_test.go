package marketpulse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResearchReportsKeepUpstreamAndFilterIdentity(t *testing.T) {
	day := time.Now().AddDate(0, 0, -1).Format("20060102")
	fields := []string{"trade_date", "title", "report_type", "author", "name", "ts_code", "inst_csname", "ind_name", "url"}
	row := []any{day, "报告样本", "个股研报", "甲", "中国巨石", "600176.SH", "测试证券", "玻纤", "https://pdf.dfcfw.com/pdf/report.pdf?download=1"}
	wrong := append([]any{}, row...)
	wrong[5] = "600000.SH"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			API    string         `json:"api_name"`
			Params map[string]any `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&v)
		if v.API != "research_report" || v.Params["ts_code"] != "600176.SH" {
			t.Error("wrong request")
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"fields": fields, "items": [][]any{row, row, wrong}}})
	}))
	defer srv.Close()
	c := New(func() string { return "test-only" }, "")
	c.URL = srv.URL
	items, meta, err := c.MarketReports(context.Background(), "stock", "", "600176.SH", "", 3)
	if err != nil || len(items) != 1 {
		t.Fatal(err, items)
	}
	if !strings.Contains(items[0].URL, "pdf.dfcfw.com") || !strings.Contains(meta.FallbackReason, "不计作独立") || items[0].Rating != "" {
		t.Fatal("upstream or missing-field boundary lost", items, meta)
	}
}
