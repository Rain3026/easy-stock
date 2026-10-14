package httpapi

import (
	dr "easy-stock/backend/internal/dailyresearch"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type researchNote struct {
	ID         string    `json:"id"`
	EntityID   string    `json:"entity_id"`
	ParentID   string    `json:"parent_id,omitempty"`
	Title      string    `json:"title"`
	Text       string    `json:"text"`
	SourceID   string    `json:"source_id,omitempty"`
	SourceURL  string    `json:"source_url,omitempty"`
	SourceHash string    `json:"source_hash,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Server) researchNoteRoutes() {
	s.mux.HandleFunc("GET /api/v1/research/notes", func(w http.ResponseWriter, r *http.Request) {
		offset, err := nonNegativeQueryInt(r, "offset", 0)
		if err != nil {
			writeError(w, 400, "无效分页")
			return
		}
		rows, e := s.dailyResearch.store.Records(r.Context(), "note", r.URL.Query().Get("entity"), 100, offset)
		if e != nil {
			writeError(w, 500, "笔记读取失败")
			return
		}
		writeJSON(w, 200, map[string]any{"data": rows})
	})
	s.mux.HandleFunc("POST /api/v1/research/notes", func(w http.ResponseWriter, r *http.Request) {
		var v researchNote
		if !skillBody(w, r, &v) {
			return
		}
		if !(strings.HasPrefix(v.EntityID, "stock:") || strings.HasPrefix(v.EntityID, "knowledge:") || strings.HasPrefix(v.EntityID, "portfolio:")) || len(v.EntityID) > 180 || len(v.Text) > 50000 || strings.TrimSpace(v.Text) == "" || len(v.Title) > 200 {
			writeError(w, 400, "笔记对象或正文无效")
			return
		}
		// A parent only links versions; concurrent edits create branches rather
		// than overwrite each other's text. Both remain visible in history.
		if v.ParentID != "" {
			raw, e := s.dailyResearch.store.Record(r.Context(), "note", v.ParentID)
			var old researchNote
			valid := e == nil && json.Unmarshal(raw, &old) == nil && old.EntityID == v.EntityID
			if !valid {
				writeError(w, 400, "原笔记版本不属于这个对象")
				return
			}
		}
		v.ID = dr.NewID()
		v.CreatedAt = time.Now().UTC()
		if e := s.dailyResearch.store.Append(r.Context(), v.ID, "note", v.EntityID, v); e != nil {
			writeError(w, 500, "笔记保存失败")
			return
		}
		writeJSON(w, 201, map[string]any{"data": v})
	})
}
