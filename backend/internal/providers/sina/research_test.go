package sina

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func reportRow(id, title, date, category string) string {
	return fmt.Sprintf(`<tr><td>1</td><td><a title="%s" href="//stock.finance.sina.com.cn/stock/go.php/vReport_Show/kind/search/rptid/%s/index.phtml">short…</a></td><td>%s</td><td>%s</td><td><a><span>测试证券</span></a></td><td>研究员甲</td></tr>`, title, id, category, date)
}

func TestReportsPreserveIdentityFilterDatesAndDeduplicate(t *testing.T) {
	now := time.Now()
	valid := now.AddDate(0, 0, -1).Format("2006-01-02")
	rows := reportRow("100", "中国巨石(600176)：完整标题", valid, "公司") + reportRow("100", "中国巨石(600176)：完整标题", valid, "公司") + reportRow("101", "其他公司(600000)：同一题材", valid, "公司") + reportRow("102", "中国巨石(600176)：未来报告", now.AddDate(0, 0, 1).Format("2006-01-02"), "公司") + reportRow("103", "中国巨石(600176)：过期报告", now.AddDate(0, 0, -90).Format("2006-01-02"), "公司")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("symbol") != "sh600176" || r.URL.Query().Get("t1") != "2" {
			t.Errorf("wrong symbol query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `<table class="tb_01">`+rows+`</table>`)
	}))
	defer srv.Close()
	client := NewClient(WithReportBaseURL(srv.URL))
	items, meta, err := client.MarketReports(context.Background(), "stock", "完整", "600176.SH", "", 10)
	if err != nil || len(items) != 1 {
		t.Fatal(err, len(items))
	}
	r := items[0]
	if r.Title != "中国巨石(600176)：完整标题" || r.Organization != "测试证券" || r.Researchers != "研究员甲" || r.Symbol != "600176.SH" || r.ID != "sina:100" || r.PublishedAt.IsZero() {
		t.Fatalf("lost identity: %+v", r)
	}
	if r.Rating != "" || r.EPS != 0 || !strings.Contains(meta.FallbackReason, "不提供") {
		t.Fatal("invented unavailable fields")
	}
	if _, _, err = client.MarketReports(context.Background(), "industry", "", "", "BK123", 10); err == nil {
		t.Fatal("unmapped classification accepted")
	}
}

func TestReportsRejectMalformedPageAndForeignLinks(t *testing.T) {
	if _, err := parseReportPage(`<html>暂时无法访问</html>`, "stock"); err == nil {
		t.Fatal("error page accepted as empty catalogue")
	}
	row := reportRow("100", "中国巨石(600176)：样本", "2026-10-13", "公司")
	row = strings.ReplaceAll(row, "//stock.finance.sina.com.cn/", "//untrusted.example/")
	items, err := parseReportPage(`<table class="tb_01">`+row+`</table>`, "stock")
	if err != nil || len(items) != 0 {
		t.Fatal(err, items)
	}
}
