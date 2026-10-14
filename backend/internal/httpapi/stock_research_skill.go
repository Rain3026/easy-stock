package httpapi

import (
	"context"
	"easy-stock/backend/internal/agent"
	dr "easy-stock/backend/internal/dailyresearch"
	"easy-stock/backend/internal/skillregistry"
	"easy-stock/backend/internal/stockanalysis"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type stockMethodSession struct {
	session            *agent.ToolSession
	mu                 sync.Mutex
	loaded, read, done bool
	ref                skillregistry.Reference
	calls              []agent.ToolCallRecord
}

func (s *Server) stockMethod(ctx context.Context, snapshot *stockanalysis.ResearchSnapshot, publish stockanalysis.ResearchPublisher) (*stockMethodSession, error) {
	service, ok := s.agentGateway.(*agent.Service)
	if !ok {
		return nil, nil
	}
	if agent.BoundRuntime(ctx) == agent.WorkBuddy {
		return nil, errors.New("当前 WorkBuddy 尚不支持应用证据 MCP，请选择已验收的 Hermes 或 Codex 后开始研究")
	}
	j := dr.Job{}
	for _, ref := range snapshot.SkillRefs {
		j.SkillCatalog = append(j.SkillCatalog, ref)
	}
	bundles, e := s.taskSkillBundles(ctx, service, &j)
	if e != nil {
		return nil, e
	}
	var method skillregistry.Bundle
	for _, b := range bundles {
		if b.Name == "a-share-stock-analysis" {
			method = b
			break
		}
	}
	if method.Hash == "" {
		return nil, errors.New("请启用 a-share-stock-analysis Skill")
	}
	bridge := &stockMethodSession{ref: skillregistry.Reference{Name: method.Name, Hash: method.Hash}}
	definitions := []agent.ToolDefinition{{Name: "load_skill", Description: "加载本任务固定版本的个股研究方法", Schema: schema(map[string]any{})}, {Name: "get_evidence_catalog", Description: "取得固定证券的量化事实、原文目录和数据缺口，不联网补数", Schema: schema(map[string]any{})}, {Name: "read_evidence_source", Description: "按目录 ID 读取本轮原始证据，保留披露日期和来源", Schema: schema(map[string]any{"source_id": textProperty("固定目录的证据 ID")}, "source_id")}}
	session, e := agent.NewToolSession(ctx, definitions, 12, func(ctx context.Context, name string, args json.RawMessage) (agent.ToolResult, error) {
		bridge.mu.Lock()
		defer bridge.mu.Unlock()
		if name == "load_skill" {
			bridge.loaded = true
			bridge.ref.LoadedAt = time.Now().UTC()
			return agent.ToolResult{Data: map[string]any{"name": method.Name, "hash": method.Hash, "instructions": string(method.Files["SKILL.md"])}}, nil
		}
		if !bridge.loaded {
			return agent.ToolResult{}, errors.New("请先加载固定 Skill")
		}
		if name == "get_evidence_catalog" {
			catalog := []map[string]any{}
			for _, src := range snapshot.Sources {
				catalog = append(catalog, map[string]any{"id": src.ID, "title": src.Title, "kind": src.Kind, "published_at": src.PublishedAt, "report_date": src.ReportDate, "time_status": src.TimeStatus})
			}
			bridge.read = true
			return agent.ToolResult{EvidenceID: snapshot.EvidenceID, Data: map[string]any{"symbol": snapshot.Symbol, "quote": snapshot.Quote, "sources": catalog, "gaps": snapshot.Limitations, "cutoff": snapshot.CutoffAt}}, nil
		}
		var in struct {
			SourceID string `json:"source_id"`
		}
		if json.Unmarshal(args, &in) != nil {
			return agent.ToolResult{}, errors.New("无效证据 ID")
		}
		for _, src := range snapshot.Sources {
			if src.ID == in.SourceID {
				return agent.ToolResult{EvidenceID: snapshot.EvidenceID, Data: src}, nil
			}
		}
		return agent.ToolResult{}, errors.New("证据不属于本轮快照")
	}, func(calls []agent.ToolCallRecord) error {
		bridge.mu.Lock()
		bridge.calls = append([]agent.ToolCallRecord(nil), calls...)
		copy := *snapshot
		copy.ToolCalls = append(append([]agent.ToolCallRecord{}, snapshot.ToolCalls...), calls...)
		if bridge.loaded {
			copy.SkillRefs = []skillregistry.Reference{bridge.ref}
		}
		bridge.mu.Unlock()
		return publish("method", "已记录 Skill 与固定证据工具调用", nil, &copy)
	})
	if e != nil {
		return nil, e
	}
	bridge.session = session
	return bridge, nil
}
func (b *stockMethodSession) needed() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.done }
func (b *stockMethodSession) complete() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.loaded || !b.read {
		return errors.New("模型未实际加载 Skill 并读取本轮证据目录，本次不发布 AI 结论")
	}
	b.done = true
	return nil
}

// Copies avoid mutating the snapshot while prompt progress is being persisted.
func (b *stockMethodSession) attach(v *stockanalysis.ResearchSnapshot) *stockanalysis.ResearchSnapshot {
	if v == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	copy := *v
	// The callback may already have attached the same trace.
	if len(b.calls) > 0 && (len(copy.ToolCalls) == 0 || copy.ToolCalls[len(copy.ToolCalls)-1].StartedAt != b.calls[len(b.calls)-1].StartedAt) {
		copy.ToolCalls = append(append([]agent.ToolCallRecord{}, v.ToolCalls...), b.calls...)
	}
	if b.loaded {
		copy.SkillRefs = []skillregistry.Reference{b.ref}
	}
	return &copy
}
