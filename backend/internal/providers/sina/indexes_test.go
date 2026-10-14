package sina

import (
	"strings"
	"testing"
	"time"

	"easy-stock/backend/internal/foundation"
)

func ndxFixture(name, updated, quoted string) string {
	f := make([]string, 27)
	for i := range f {
		f[i] = "0"
	}
	f[0] = name
	f[1] = "30569.4809"
	f[2] = "-0.31"
	f[3] = updated
	f[4] = "-95.2158"
	f[25] = quoted
	return `var hq_str_gb_$ndx="` + strings.Join(f, ",") + `";`
}
func TestNasdaq100IdentityAndExchangeTime(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T07:40:00+08:00")
	row, err := parseNasdaq100(ndxFixture("纳斯达克100", "2026-10-14 05:30:00", "Oct 13 05:16PM EDT"), foundation.SourceMeta{}, now)
	if err != nil || row.ID != "nasdaq100" || row.Price != 30569.4809 || row.TradeTime.Format(time.RFC3339) != "2026-10-13T17:16:00-04:00" || row.Status != "closed" {
		t.Fatal(row, err)
	}
	for _, body := range []string{ndxFixture("纳斯达克综合", "2026-10-14 05:30:00", "Oct 13 05:16PM EDT"), ndxFixture("纳斯达克100", "2026-10-14 05:30:00", "Oct 1 04:00PM EDT"), `var hq_str_gb_$ndx="";`, strings.Replace(ndxFixture("纳斯达克100", "2026-10-14 05:30:00", "Oct 13 05:16PM EDT"), "30569.4809", "NaN", 1)} {
		if _, err := parseNasdaq100(body, foundation.SourceMeta{}, now); err == nil {
			t.Fatal("bad quote accepted", body)
		}
	}
	open, _ := time.Parse(time.RFC3339, "2026-10-13T23:30:00+08:00")
	if _, err := parseNasdaq100(ndxFixture("纳斯达克100", "2026-10-13 23:30:00", "Oct 13 10:00AM EDT"), foundation.SourceMeta{}, open); err == nil {
		t.Fatal("stale intraday quote accepted")
	}
	winter, _ := time.Parse(time.RFC3339, "2026-12-02T06:40:00+08:00")
	r, err := parseNasdaq100(ndxFixture("纳斯达克100", "2026-12-02 05:30:00", "Dec 1 04:00PM EST"), foundation.SourceMeta{}, winter)
	if err != nil || r.TradeTime.Format(time.RFC3339) != "2026-12-01T16:00:00-05:00" {
		t.Fatal(r, err)
	}
}
