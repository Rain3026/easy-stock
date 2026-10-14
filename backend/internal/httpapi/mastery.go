package httpapi

import (
	"easy-stock/backend/internal/methodology"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

func (s *Server) masteryIndex(w http.ResponseWriter, r *http.Request) {
	if s.masteryLibrary == nil {
		writeError(w, http.StatusServiceUnavailable, "游资心法资料库不可用")
		return
	}
	ctx, cancel := contextWithTimeout(r, 45*time.Second)
	defer cancel()
	snapshot, err := s.masteryLibrary.Snapshot(ctx, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": snapshot})
}

func (s *Server) masteryTrader(w http.ResponseWriter, r *http.Request) {
	if s.masteryLibrary == nil {
		writeError(w, http.StatusServiceUnavailable, "游资心法资料库不可用")
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	documentID := strings.TrimSpace(r.URL.Query().Get("document"))
	if id == "" && documentID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	ctx, cancel := contextWithTimeout(r, 45*time.Second)
	defer cancel()
	var trader methodology.TraderDetail
	var err error
	if documentID != "" {
		trader, err = s.masteryLibrary.TraderForDocument(ctx, documentID)
	} else {
		trader, err = s.masteryLibrary.Trader(ctx, id)
	}
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "未找到该游资心法")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": trader})
}

func (s *Server) masteryRefresh(w http.ResponseWriter, r *http.Request) {
	if s.masteryLibrary == nil {
		writeError(w, http.StatusServiceUnavailable, "游资心法资料库不可用")
		return
	}
	ctx, cancel := contextWithTimeout(r, 90*time.Second)
	defer cancel()
	snapshot, err := s.masteryLibrary.Snapshot(ctx, true)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": snapshot})
}
