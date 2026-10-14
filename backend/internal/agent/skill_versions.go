package agent

import (
	"context"
	"easy-stock/backend/internal/skillregistry"
	"fmt"
	"os"
	"path/filepath"
)

// SnapshotSkills reads the already bound task copy, so concurrent settings or
// imports cannot change a running task's version.
func (s *Service) SnapshotSkills(ctx context.Context) ([]skillregistry.Bundle, error) {
	r := s.HermesRuntime
	if b, ok := ctx.Value(bindingKey{}).(*taskBinding); ok && b != nil && b.owner == s {
		r = b.hermes
	}
	settings, e := r.AgentSettings()
	if e != nil {
		return nil, e
	}
	out := []skillregistry.Bundle{}
	for _, skill := range settings.Skills {
		if !skill.Enabled {
			continue
		}
		b := skillregistry.Bundle{Name: skill.Name, Description: skill.Description, Category: skill.Category, Files: map[string][]byte{}}
		total := 0
		e = filepath.WalkDir(skill.directory, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("Skill contains symlink")
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() || info.Size() > 8<<20 {
				return fmt.Errorf("invalid skill file")
			}
			raw, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			total += len(raw)
			if total > 64<<20 {
				return fmt.Errorf("skill too large")
			}
			rel, e := filepath.Rel(skill.directory, p)
			if e != nil {
				return e
			}
			b.Files[filepath.ToSlash(rel)] = raw
			return nil
		})
		if e != nil {
			return nil, e
		}
		b, e = skillregistry.Validate(b)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, nil
}

func (s *Service) SetSkillResolver(resolve func(context.Context, skillregistry.Bundle) (skillregistry.Bundle, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skillResolver = resolve
}

// Only task-local copies are replaced. The shared installed directory and a
// different running task are never modified by a publication or rollback.
func (s *Service) ProjectSkillBundle(ctx context.Context, bundle skillregistry.Bundle) error {
	b, ok := ctx.Value(bindingKey{}).(*taskBinding)
	if !ok || b == nil || b.owner != s {
		return fmt.Errorf("skill projection requires a task binding")
	}
	v, e := skillregistry.Validate(bundle)
	if e != nil {
		return e
	}
	settings, e := b.hermes.AgentSettings()
	if e != nil {
		return e
	}
	for _, info := range settings.Skills {
		if info.Name != v.Name {
			continue
		}
		rel, e := filepath.Rel(b.hermes.home, info.directory)
		if e != nil || rel == ".." || filepath.IsAbs(rel) || len(rel) > 3 && rel[:3] == "../" {
			return fmt.Errorf("skill projection outside task")
		}
		tmp, e := os.MkdirTemp(filepath.Dir(info.directory), ".skill-version-")
		if e != nil {
			return e
		}
		defer os.RemoveAll(tmp)
		for name, raw := range v.Files {
			target := filepath.Join(tmp, filepath.FromSlash(name))
			if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				return e
			}
			if e = os.WriteFile(target, raw, 0600); e != nil {
				return e
			}
		}
		// The target is a newly-created task copy; its source is retained in the
		// immutable registry. No user-owned directory is removed.
		if e = os.RemoveAll(info.directory); e != nil {
			return e
		}
		return os.Rename(tmp, info.directory)
	}
	return fmt.Errorf("enabled skill %s not found in task", v.Name)
}
