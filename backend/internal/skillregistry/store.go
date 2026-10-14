// Package skillregistry retains the exact files loaded by a research task.
package skillregistry

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

type Bundle struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Hash        string            `json:"hash"`
	Files       map[string][]byte `json:"files"`
}
type Reference struct {
	Name     string    `json:"name"`
	Hash     string    `json:"hash"`
	LoadedAt time.Time `json:"loaded_at"`
}
type Store struct{ db *sql.DB }

func Open(db *sql.DB) (*Store, error) {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS skill_versions(name TEXT NOT NULL,hash TEXT NOT NULL,created_at TEXT NOT NULL,payload BLOB NOT NULL,PRIMARY KEY(name,hash));CREATE TABLE IF NOT EXISTS skill_events(id TEXT PRIMARY KEY,kind TEXT NOT NULL,name TEXT NOT NULL,created_at TEXT NOT NULL,payload BLOB NOT NULL);`)
	if e == nil {
		_, e = db.Exec(`CREATE TABLE IF NOT EXISTS skill_active(name TEXT PRIMARY KEY,hash TEXT NOT NULL);
 CREATE TRIGGER IF NOT EXISTS skill_versions_no_update BEFORE UPDATE ON skill_versions BEGIN SELECT RAISE(ABORT,'immutable skill version'); END;
 CREATE TRIGGER IF NOT EXISTS skill_versions_no_delete BEFORE DELETE ON skill_versions BEGIN SELECT RAISE(ABORT,'immutable skill version'); END;
 CREATE TRIGGER IF NOT EXISTS skill_events_no_update BEFORE UPDATE ON skill_events BEGIN SELECT RAISE(ABORT,'immutable skill event'); END;
 CREATE TRIGGER IF NOT EXISTS skill_events_no_delete BEFORE DELETE ON skill_events BEGIN SELECT RAISE(ABORT,'immutable skill event'); END;`)
	}
	return &Store{db}, e
}
func Validate(b Bundle) (Bundle, error) {
	if b.Name == "" || b.Name == "." || b.Name == ".." || strings.ContainsAny(b.Name, "/\\") || len(b.Files) == 0 || len(b.Files) > 1000 || len(b.Files["SKILL.md"]) == 0 {
		return b, fmt.Errorf("invalid skill bundle")
	}
	names := []string{}
	total := 0
	for n, v := range b.Files {
		if path.Clean(n) != n || path.IsAbs(n) || strings.HasPrefix(n, "..") || strings.Contains(n, "\\") {
			return b, fmt.Errorf("invalid skill resource path")
		}
		total += len(v)
		names = append(names, n)
	}
	if total > 64<<20 {
		return b, fmt.Errorf("skill bundle too large")
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		fmt.Fprintf(h, "%d:%s:%d:", len(n), n, len(b.Files[n]))
		h.Write(b.Files[n])
	}
	b.Hash = hex.EncodeToString(h.Sum(nil))
	return b, nil
}
func (s *Store) Register(ctx context.Context, b Bundle) (Bundle, error) {
	v, e := Validate(b)
	if e != nil {
		return v, e
	}
	raw, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO skill_versions VALUES(?,?,?,?) ON CONFLICT(name,hash) DO NOTHING`, v.Name, v.Hash, time.Now().UTC().Format(time.RFC3339Nano), raw)
	return v, e
}
func (s *Store) Get(ctx context.Context, name, hash string) (Bundle, error) {
	var b Bundle
	var raw []byte
	e := s.db.QueryRowContext(ctx, `SELECT payload FROM skill_versions WHERE name=? AND hash=?`, name, hash).Scan(&raw)
	if e == nil {
		e = json.Unmarshal(raw, &b)
	}
	return b, e
}
func (s *Store) Versions(ctx context.Context) ([]Reference, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT name,hash,created_at FROM skill_versions ORDER BY created_at DESC LIMIT 200`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Reference{}
	for rows.Next() {
		var v Reference
		var at string
		if e = rows.Scan(&v.Name, &v.Hash, &at); e != nil {
			return nil, e
		}
		v.LoadedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, v)
	}
	return out, rows.Err()
}
