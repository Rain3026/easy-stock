package tencent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStockQuotesUnitsAndSourceTime(t *testing.T) {
	f := make([]string, 54)
	for i := range f {
		f[i] = "0"
	}
	for k, v := range map[int]string{1: "Example", 2: "600176", 3: "38.46", 4: "38.30", 5: "36.88", 6: "1660938", 30: "20261013161442", 31: "0.16", 32: "0.42", 33: "38.95", 34: "36.05", 37: "624262"} {
		f[k] = v
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "v_sh600176=\"%s\";", strings.Join(f, "~"))
	}))
	defer s.Close()
	c := NewClient(WithQuoteBaseURL(s.URL))
	v, e := c.Realtime(context.Background(), []string{"600176.SH"})
	if e != nil || len(v) != 1 || v[0].Volume != 166093800 || v[0].Amount != 6242620000 || v[0].TradeTime.IsZero() {
		t.Fatal(v, e)
	}
}
func TestStockKLineUsesExistingAdjustedAdapter(t *testing.T) {
	for _, field := range []string{"qfqday", "day"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"code":0,"data":{"sh600176":{"%s":[["2026-10-13","36.88","38.46","38.95","36.05","1660938"]]}}}`, field)
		}))
		c := NewClient(WithKLineBaseURL(s.URL))
		v, e := c.KLine(context.Background(), "600176.SH", "day", 5)
		s.Close()
		if field == "qfqday" && (e != nil || len(v) != 1 || v[0].Meta.Adjustment != "qfq") {
			t.Fatal(v, e)
		}
		if field == "day" && e == nil {
			t.Fatal("unadjusted stock accepted")
		}
	}
}
