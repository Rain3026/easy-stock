package foundation

import "time"

// IndexSessionStatus uses the exchange clock, not the computer's local date.
// A recently fetched value does not prove the exchange is still trading.
func IndexSessionStatus(market string, quote, now time.Time) string {
	zones := map[string]string{"CN": "Asia/Shanghai", "HK": "Asia/Hong_Kong", "US": "America/New_York", "UK": "Europe/London", "JP": "Asia/Tokyo", "KR": "Asia/Seoul", "TW": "Asia/Taipei", "DE": "Europe/Berlin", "FR": "Europe/Paris"}
	zone, ok := zones[market]
	if !ok || quote.IsZero() {
		return "unknown"
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return "unknown"
	}
	n := now.In(loc)
	q := quote.In(loc)
	m := n.Hour()*60 + n.Minute()
	open := false
	switch market {
	case "CN":
		open = IsAStockTradingDay(n) && ((m >= 570 && m < 690) || (m >= 780 && m < 900))
	case "HK":
		open = (m >= 570 && m < 720) || (m >= 780 && m < 960)
	case "US":
		open = m >= 570 && m < 960
	case "UK":
		open = m >= 480 && m < 990
	case "JP":
		open = (m >= 540 && m < 690) || (m >= 750 && m < 930)
	case "KR":
		open = m >= 540 && m < 930
	case "TW":
		open = m >= 540 && m < 810
	case "DE", "FR":
		open = m >= 540 && m < 1050
	}
	if n.Weekday() == time.Saturday || n.Weekday() == time.Sunday || !open {
		return "closed"
	}
	if q.Format("2006-01-02") != n.Format("2006-01-02") || now.Sub(quote) > 20*time.Minute || quote.After(now.Add(time.Minute)) {
		return "unknown"
	}
	return "open"
}
