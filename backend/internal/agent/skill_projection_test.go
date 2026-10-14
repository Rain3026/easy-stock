package agent

import (
	"context"
	"easy-stock/backend/internal/skillregistry"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedSkillsPinEachTaskAndBothRuntimeProjections(t *testing.T) {
	s := testService(t)
	original := []byte("---\nname: versioned-research\ndescription: test method\n---\nOld method\n")
	if _, e := s.ImportSkills([]SkillImportFile{{Name: "research/versioned-research/SKILL.md", Data: original}}); e != nil {
		t.Fatal(e)
	}
	old, release, e := s.BindTask(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	before, e := s.SnapshotSkills(old)
	if e != nil {
		t.Fatal(e)
	}
	s.SetSkillResolver(func(_ context.Context, b skillregistry.Bundle) (skillregistry.Bundle, error) {
		if b.Name == "versioned-research" {
			b.Files["SKILL.md"] = append(original, []byte("New version\n")...)
		}
		return skillregistry.Validate(b)
	})
	next, cleanup, e := s.BindTask(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer cleanup()
	after, e := s.SnapshotSkills(next)
	if e != nil {
		t.Fatal(e)
	}
	hashes := func(bs []skillregistry.Bundle) map[string]string {
		m := map[string]string{}
		for _, b := range bs {
			m[b.Name] = b.Hash
		}
		return m
	}
	if hashes(before)["versioned-research"] == hashes(after)["versioned-research"] {
		t.Fatal("new task did not receive publication")
	}
	retained, _ := s.SnapshotSkills(old)
	if hashes(retained)["versioned-research"] != hashes(before)["versioned-research"] {
		t.Fatal("old task modified")
	}
	binding := next.Value(bindingKey{}).(*taskBinding)
	codexHome := t.TempDir()
	if e = binding.codex.projectSkills(codexHome); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(codexHome, "skills", "versioned-research", "SKILL.md"))
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range after {
		if b.Name == "versioned-research" && string(b.Files["SKILL.md"]) != string(raw) {
			t.Fatal("Hermes/Codex projection differs")
		}
	}
	shared, _ := s.SnapshotSkills(context.Background())
	if hashes(shared)["versioned-research"] != hashes(before)["versioned-research"] {
		t.Fatal("publisher changed user installed source")
	}
}
