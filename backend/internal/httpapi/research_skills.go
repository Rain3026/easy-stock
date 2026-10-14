package httpapi

import (
	"context"
	"easy-stock/backend/internal/agent"
	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/skillregistry"
	"time"
)

func (s *Server) taskSkillBundles(ctx context.Context, service *agent.Service, j *dr.Job) ([]skillregistry.Bundle, error) {
	store := s.dailyResearch.store.Skills()
	if len(j.SkillCatalog) > 0 {
		out := []skillregistry.Bundle{}
		for _, ref := range j.SkillCatalog {
			b, e := store.Get(ctx, ref.Name, ref.Hash)
			if e != nil {
				return nil, e
			}
			if e = service.ProjectSkillBundle(ctx, b); e != nil {
				return nil, e
			}
			out = append(out, b)
		}
		return out, nil
	}
	bundles, e := service.SnapshotSkills(ctx)
	if e != nil {
		return nil, e
	}
	for i, b := range bundles {
		b, e = store.Register(ctx, b)
		if e != nil {
			return nil, e
		}
		bundles[i] = b
		j.SkillCatalog = append(j.SkillCatalog, skillregistry.Reference{Name: b.Name, Hash: b.Hash, LoadedAt: time.Now().UTC()})
	}
	return bundles, nil
}
