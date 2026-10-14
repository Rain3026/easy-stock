package dailyresearch

import (
	"context"
	"crypto/rand"
	"database/sql"
	"easy-stock/backend/internal/datahub"
	"easy-stock/backend/internal/skillregistry"
	"encoding/hex"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	db     *sql.DB
	facts  *datahub.FactStore
	skills *skillregistry.Store
}

func (s *Store) Facts() *datahub.FactStore { return s.facts }

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func Open(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	} else if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS research_records(id TEXT PRIMARY KEY,kind TEXT NOT NULL,snapshot_id TEXT NOT NULL,created_at TEXT NOT NULL,payload BLOB NOT NULL); CREATE TABLE IF NOT EXISTS research_jobs(id TEXT PRIMARY KEY,status TEXT NOT NULL,payload BLOB NOT NULL); CREATE TABLE IF NOT EXISTS research_overview_settings(id INTEGER PRIMARY KEY CHECK(id=1), observation_line REAL NOT NULL); CREATE TABLE IF NOT EXISTS research_settings(id INTEGER PRIMARY KEY CHECK(id=1),payload BLOB NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	facts, e := datahub.NewFactStore(db)
	if e != nil {
		db.Close()
		return nil, e
	}
	skills, e := skillregistry.Open(db)
	if e != nil {
		db.Close()
		return nil, e
	}
	s := &Store{db: db, facts: facts, skills: skills}
	if e = s.initPortfolio(); e != nil {
		db.Close()
		return nil, e
	}
	jobs, err := s.Jobs(context.Background())
	if err != nil {
		db.Close()
		return nil, err
	}
	for _, j := range jobs {
		if j.Status == "running" {
			j.Status = "interrupted"
			j.Error = "应用已重启，请重新发起；已完成记录保留"
			if err = s.SaveJob(context.Background(), j); err != nil {
				db.Close()
				return nil, err
			}
		}
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) SaveJob(ctx context.Context, j Job) error {
	j.UpdatedAt = time.Now().UTC()
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO research_jobs VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,payload=excluded.payload`, j.ID, j.Status, b)
	return e
}
func (s *Store) Job(ctx context.Context, id string) (Job, error) {
	var j Job
	var b []byte
	e := s.db.QueryRowContext(ctx, `SELECT payload FROM research_jobs WHERE id=?`, id).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &j)
	}
	return j, e
}
func (s *Store) Jobs(ctx context.Context) ([]Job, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT payload FROM research_jobs ORDER BY rowid DESC LIMIT 100`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var b []byte
		var j Job
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &j); e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// JobsForSnapshot resolves old pinned dashboard bindings beyond the recent queue.
func (s *Store) JobsForSnapshot(ctx context.Context, snapshotID string) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM research_jobs WHERE (json_extract(CAST(payload AS TEXT),'$.snapshot_id')=? OR json_extract(CAST(payload AS TEXT),'$.debate.snapshot_id')=?) AND (json_extract(CAST(payload AS TEXT),'$.kind')='debate' OR json_type(CAST(payload AS TEXT),'$.debate')='object') ORDER BY rowid DESC LIMIT 1`, snapshotID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var raw []byte
		var job Job
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &job); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

// Append never overwrites an earlier judgement, even for the same business date.
func (s *Store) Append(ctx context.Context, id, kind, parent string, value any) error {
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO research_records VALUES(?,?,?,?,?)`, id, kind, parent, time.Now().UTC().Format(time.RFC3339Nano), b)
	return e
}
func (s *Store) Snapshot(ctx context.Context, id string) (Snapshot, error) {
	var v Snapshot
	var b []byte
	e := s.db.QueryRowContext(ctx, `SELECT payload FROM research_records WHERE id=? AND kind='snapshot'`, id).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}

// Record reads a historical version directly, independent of list pagination.
func (s *Store) Record(ctx context.Context, kind, id string) (json.RawMessage, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM research_records WHERE kind=? AND id=?`, kind, id).Scan(&raw)
	return json.RawMessage(raw), err
}
func (s *Store) Records(ctx context.Context, kind, parent string, paging ...int) ([]json.RawMessage, error) {
	limit, offset := 100, 0
	if len(paging) > 0 {
		limit = max(1, min(paging[0], 100))
	}
	if len(paging) > 1 {
		offset = max(0, paging[1])
	}
	rows, e := s.db.QueryContext(ctx, `SELECT payload FROM research_records WHERE kind=? AND (?='' OR snapshot_id=?) ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, kind, parent, parent, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Store) Settings(ctx context.Context) (Settings, error) {
	v := DefaultSettings()
	var b []byte
	e := s.db.QueryRowContext(ctx, `SELECT payload FROM research_settings WHERE id=1`).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return v, nil
	}
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) SaveSettings(ctx context.Context, v Settings) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO research_settings VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`, b)
	return e
}

// The overview setting is independent of the agent role settings and archived snapshots.
func (s *Store) ObservationLine(ctx context.Context) (float64, error) {
	var v float64
	e := s.db.QueryRowContext(ctx, `SELECT observation_line FROM research_overview_settings WHERE id=1`).Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		return 18100, nil
	}
	return v, e
}
func (s *Store) SaveObservationLine(ctx context.Context, v float64) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO research_overview_settings VALUES(1,?) ON CONFLICT(id) DO UPDATE SET observation_line=excluded.observation_line`, v)
	return e
}

func (s *Store) Skills() *skillregistry.Store { return s.skills }
