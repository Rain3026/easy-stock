package sina

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"easy-stock/backend/internal/foundation"
)

func breadthFixtureMembers() ([]foundation.BreadthMember, time.Time) {
	now := time.Date(2026, 8, 11, 10, 30, 0, 0, foundation.AStockLocation)
	out := []foundation.BreadthMember{}
	for i := 0; i < 4000; i++ {
		s := fmt.Sprintf("%06d.SZ", i-1800)
		if i < 1800 {
			s = fmt.Sprintf("%06d.SH", 600000+i)
		}
		out = append(out, foundation.BreadthMember{Symbol: s, At: "2026-08-11 10:30:00", Amount: 100, Change: 1, ChangeUnit: "CNY"})
	}
	return out, now
}

func TestSinaBreadthRejectsUnsafeUniverseAndTimes(t *testing.T) {
	for _, kind := range []string{"valid", "partial", "duplicate", "foreign", "stale", "future", "skew", "negative", "nan"} {
		t.Run(kind, func(t *testing.T) {
			rows, now := breadthFixtureMembers()
			switch kind {
			case "partial":
				rows = rows[:2000]
			case "duplicate":
				rows[1] = rows[0]
			case "foreign":
				rows[0].Symbol = "920000.BJ"
			case "stale":
				rows[0].At = "2026-08-10 10:30:00"
			case "future":
				rows[0].At = "2026-08-11 10:35:00"
			case "skew":
				rows[0].At = "2026-08-11 10:27:00"
			case "negative":
				rows[0].Amount = -1
			case "nan":
				rows[0].Change, _ = strconv.ParseFloat("NaN", 64)
			}
			b, e := aggregateSinaBreadth(rows, "2026-08-11", "intraday", now)
			if kind == "valid" {
				if e != nil || b.Up != 4000 || b.Coverage != 4000 || b.ActiveHash == "" {
					t.Fatal(b, e)
				}
			} else if e == nil {
				t.Fatal("unsafe breadth admitted", kind)
			}
		})
	}
	rows, now := breadthFixtureMembers()
	rows[0].Amount = 0
	rows[0].At = ""
	b, e := aggregateSinaBreadth(rows, "2026-08-11", "intraday", now)
	if e != nil || b.Excluded != 1 || b.Up != 3999 {
		t.Fatal("suspension denominator wrong", b, e)
	}
	for i := range rows {
		rows[i].At = "2026-08-11 11:30:00"
	}
	if _, e = aggregateSinaBreadth(rows, "2026-08-11", "closed", now.Add(5*time.Hour)); e == nil {
		t.Fatal("lunch quote accepted as close")
	}
}

func TestSinaBreadthQuoteParserDoesNotInventMissingValues(t *testing.T) {
	fields := make([]string, 33)
	fields[0] = "fixture"
	fields[2] = "10"
	fields[3] = "11"
	fields[9] = "100"
	fields[30] = "2026-08-11"
	fields[31] = "10:30:00"
	makeBody := func() string { return `var hq_str_sh600000="` + strings.Join(fields, ",") + `";` }
	for _, value := range []string{"", "-", "NaN", "-1"} {
		fields[9] = value
		if _, e := parseBreadthQuotes(makeBody(), []string{"600000.SH"}); e == nil {
			t.Fatal("bad amount admitted", value)
		}
	}
	fields[9] = "100"
	body := makeBody()
	for _, bad := range []string{"", body + body, `var hq_str_sh600000="";`} {
		if _, e := parseBreadthQuotes(bad, []string{"600000.SH"}); e == nil {
			t.Fatal("missing/duplicate quote accepted")
		}
	}
	if _, e := parseBreadthQuotes(body, []string{"600000.SH", "600001.SH"}); e == nil {
		t.Fatal("partial quote batch accepted")
	}
}

func TestSinaBreadthPagedUnionIncludesCDRAndExcludesBeijing(t *testing.T) {
	for _, fault := range []string{"", "short-page", "count-change", "duplicate"} {
		t.Run(fault, func(t *testing.T) {
			members, now := breadthFixtureMembers()
			nodes := map[string][]string{"hs_a": {"bj920000", "bj920001"}, "kcb": {"sh600000", "sh689009"}}
			for _, m := range members {
				nodes["hs_a"] = append(nodes["hs_a"], strings.ToLower(m.Symbol[7:])+m.Symbol[:6])
			}
			var counts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				q := r.URL.Query()
				node := q.Get("node")
				if strings.HasSuffix(r.URL.Path, "getHQNodeStockCount") {
					n := len(nodes[node])
					if node == "hs_a" && counts.Add(1) > 1 && fault == "count-change" {
						n++
					}
					json.NewEncoder(w).Encode(strconv.Itoa(n))
					return
				}
				if strings.HasSuffix(r.URL.Path, "getHQNodeData") {
					page, _ := strconv.Atoi(q.Get("page"))
					start := (page - 1) * 80
					end := min(start+80, len(nodes[node]))
					data := []map[string]string{}
					for _, s := range nodes[node][start:end] {
						data = append(data, map[string]string{"symbol": s})
					}
					if node == "hs_a" && page == 2 {
						if fault == "short-page" {
							data = data[:len(data)-1]
						}
						if fault == "duplicate" {
							data[1] = data[0]
						}
					}
					json.NewEncoder(w).Encode(data)
					return
				}
				for _, s := range strings.Split(q.Get("list"), ",") {
					fields := make([]string, 33)
					fields[0] = "fixture"
					fields[2] = "10"
					fields[3] = "11"
					fields[9] = "100"
					fields[30] = "2026-08-11"
					fields[31] = "10:30:00"
					fmt.Fprintf(w, "var hq_str_%s=\"%s\";", s, strings.Join(fields, ","))
				}
			}))
			defer srv.Close()
			c := NewClient(WithBreadthBaseURL(srv.URL+"/Market_Center."), WithBaseURL(srv.URL+"/quotes"), WithHTTPClient(srv.Client()))
			b, e := c.MarketBreadth(context.Background(), "2026-08-11", "intraday", now)
			if fault != "" {
				if e == nil {
					t.Fatal("broken pagination admitted", fault)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if b.Coverage != 4001 || b.Up != 4001 || b.OtherMarketsExcluded != 2 {
				t.Fatalf("bad union %+v", b)
			}
			found := false
			for _, m := range b.Members {
				if m.Symbol == "689009.SH" {
					found = true
				}
			}
			if !found {
				t.Fatal("STAR CDR silently omitted")
			}
		})
	}
}
