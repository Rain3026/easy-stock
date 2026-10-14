package dailyresearch

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var ErrPlanConflict = errors.New("portfolio plan revision changed")

type PortfolioVersion struct {
	Revision  int64           `json:"revision"`
	Hash      string          `json:"hash"`
	CreatedAt time.Time       `json:"created_at"`
	Origin    string          `json:"origin"`
	State     json.RawMessage `json:"state"`
}

func (s *Store) initPortfolio() error {
	_, e := s.db.Exec(`CREATE TABLE IF NOT EXISTS portfolio_plan_versions(revision INTEGER PRIMARY KEY,hash TEXT NOT NULL,created_at TEXT NOT NULL,origin TEXT NOT NULL,state BLOB NOT NULL); CREATE TRIGGER IF NOT EXISTS portfolio_versions_no_update BEFORE UPDATE ON portfolio_plan_versions BEGIN SELECT RAISE(ABORT,'immutable portfolio version'); END; CREATE TRIGGER IF NOT EXISTS portfolio_versions_no_delete BEFORE DELETE ON portfolio_plan_versions BEGIN SELECT RAISE(ABORT,'immutable portfolio version'); END;`)
	return e
}
func readPortfolio(row interface{ Scan(...any) error }) (PortfolioVersion, error) {
	var v PortfolioVersion
	var at string
	e := row.Scan(&v.Revision, &v.Hash, &at, &v.Origin, &v.State)
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, at)
	if errors.Is(e, sql.ErrNoRows) {
		return PortfolioVersion{State: json.RawMessage("null")}, nil
	}
	return v, e
}
func (s *Store) Portfolio(ctx context.Context) (PortfolioVersion, error) {
	return readPortfolio(s.db.QueryRowContext(ctx, `SELECT revision,hash,created_at,origin,state FROM portfolio_plan_versions ORDER BY revision DESC LIMIT 1`))
}
func (s *Store) SavePortfolio(ctx context.Context, state json.RawMessage, expected int64, origin string) (PortfolioVersion, error) {
	var raw any
	if e := json.Unmarshal(state, &raw); e != nil {
		return PortfolioVersion{}, e
	}
	state, e := json.Marshal(raw)
	if e != nil {
		return PortfolioVersion{}, e
	}
	h := sha256.Sum256(state)
	digest := hex.EncodeToString(h[:])
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return PortfolioVersion{}, e
	}
	defer tx.Rollback()
	old, e := readPortfolio(tx.QueryRowContext(ctx, `SELECT revision,hash,created_at,origin,state FROM portfolio_plan_versions ORDER BY revision DESC LIMIT 1`))
	if e != nil {
		return old, e
	}
	if old.Hash == digest {
		return old, nil
	}
	if old.Revision != expected {
		return old, ErrPlanConflict
	}
	v := PortfolioVersion{Revision: old.Revision + 1, Hash: digest, CreatedAt: time.Now().UTC(), Origin: origin, State: state}
	_, e = tx.ExecContext(ctx, `INSERT INTO portfolio_plan_versions VALUES(?,?,?,?,?)`, v.Revision, v.Hash, v.CreatedAt.Format(time.RFC3339Nano), origin, state)
	if e != nil {
		return old, e
	}
	e = tx.Commit()
	return v, e
}
func (s *Store) PortfolioHistory(ctx context.Context) ([]PortfolioVersion, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT revision,hash,created_at,origin,state FROM portfolio_plan_versions ORDER BY revision DESC LIMIT 100`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []PortfolioVersion{}
	for rows.Next() {
		v, e := readPortfolio(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
