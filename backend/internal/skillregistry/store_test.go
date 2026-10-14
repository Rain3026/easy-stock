package skillregistry

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"testing"
)

func TestVersionsRemainIndependent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bundle := Bundle{Name: "research", Files: map[string][]byte{"SKILL.md": []byte("original"), "reference/rules.txt": []byte("v1")}}
	old, err := s.Register(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Files["reference/rules.txt"] = []byte("v2")
	next, err := s.Register(ctx, bundle)
	if err != nil || next.Hash == old.Hash {
		t.Fatal("resource edit must produce a version", err)
	}
	original, err := s.Get(ctx, old.Name, old.Hash)
	if err != nil || string(original.Files["reference/rules.txt"]) != "v1" {
		t.Fatal("running task version changed", err)
	}
	if _, err = s.Register(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	versions, err := s.Versions(ctx)
	if err != nil || len(versions) != 2 {
		t.Fatal("duplicate versions", err, len(versions))
	}
	bundle.Files["../outside"] = []byte("bad")
	if _, err = s.Register(ctx, bundle); err == nil {
		t.Fatal("unsafe resource accepted")
	}
}
