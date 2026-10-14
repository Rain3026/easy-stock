package httpapi

import (
	"context"
	"easy-stock/backend/internal/marketpulse"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestOverviewCacheJoinsRefreshAndRestoresOriginalTime(t *testing.T) {
	dir := t.TempDir()
	at := time.Now().Add(-time.Hour).UTC()
	old := cachedOverview{Data: dailyOverviewData{Pulse: marketpulse.Snapshot{Date: "2026-10-12", FetchedAt: at}}, Fetched: at}
	raw, _ := json.Marshal(old)
	os.WriteFile(filepath.Join(dir, "overview-v1.json"), raw, 0600)
	var c overviewCache
	c.restore(dir)
	var calls atomic.Int32
	release := make(chan struct{})
	load := func(context.Context) dailyOverviewData {
		calls.Add(1)
		<-release
		return dailyOverviewData{Pulse: marketpulse.Snapshot{Date: "2026-10-13"}}
	}
	for i := 0; i < 20; i++ {
		v, running, when, _ := c.read(time.Now(), false, load)
		if v == nil || v.Pulse.Date != "2026-10-12" || !running || !when.Equal(at) {
			t.Fatal("lost original cache or timestamp")
		}
	}
	_, _, _, done := c.read(time.Now(), false, load)
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refresh blocked")
	}
	v, running, _, _ := c.read(time.Now(), false, load)
	if calls.Load() != 1 || running || v.Pulse.Date != "2026-10-13" {
		t.Fatal(calls.Load(), v, running)
	}
	var restored overviewCache
	restored.restore(dir)
	if restored.data == nil || restored.data.Pulse.Date != "2026-10-13" {
		t.Fatal("cache not persisted")
	}
}
