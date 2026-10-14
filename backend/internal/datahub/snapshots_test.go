package datahub

import (
	"context"
	"database/sql"
	"encoding/json"
	_ "modernc.org/sqlite"
	"testing"
	"time"
)

func TestImmutableSnapshotBackfillAndIdentity(t *testing.T) {
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	s, e := NewFactStore(db)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	at := time.Now().UTC()
	v := FactSnapshot{EntityID: "stock:600176.SH", Kind: "facts", OriginID: "old-native-record", MethodVersion: "v1", CapturedAt: at, CutoffAt: at, Payload: json.RawMessage(`{"price":10}`)}
	a, e := s.Freeze(ctx, v)
	if e != nil {
		t.Fatal(e)
	}
	same, e := s.Freeze(ctx, v)
	if e != nil || same.ID != a.ID {
		t.Fatal("duplicate freeze", e)
	}
	v.ParentID = a.ID
	v.Payload = json.RawMessage(`{"price":11}`)
	b, e := s.Freeze(ctx, v)
	if e != nil || a.ID == b.ID {
		t.Fatal("backfill must create new snapshot", e)
	}
	original, e := s.Get(ctx, a.ID)
	if e != nil || string(original.Payload) != `{"price":10}` {
		t.Fatal("original changed")
	}
	if _, e = db.Exec(`UPDATE fact_snapshots SET payload='{}' WHERE id=?`, a.ID); e == nil {
		t.Fatal("mutable evidence")
	}
	if _, e = db.Exec(`DELETE FROM fact_snapshots WHERE id=?`, a.ID); e == nil {
		t.Fatal("deletable original")
	}
	v.EntityID = "portfolio:other"
	if _, e = s.Freeze(ctx, v); e == nil {
		t.Fatal("foreign parent allowed")
	}
	rows, e := s.List(ctx, "stock:600176.SH", 20)
	if e != nil || len(rows) != 2 || rows[0].Payload != nil {
		t.Fatal(rows, e)
	}
}
