package guanlan

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRefinementCompatibilityAndPersistence(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	d := mustDraft(t, s)
	i := d.Config.Items[0]
	var old Instance
	raw, _ := json.Marshal(i)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "refresh_seconds")
	style := fields["style"].(map[string]any)
	for _, k := range []string{"show_source", "scale_content", "scale_base_w", "scale_base_h"} {
		delete(style, k)
	}
	raw, _ = json.Marshal(fields)
	_ = json.Unmarshal(raw, &old)
	if e := ValidateInstance(old); e != nil {
		t.Fatal(e)
	}
	if old.Style.ShowSource != nil || old.Style.ScaleContent || old.RefreshSeconds != nil {
		t.Fatal("legacy defaults changed")
	}
	interval := 120
	i.RefreshSeconds = &interval
	source := false
	i.Style.ShowSource = &source
	i.Style.ScaleContent = true
	i.Style.ScaleBaseW = i.Layout.W
	i.Style.ScaleBaseH = i.Layout.H
	d.Config.Items[0] = i
	d, e := s.UpdateDraft(ctx, d.ID, d.Config, d.Revision)
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.SaveDraft(ctx, d.ID, d.Revision, false, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, flags := range []struct{ data, style bool }{{true, true}, {false, true}, {true, false}} {
		cloned, e := s.CreateDraft(ctx, CreateRequest{Name: "隔离测试复制", SourceID: v.Version.ID, CopyParams: flags.data, CopyStyle: flags.style})
		if e != nil {
			t.Fatal(e)
		}
		got := cloned.Config.Items[0]
		if flags.data && (got.RefreshSeconds == nil || *got.RefreshSeconds != 120) {
			t.Fatal("refresh lost on copy")
		}
		if !flags.data && got.RefreshSeconds != nil {
			t.Fatal("data copy reset missed interval")
		}
		if flags.style && (got.Style.ShowSource == nil || *got.Style.ShowSource || got.Style.ScaleBaseW != i.Layout.W) {
			t.Fatal("style copy lost refinement")
		}
	}
	p, e := s.PutPreset(ctx, Preset{Name: "隔离测试预设", Item: i})
	if e != nil {
		t.Fatal(e)
	}
	if p.Item.RefreshSeconds == nil || *p.Item.RefreshSeconds != 120 {
		t.Fatal("preset interval lost")
	}
}
func TestRefinementIntervalValidation(t *testing.T) {
	for _, n := range []int{-1, 1, 59, 86401} {
		i := Catalog()[0].Default
		i.RefreshSeconds = &n
		if ValidateInstance(i) == nil {
			t.Fatalf("accepted %d", n)
		}
	}
	for _, n := range []int{0, 60, 120, 301, 86400} {
		i := Catalog()[0].Default
		i.RefreshSeconds = &n
		if e := ValidateInstance(i); e != nil {
			t.Fatalf("rejected %d: %v", n, e)
		}
	}
}
