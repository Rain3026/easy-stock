package cninfo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnnouncementFallbackUsesExactIdentityAndDeduplicates(t *testing.T) {
	directory := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/new/data/szse_stock.json" {
			directory++
			w.Write([]byte(`{"stockList":[{"code":"600176","orgId":"gssh0600176"}]}`))
			return
		}
		r.ParseForm()
		if r.Form.Get("stock") != "600176,gssh0600176" || r.Form.Get("column") != "sse" {
			t.Errorf("wrong security filter %v", r.Form)
		}
		w.Write([]byte(`{"announcements":[{"announcementId":"a","secCode":"600176","secName":"样本","announcementTitle":"<em>半年度</em>报告","announcementTime":1775001600000,"adjunctUrl":"finalpage/2026-04-01/a.PDF"},{"announcementId":"a","secCode":"600176","announcementTitle":"duplicate","announcementTime":1775001600000,"adjunctUrl":"finalpage/2026-04-01/a.PDF"},{"announcementId":"b","secCode":"000001","announcementTitle":"different stock","announcementTime":1775001600000,"adjunctUrl":"finalpage/2026-04-01/b.PDF"}]}`))
	}))
	defer s.Close()
	c := NewClient()
	c.BaseURL = s.URL
	c.HTTP = s.Client()
	for range 2 {
		rows, m, e := c.MarketAnnouncements(context.Background(), "报告", "600176.SH", "", 3)
		if e != nil || len(rows) != 1 || rows[0].ID != "cninfo:a" || rows[0].Title != "半年度报告" || rows[0].Content != "" || m.Family != "cninfo" {
			t.Fatalf("%+v %+v %v", rows, m, e)
		}
	}
	if directory != 1 {
		t.Fatal("directory cache was not reused")
	}
	if _, _, e := c.MarketAnnouncements(context.Background(), "", "600176.SH", "unknown-category", 3); e == nil {
		t.Fatal("silently dropped category")
	}
}
func TestHTTPErrorDoesNotLeakBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429); w.Write([]byte("secret")) }))
	defer s.Close()
	c := NewClient()
	c.BaseURL = s.URL
	c.HTTP = s.Client()
	_, _, e := c.MarketAnnouncements(context.Background(), "", "", "", 3)
	if e == nil || !strings.Contains(e.Error(), "429") || strings.Contains(e.Error(), "secret") {
		t.Fatal(e)
	}
}
