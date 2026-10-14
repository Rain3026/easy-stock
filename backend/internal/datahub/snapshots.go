package datahub

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// FactSnapshot is shared by market, theme, stock and portfolio research.
// Payload is evidence, not instructions. Backfills append a child instead of rewriting history.
type FactSnapshot struct {
	ID            string           `json:"id"`
	EntityID      string           `json:"entity_id"`
	Kind          string           `json:"kind"`
	OriginID      string           `json:"origin_id,omitempty"`
	ParentID      string           `json:"parent_id,omitempty"`
	MethodVersion string           `json:"method_version"`
	CapturedAt    time.Time        `json:"captured_at"`
	CutoffAt      time.Time        `json:"cutoff_at"`
	DataAt        *time.Time       `json:"data_at,omitempty"`
	Sources       []EvidenceSource `json:"sources"`
	Gaps          []string         `json:"gaps"`
	Payload       json.RawMessage  `json:"payload"`
}
type EvidenceSource struct {
	ID          string     `json:"id"`
	Family      string     `json:"family"`
	Channel     string     `json:"channel,omitempty"`
	URL         string     `json:"url,omitempty"`
	DataAt      *time.Time `json:"data_at,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CapturedAt  time.Time  `json:"captured_at"`
	Status      string     `json:"status"`
}
type FactStore struct{ db *sql.DB }

func NewFactStore(db *sql.DB) (*FactStore, error) {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS fact_snapshots(id TEXT PRIMARY KEY,entity_id TEXT NOT NULL,kind TEXT NOT NULL,origin_id TEXT NOT NULL,parent_id TEXT NOT NULL,captured_at TEXT NOT NULL,payload BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS fact_snapshots_entity ON fact_snapshots(entity_id,captured_at);
 CREATE TRIGGER IF NOT EXISTS fact_snapshots_no_update BEFORE UPDATE ON fact_snapshots BEGIN SELECT RAISE(ABORT,'fact snapshots are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS fact_snapshots_no_delete BEFORE DELETE ON fact_snapshots BEGIN SELECT RAISE(ABORT,'fact snapshots are immutable'); END;`)
	if e != nil {
		return nil, e
	}
	return &FactStore{db}, nil
}

var entityPattern = regexp.MustCompile(`^(market|stock|theme|portfolio|research):[a-zA-Z0-9._:-]{1,180}$`)

func EntityID(kind, key string) (string, error) {
	v := kind + ":" + key
	if !entityPattern.MatchString(v) {
		return "", fmt.Errorf("invalid research entity")
	}
	return v, nil
}
func (s *FactStore) Freeze(ctx context.Context, v FactSnapshot) (FactSnapshot, error) {
	if !entityPattern.MatchString(v.EntityID) || v.Kind == "" || v.MethodVersion == "" || v.CutoffAt.IsZero() || !json.Valid(v.Payload) || len(v.Payload) > 8<<20 {
		return v, fmt.Errorf("invalid fact snapshot contract")
	}
	if v.CapturedAt.IsZero() {
		v.CapturedAt = time.Now().UTC()
	}
	v.CapturedAt = v.CapturedAt.UTC()
	v.CutoffAt = v.CutoffAt.UTC()
	if v.CutoffAt.After(v.CapturedAt.Add(time.Minute)) {
		return v, fmt.Errorf("snapshot cutoff cannot be in the future")
	}
	if v.Sources == nil {
		v.Sources = []EvidenceSource{}
	}
	if v.Gaps == nil {
		v.Gaps = []string{}
	}
	v.ID = ""
	b, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	sum := sha256.Sum256(b)
	v.ID = "fact-" + hex.EncodeToString(sum[:])
	if v.ParentID != "" {
		parent, e := s.Get(ctx, v.ParentID)
		if e != nil {
			return v, fmt.Errorf("parent snapshot not found")
		}
		if parent.EntityID != v.EntityID || parent.CapturedAt.After(v.CapturedAt) {
			return v, fmt.Errorf("parent must be an earlier snapshot of the same entity")
		}
	}
	b, e = json.Marshal(v)
	if e != nil {
		return v, e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO fact_snapshots VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, v.ID, v.EntityID, v.Kind, v.OriginID, v.ParentID, v.CapturedAt.Format(time.RFC3339Nano), b)
	return v, e
}
func (s *FactStore) Get(ctx context.Context, id string) (FactSnapshot, error) {
	var v FactSnapshot
	var b []byte
	e := s.db.QueryRowContext(ctx, `SELECT payload FROM fact_snapshots WHERE id=?`, id).Scan(&b)
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *FactStore) List(ctx context.Context, entity string, limit int) ([]FactSnapshot, error) {
	if entity != "" && !entityPattern.MatchString(entity) {
		return nil, errors.New("invalid entity filter")
	}
	limit = max(1, min(limit, 100))
	rows, e := s.db.QueryContext(ctx, `SELECT payload FROM fact_snapshots WHERE (?='' OR entity_id=?) ORDER BY captured_at DESC,id LIMIT ?`, entity, entity, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []FactSnapshot{}
	for rows.Next() {
		var b []byte
		var v FactSnapshot
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		v.Payload = nil
		out = append(out, v)
	}
	return out, rows.Err()
}
