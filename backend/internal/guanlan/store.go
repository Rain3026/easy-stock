package guanlan

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easy-stock/backend/internal/foundation"
	_ "modernc.org/sqlite"
)

var ErrConflict = errors.New("配置已被其他窗口修改，本地草稿保留，请重新读取后处理")
var ErrNotFound = errors.New("配置或对象不存在")
var ErrInvalid = errors.New("配置无效")

type Store struct{ db *sql.DB }

func NewID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Open(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	}
	if path != ":memory:" {
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return nil, e
		}
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	var version int
	if e = db.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return fail(e)
	}
	if version > Schema {
		return fail(errors.New("看板数据库版本较新，原文件保留"))
	}
	_, e = db.Exec(`PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS board_state(id INTEGER PRIMARY KEY CHECK(id=1),payload BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS board_versions(id TEXT PRIMARY KEY,number INTEGER UNIQUE NOT NULL,source_id TEXT NOT NULL,created_at TEXT NOT NULL,hash TEXT NOT NULL,payload BLOB NOT NULL,saved_key TEXT UNIQUE NOT NULL);
 CREATE TABLE IF NOT EXISTS board_version_meta(id TEXT PRIMARY KEY,name TEXT NOT NULL,archived INTEGER NOT NULL DEFAULT 0,revision INTEGER NOT NULL DEFAULT 1);
 CREATE TRIGGER IF NOT EXISTS board_versions_immutable_update BEFORE UPDATE ON board_versions BEGIN SELECT RAISE(ABORT,'saved layout is immutable'); END;
 CREATE TRIGGER IF NOT EXISTS board_versions_immutable_delete BEFORE DELETE ON board_versions BEGIN SELECT RAISE(ABORT,'saved layout is immutable'); END;
 PRAGMA user_version=1;`)
	if e != nil {
		return fail(e)
	}
	initial := State{Drafts: map[string]Draft{}, Library: map[string]Preference{}, Presets: map[string]Preset{}, Watchlists: map[string]Watchlist{}}
	b, _ := json.Marshal(initial)
	_, e = db.Exec("INSERT OR IGNORE INTO board_state VALUES(1,?)", b)
	if e != nil {
		return fail(e)
	}
	return s, nil
}
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}
func readState(ctx context.Context, tx *sql.Tx) (State, error) {
	var v State
	var b []byte
	e := tx.QueryRowContext(ctx, "SELECT payload FROM board_state WHERE id=1").Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) Snapshot(ctx context.Context) (State, error) {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return State{}, e
	}
	defer tx.Rollback()
	v, e := readState(ctx, tx)
	if e != nil {
		return v, e
	}
	return v, tx.Commit()
}
func (s *Store) mutate(ctx context.Context, fn func(*sql.Tx, *State) error) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	state, e := readState(ctx, tx)
	if e != nil {
		return e
	}
	if e = fn(tx, &state); e != nil {
		return e
	}
	state.Revision++
	b, e := json.Marshal(state)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE board_state SET payload=? WHERE id=1", b); e != nil {
		return e
	}
	return tx.Commit()
}
func invalid(e error) error { return fmt.Errorf("%w: %v", ErrInvalid, e) }
func configCheck(c Config) error {
	if e := ValidateConfig(c); e != nil {
		return invalid(e)
	}
	return nil
}

type CreateRequest struct {
	Name       string `json:"name"`
	SourceID   string `json:"source_id"`
	CopyParams bool   `json:"copy_params"`
	CopyStyle  bool   `json:"copy_style"`
}

func (s *Store) CreateDraft(ctx context.Context, req CreateRequest) (Draft, error) {
	var out Draft
	if !validName(req.Name) {
		return out, invalid(errors.New("请输入80字以内的配置名称"))
	}
	e := s.mutate(ctx, func(tx *sql.Tx, state *State) error {
		if len(state.Drafts) >= 100 {
			return invalid(errors.New("草稿数量已达上限，请先清理不用的草稿"))
		}
		c := Config{Schema: Schema, Name: strings.TrimSpace(req.Name), Snap: true, Items: []Instance{}}
		if req.SourceID != "" {
			v, e := versionTx(ctx, tx, req.SourceID)
			if e != nil {
				return e
			}
			c = Clone(v.Config)
			c.Name = strings.TrimSpace(req.Name)
			nextY := 0.0
			for n := range c.Items {
				i := &c.Items[n]
				i.ID = NewID()
				d, ok := Find(i.Type)
				if !ok {
					continue
				}
				def := d.Default
				if p := state.Library[i.Type]; p.Default != nil {
					def = Clone(*p.Default)
				}
				if !req.CopyParams {
					i.Params = Clone(def.Params)
				}
				if !req.CopyStyle {
					i.Style = def.Style
					i.Layout = def.Layout
					i.Layout.Y = nextY
					nextY += i.Layout.H
				}
			}
		}
		now := time.Now().UTC()
		out = Draft{ID: NewID(), Revision: 1, SourceID: req.SourceID, Config: c, CreatedAt: now, UpdatedAt: now}
		state.Drafts[out.ID] = out
		return nil
	})
	return out, e
}
func (s *Store) UpdateDraft(ctx context.Context, id string, c Config, expected int64) (Draft, error) {
	var out Draft
	if e := configCheck(c); e != nil {
		return out, e
	}
	e := s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		v, ok := state.Drafts[id]
		if !ok {
			return ErrNotFound
		}
		if v.Revision != expected {
			return ErrConflict
		}
		v.Config = Clone(c)
		v.Revision++
		v.UpdatedAt = time.Now().UTC()
		state.Drafts[id] = v
		out = v
		return nil
	})
	return out, e
}
func (s *Store) DiscardDraft(ctx context.Context, id string, expected int64) error {
	return s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		v, ok := state.Drafts[id]
		if !ok {
			return ErrNotFound
		}
		if v.Revision != expected {
			return ErrConflict
		}
		delete(state.Drafts, id)
		return nil
	})
}
func versionTx(ctx context.Context, tx *sql.Tx, id string) (Version, error) {
	var v Version
	var raw []byte
	var created string
	e := tx.QueryRowContext(ctx, `SELECT v.id,v.number,v.source_id,v.created_at,v.hash,v.payload,m.name,m.archived,m.revision FROM board_versions v JOIN board_version_meta m ON m.id=v.id WHERE v.id=?`, id).Scan(&v.ID, &v.Number, &v.SourceID, &created, &v.Hash, &raw, &v.Name, &v.Archived, &v.Revision)
	if errors.Is(e, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	e = json.Unmarshal(raw, &v.Config)
	return v, e
}
func (s *Store) Version(ctx context.Context, id string) (Version, error) {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return Version{}, e
	}
	defer tx.Rollback()
	v, e := versionTx(ctx, tx, id)
	if e != nil {
		return v, e
	}
	return v, tx.Commit()
}
func (s *Store) SaveDraft(ctx context.Context, id string, expected int64, activate bool, activeRevision int64) (SaveResult, error) {
	var out SaveResult
	e := s.mutate(ctx, func(tx *sql.Tx, state *State) error {
		draft, ok := state.Drafts[id]
		if !ok {
			return ErrNotFound
		}
		if draft.Revision != expected {
			return ErrConflict
		}
		if e := configCheck(draft.Config); e != nil {
			return e
		}
		if activate && state.Active.Revision != activeRevision {
			return ErrConflict
		}
		key := fmt.Sprintf("%s:%d", id, expected)
		var savedID string
		e := tx.QueryRowContext(ctx, "SELECT id FROM board_versions WHERE saved_key=?", key).Scan(&savedID)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if savedID == "" {
			savedID = NewID()
			var number int64
			if e = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(number),0)+1 FROM board_versions").Scan(&number); e != nil {
				return e
			}
			raw, e := json.Marshal(draft.Config)
			if e != nil {
				return e
			}
			hash := sha256.Sum256(raw)
			now := time.Now().UTC().Format(time.RFC3339Nano)
			_, e = tx.ExecContext(ctx, "INSERT INTO board_versions VALUES(?,?,?,?,?,?,?)", savedID, number, draft.SourceID, now, hex.EncodeToString(hash[:]), raw, key)
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "INSERT INTO board_version_meta(id,name) VALUES(?,?)", savedID, draft.Config.Name); e != nil {
				return e
			}
		}
		v, e := versionTx(ctx, tx, savedID)
		if e != nil {
			return e
		}
		if activate {
			if v.Archived {
				return invalid(errors.New("请先恢复归档版本"))
			}
			if state.Active.VersionID != savedID {
				state.Active = Active{VersionID: savedID, Revision: state.Active.Revision + 1}
			}
		}
		out = SaveResult{Version: v, Active: state.Active}
		// Continuing this draft derives from the just-saved immutable version.
		draft.SourceID = savedID
		state.Drafts[id] = draft
		return nil
	})
	return out, e
}
func (s *Store) Activate(ctx context.Context, id string, expected int64) (Active, error) {
	var out Active
	e := s.mutate(ctx, func(tx *sql.Tx, state *State) error {
		if state.Active.Revision != expected {
			return ErrConflict
		}
		v, e := versionTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if v.Archived {
			return invalid(errors.New("请先恢复归档版本"))
		}
		if state.Active.VersionID != id {
			state.Active = Active{VersionID: id, Revision: state.Active.Revision + 1}
		}
		out = state.Active
		return nil
	})
	return out, e
}
func (s *Store) Versions(ctx context.Context, q string, page, size int, archived bool) (Page, error) {
	result := Page{Items: []Version{}, Page: page, PageSize: size}
	if page < 1 || size < 1 || size > 20 || len(q) > 200 {
		return result, invalid(errors.New("无效分页或搜索"))
	}
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return result, e
	}
	defer tx.Rollback()
	// instr treats percent/underscore as literal text rather than LIKE wildcards.
	where := " FROM board_versions v JOIN board_version_meta m ON m.id=v.id WHERE m.archived=? AND (?='' OR instr(lower(m.name),lower(?))>0)"
	if e = tx.QueryRowContext(ctx, "SELECT count(*)"+where, archived, q, q).Scan(&result.Total); e != nil {
		return result, e
	}
	rows, e := tx.QueryContext(ctx, "SELECT v.id"+where+" ORDER BY v.number DESC LIMIT ? OFFSET ?", archived, q, q, size, (page-1)*size)
	if e != nil {
		return result, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return result, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, e
	}
	for _, id := range ids {
		v, e := versionTx(ctx, tx, id)
		if e != nil {
			return result, e
		}
		result.Items = append(result.Items, v)
	}
	return result, tx.Commit()
}
func (s *Store) Metadata(ctx context.Context, id string, name *string, archived *bool, expected int64) (Version, error) {
	var out Version
	e := s.mutate(ctx, func(tx *sql.Tx, state *State) error {
		v, e := versionTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if v.Revision != expected {
			return ErrConflict
		}
		if name != nil {
			if !validName(*name) {
				return invalid(errors.New("配置名称无效"))
			}
			v.Name = strings.TrimSpace(*name)
		}
		if archived != nil {
			if *archived && state.Active.VersionID == id {
				return invalid(errors.New("请先启用其他版本，再归档当前版本"))
			}
			v.Archived = *archived
		}
		_, e = tx.ExecContext(ctx, "UPDATE board_version_meta SET name=?,archived=?,revision=revision+1 WHERE id=?", v.Name, v.Archived, id)
		if e != nil {
			return e
		}
		out, e = versionTx(ctx, tx, id)
		return e
	})
	return out, e
}
func (s *Store) Preferences(ctx context.Context, prefs map[string]Preference, expected int64) error {
	for id, p := range prefs {
		if _, ok := Find(id); !ok {
			return invalid(errors.New("组件类型不存在"))
		}
		if p.Default != nil {
			if p.Default.Type != id {
				return invalid(errors.New("默认类型不匹配"))
			}
			if e := ValidateInstance(*p.Default); e != nil {
				return invalid(e)
			}
		}
	}
	return s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		if state.LibraryRevision != expected {
			return ErrConflict
		}
		state.Library = Clone(prefs)
		state.LibraryRevision++
		if state.Library == nil {
			state.Library = map[string]Preference{}
		}
		return nil
	})
}
func (s *Store) PutPreset(ctx context.Context, p Preset) (Preset, error) {
	if !validName(p.Name) {
		return p, invalid(errors.New("预设名称无效"))
	}
	if e := ValidateInstance(p.Item); e != nil {
		return p, invalid(e)
	}
	e := s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		if p.ID == "" {
			if len(state.Presets) >= 200 {
				return invalid(errors.New("预设数量达到上限"))
			}
			p.ID = NewID()
			p.Revision = 1
		} else {
			old, ok := state.Presets[p.ID]
			if !ok {
				return ErrNotFound
			}
			if old.Revision != p.Revision {
				return ErrConflict
			}
			p.Revision++
		}
		p.Item = Clone(p.Item)
		p.Item.ID = ""
		state.Presets[p.ID] = p
		return nil
	})
	return p, e
}
func (s *Store) DeletePreset(ctx context.Context, id string, expected int64) error {
	return s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		old, ok := state.Presets[id]
		if !ok {
			return ErrNotFound
		}
		if old.Revision != expected {
			return ErrConflict
		}
		delete(state.Presets, id)
		return nil
	})
}
func (s *Store) PutWatchlist(ctx context.Context, list Watchlist) (Watchlist, error) {
	if !validName(list.Name) || list.Symbols == nil || len(list.Symbols) > 100 {
		return list, invalid(errors.New("关注名单名称无效或超过100只"))
	}
	symbols := []string{}
	seen := map[string]bool{}
	for _, symbol := range list.Symbols {
		v, e := foundation.NormalizeSymbol(symbol)
		if e != nil {
			return list, invalid(e)
		}
		if !seen[v.Canonical] {
			symbols = append(symbols, v.Canonical)
			seen[v.Canonical] = true
		}
	}
	list.Symbols = symbols
	e := s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		if list.ID == "" {
			if len(state.Watchlists) >= 100 {
				return invalid(errors.New("关注名单数量达到上限"))
			}
			list.ID = NewID()
			list.Revision = 1
		} else {
			old, ok := state.Watchlists[list.ID]
			if !ok {
				return ErrNotFound
			}
			if old.Revision != list.Revision {
				return ErrConflict
			}
			list.Revision++
		}
		state.Watchlists[list.ID] = list
		return nil
	})
	return list, e
}

type Reference struct {
	VersionID string `json:"version_id"`
	Number    int64  `json:"number"`
	Name      string `json:"name"`
	Count     int    `json:"count"`
	Archived  bool   `json:"archived"`
}

func (s *Store) References(ctx context.Context) (map[string][]Reference, error) {
	result := map[string][]Reference{}
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, "SELECT id FROM board_versions ORDER BY number DESC")
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for _, id := range ids {
		v, e := versionTx(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		counts := map[string]int{}
		for _, item := range v.Config.Items {
			counts[item.Type]++
		}
		for typ, count := range counts {
			result[typ] = append(result[typ], Reference{VersionID: id, Number: v.Number, Name: v.Name, Count: count, Archived: v.Archived})
		}
	}
	return result, tx.Commit()
}
