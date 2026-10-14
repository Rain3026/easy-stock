package guanlan

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"sync"
	"testing"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "guanlan.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}
func mustDraft(t *testing.T, s *Store) Draft {
	t.Helper()
	d, e := s.CreateDraft(context.Background(), CreateRequest{Name: "盘中配置"})
	if e != nil {
		t.Fatal(e)
	}
	item := Catalog()[0].Default
	item.ID = NewID()
	d.Config.Items = append(d.Config.Items, item)
	d, e = s.UpdateDraft(context.Background(), d.ID, d.Config, d.Revision)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestGuanlanVersionImmutabilityAndSaveIdempotency(t *testing.T) {
	ctx := context.Background()
	s, _ := testStore(t)
	d := mustDraft(t, s)
	first, e := s.SaveDraft(ctx, d.ID, d.Revision, false, 0)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.SaveDraft(ctx, d.ID, d.Revision, false, 0)
	if e != nil || retry.Version.ID != first.Version.ID {
		t.Fatal("retry duplicated version", e)
	}
	state, _ := s.Snapshot(ctx)
	if state.Active.VersionID != "" {
		t.Fatal("save unexpectedly enabled")
	}
	d.Config.Items[0].Style.Title = "改后的标题"
	d, e = s.UpdateDraft(ctx, d.ID, d.Config, d.Revision)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.SaveDraft(ctx, d.ID, d.Revision, true, 0)
	if e != nil {
		t.Fatal(e)
	}
	old, _ := s.Version(ctx, first.Version.ID)
	if old.Config.Items[0].Style.Title == "改后的标题" || old.Hash != first.Version.Hash || second.Version.Number != 2 || second.Version.SourceID != first.Version.ID {
		t.Fatal("historical version mutated")
	}
	alias := "新的显示名"
	old, e = s.Metadata(ctx, old.ID, &alias, nil, old.Revision)
	if e != nil || old.Config.Name != first.Version.Config.Name || old.Name != alias {
		t.Fatal("alias changed frozen config", e)
	}
}
func TestGuanlanAtomicActivationAndArchiveGuard(t *testing.T) {
	ctx := context.Background()
	s, _ := testStore(t)
	d := mustDraft(t, s)
	one, e := s.SaveDraft(ctx, d.ID, d.Revision, true, 0)
	if e != nil {
		t.Fatal(e)
	}
	d.Config.Name = "下一版"
	d, _ = s.UpdateDraft(ctx, d.ID, d.Config, d.Revision)
	_, e = s.SaveDraft(ctx, d.ID, d.Revision, true, 0)
	if !errors.Is(e, ErrConflict) {
		t.Fatal("stale active revision accepted", e)
	}
	page, _ := s.Versions(ctx, "", 1, 2, false)
	if page.Total != 1 {
		t.Fatal("failed atomic activation left a version")
	}
	archive := true
	if _, e = s.Metadata(ctx, one.Version.ID, nil, &archive, one.Version.Revision); !errors.Is(e, ErrInvalid) {
		t.Fatal("active version archived", e)
	}
	two, e := s.SaveDraft(ctx, d.ID, d.Revision, true, 1)
	if e != nil {
		t.Fatal(e)
	}
	archived, e := s.Metadata(ctx, one.Version.ID, nil, &archive, one.Version.Revision)
	if e != nil || !archived.Archived {
		t.Fatal(e)
	}
	if _, e = s.Activate(ctx, one.Version.ID, two.Active.Revision); !errors.Is(e, ErrInvalid) {
		t.Fatal("archived version enabled")
	}
	archive = false
	restored, e := s.Metadata(ctx, one.Version.ID, nil, &archive, archived.Revision)
	if e != nil || restored.Hash != one.Version.Hash {
		t.Fatal("restore mutated history", e)
	}
}
func TestGuanlanCloneDefaultsPresetAndRestartIsolation(t *testing.T) {
	ctx := context.Background()
	s, path := testStore(t)
	d := mustDraft(t, s)
	v, _ := s.SaveDraft(ctx, d.ID, d.Revision, false, 0)
	clone, e := s.CreateDraft(ctx, CreateRequest{Name: "复制版", SourceID: v.Version.ID, CopyParams: true, CopyStyle: true})
	if e != nil {
		t.Fatal(e)
	}
	if clone.Config.Items[0].ID == d.Config.Items[0].ID {
		t.Fatal("clone shared instance identity")
	}
	state, _ := s.Snapshot(ctx)
	def := Catalog()[0].Default
	def.Style.Title = "以后新增默认"
	prefs := map[string]Preference{"volume": {Default: &def, Hidden: true}}
	if e = s.Preferences(ctx, prefs, state.LibraryRevision); e != nil {
		t.Fatal(e)
	}
	preset, e := s.PutPreset(ctx, Preset{Name: "量能预设", Item: def})
	if e != nil {
		t.Fatal(e)
	}
	preset.Item.Style.Title = "预设更新"
	_, e = s.PutPreset(ctx, preset)
	if e != nil {
		t.Fatal(e)
	}
	original, _ := s.Version(ctx, v.Version.ID)
	if original.Config.Items[0].Style.Title != d.Config.Items[0].Style.Title {
		t.Fatal("defaults or preset mutated instance")
	}
	s.Close()
	reopened, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	state, e = reopened.Snapshot(ctx)
	if e != nil || len(state.Drafts) != 2 || len(state.Presets) != 1 || !state.Library["volume"].Hidden {
		t.Fatal("restart lost configs", e)
	}
	page, e := reopened.Versions(ctx, "盘中", 1, 2, false)
	if e != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatal("version search/paging", e)
	}
}
func TestGuanlanConcurrentDraftEditAndValidation(t *testing.T) {
	ctx := context.Background()
	s, _ := testStore(t)
	d := mustDraft(t, s)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.UpdateDraft(ctx, d.ID, d.Config, d.Revision); errs <- e }()
	}
	wg.Wait()
	close(errs)
	wins, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			wins++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("concurrent edit silently overwrote")
	}
	bad := Clone(d.Config)
	bad.Items = append(bad.Items, bad.Items[0])
	if ValidateConfig(bad) == nil {
		t.Fatal("duplicate instance accepted")
	}
	bad = Clone(d.Config)
	bad.Items[0].Params["command"] = "run-mcp"
	if ValidateConfig(bad) == nil {
		t.Fatal("arbitrary tool binding accepted")
	}
	bad = Clone(d.Config)
	bad.Items[0].Layout.W = 1
	if ValidateConfig(bad) == nil {
		t.Fatal("unreadable width accepted")
	}
}
func TestGuanlanWatchlistSharedAndVersionReferences(t *testing.T) {
	ctx := context.Background()
	s, _ := testStore(t)
	list, e := s.PutWatchlist(ctx, Watchlist{Name: "关注", Symbols: []string{"600000.SH", "600000.SH"}})
	if e != nil || len(list.Symbols) != 1 {
		t.Fatal("watchlist normalize/dedup", e)
	}
	list.Name = "新的名称"
	updated, e := s.PutWatchlist(ctx, list)
	if e != nil || updated.Revision != 2 {
		t.Fatal(e)
	}
	if _, e = s.PutWatchlist(ctx, list); !errors.Is(e, ErrConflict) {
		t.Fatal("watchlist lost update")
	}
	d := mustDraft(t, s)
	one, _ := s.SaveDraft(ctx, d.ID, d.Revision, false, 0)
	clone, _ := s.CreateDraft(ctx, CreateRequest{Name: "复制", SourceID: one.Version.ID, CopyParams: true, CopyStyle: true})
	_, _ = s.SaveDraft(ctx, clone.ID, clone.Revision, false, 0)
	refs, e := s.References(ctx)
	if e != nil || len(refs["volume"]) != 2 {
		t.Fatal("references must count configuration versions", e)
	}
	if e = s.DiscardDraft(ctx, d.ID, d.Revision); e != nil {
		t.Fatal(e)
	}
	state, _ := s.Snapshot(ctx)
	if len(state.Watchlists) != 1 {
		t.Fatal("removing draft removed business list")
	}
}

func TestGuanlanPixelPrecisionAndDatabaseImmutability(t *testing.T) {
	ctx := context.Background()
	s, _ := testStore(t)
	d := mustDraft(t, s)
	d.Config.Items[0].Layout.W = 53.3333333333
	d.Config.Items[0].Layout.H = 25.0833333333
	d, e := s.UpdateDraft(ctx, d.ID, d.Config, d.Revision)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := s.SaveDraft(ctx, d.ID, d.Revision, false, 0)
	if e != nil {
		t.Fatal(e)
	}
	if saved.Version.Config.Items[0].Layout.W != d.Config.Items[0].Layout.W {
		t.Fatal("pixel precision lost")
	}
	if _, e = s.db.Exec("UPDATE board_versions SET hash='changed' WHERE id=?", saved.Version.ID); e == nil {
		t.Fatal("SQL could rewrite immutable version")
	}
	if _, e = s.db.Exec("DELETE FROM board_versions WHERE id=?", saved.Version.ID); e == nil {
		t.Fatal("SQL could delete immutable version")
	}
	bad := d.Config.Items[0]
	bad.Layout.W = math.NaN()
	if ValidateInstance(bad) == nil {
		t.Fatal("non-finite size accepted")
	}
}

func TestGuanlanPresetDeleteAndStyleResetDoNotMutateVersions(t *testing.T) {
	ctx := context.Background()
	s, _ := testStore(t)
	d := mustDraft(t, s)
	second := Catalog()[1].Default
	second.ID = NewID()
	d.Config.Items = append(d.Config.Items, second)
	d, e := s.UpdateDraft(ctx, d.ID, d.Config, d.Revision)
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.SaveDraft(ctx, d.ID, d.Revision, false, 0)
	if e != nil {
		t.Fatal(e)
	}
	reset, e := s.CreateDraft(ctx, CreateRequest{Name: "reset", SourceID: v.Version.ID, CopyParams: true, CopyStyle: false})
	if e != nil {
		t.Fatal(e)
	}
	if reset.Config.Items[1].Layout.Y < reset.Config.Items[0].Layout.H {
		t.Fatal("reset style clone overlaps")
	}
	preset, e := s.PutPreset(ctx, Preset{Name: "fixture", Item: d.Config.Items[0]})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DeletePreset(ctx, preset.ID, preset.Revision+1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale preset deletion accepted", e)
	}
	if e = s.DeletePreset(ctx, preset.ID, preset.Revision); e != nil {
		t.Fatal(e)
	}
	original, e := s.Version(ctx, v.Version.ID)
	if e != nil || original.Hash != v.Version.Hash || len(original.Config.Items) != 2 {
		t.Fatal("preset deletion modified layout", e)
	}
}
