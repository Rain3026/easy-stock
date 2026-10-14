package foundation

import (
	"testing"
	"time"
)

func TestOverseasIndexStatusAcrossMidnightAndDST(t *testing.T) {
	at, _ := time.Parse(time.RFC3339, "2026-10-13T16:16:00Z")
	if IndexSessionStatus("US", at.Add(5*time.Second), at) != "open" || IndexSessionStatus("CN", at, at) != "closed" {
		t.Fatal("wrong exchange clock")
	}
	close, _ := time.Parse(time.RFC3339, "2026-10-13T20:10:00Z")
	if IndexSessionStatus("US", close, close) != "closed" {
		t.Fatal("fresh after-hours value marked open")
	}
	winter, _ := time.Parse(time.RFC3339, "2026-01-13T14:15:00Z")
	if IndexSessionStatus("US", winter, winter) != "closed" || IndexSessionStatus("US", winter.Add(time.Hour), winter.Add(time.Hour)) != "open" {
		t.Fatal("DST ignored")
	}
	if IndexSessionStatus("US", at.Add(-time.Hour), at) != "unknown" {
		t.Fatal("stale quote treated as live")
	}
}
