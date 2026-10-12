package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestThemeScreenFullPoolIsOptInAndKeepsSnapshot(t *testing.T) {
	server := NewServer(Config{SectorMap: fakePagedSectorMapProvider{}})
	defer server.Close()
	base := "/api/v1/themes/screen?theme=medicine"
	bounded := requestThemeScreen(t, server, base)
	full := requestThemeScreen(t, server, base+"&full_pool=1&page=2&page_size=10")
	if len(uniqueThemeScreenStocks(bounded.Map)) != 20 || !bounded.Pagination.HasMore {
		t.Fatalf("default server pagination changed: %+v", bounded.Pagination)
	}
	if len(uniqueThemeScreenStocks(full.Map)) != 25 || len(full.Order) != 25 || full.Pagination.Total != 25 || full.Pagination.Page != 1 || full.Pagination.TotalPages != 1 || full.Pagination.HasMore {
		t.Fatalf("full pool truncated or duplicated: %+v", full)
	}
	if !full.Complete || full.SnapshotID != bounded.SnapshotID || full.Map.Groups[0].Nodes[0].CandidateCount != 25 {
		t.Fatalf("full pool lost snapshot or membership metadata: %+v", full)
	}
	filtered := requestThemeScreen(t, server, base+"&full_pool=1&node=branch")
	if filtered.Pagination.Total != 5 || len(filtered.Order) != 5 {
		t.Fatalf("full pool changed node filtering: %+v", filtered)
	}
	empty := requestThemeScreen(t, server, base+"&full_pool=1&q=no-such-stock")
	if empty.Pagination.Total != 0 || empty.Pagination.TotalPages != 0 || len(empty.Order) != 0 {
		t.Fatalf("bad empty pool: %+v", empty)
	}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest(http.MethodGet, base+"&full_pool=maybe", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid full_pool status = %d", w.Code)
	}
}
