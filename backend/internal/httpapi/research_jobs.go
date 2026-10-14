package httpapi

import (
	"context"
	"crypto/sha256"
	"easy-stock/backend/internal/agent"
	dr "easy-stock/backend/internal/dailyresearch"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

type researchJobOptions struct {
	Fingerprint    string
	Request        json.RawMessage
	ReuseCompleted bool
	Previous       *dr.Job
}
type debateRequest struct {
	Subject  string      `json:"subject"`
	Settings dr.Settings `json:"settings"`
}

func fingerprint(values ...any) string {
	raw, _ := json.Marshal(values)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (s *Server) methodFingerprint(ctx context.Context) string {
	if service, ok := s.agentGateway.(*agent.Service); ok {
		bound, cleanup, e := service.BindTask(ctx)
		if e != nil {
			return "unavailable"
		}
		defer cleanup()
		bundles, e := service.SnapshotSkills(bound)
		if e != nil {
			return "unavailable"
		}
		return fingerprint(bundles)
	}
	return "test-gateway"
}
func (s *Server) debateJobOptions(ctx context.Context, v dr.Snapshot, subject string, cfg dr.Settings) researchJobOptions {
	raw, _ := json.Marshal(debateRequest{subject, cfg})
	return researchJobOptions{Request: raw, Fingerprint: fingerprint("debate", v.ID, raw, s.methodFingerprint(ctx), s.settingsStore.Snapshot()), ReuseCompleted: true}
}
func (s *Server) dailyResearchRetry(w http.ResponseWriter, r *http.Request) {
	old, e := s.dailyResearch.store.Job(r.Context(), r.PathValue("id"))
	if e != nil {
		writeError(w, 404, "任务不存在")
		return
	}
	if old.Status == "running" || old.Status == "succeeded" {
		writeError(w, 409, "只可重试未完成任务")
		return
	}
	opt := researchJobOptions{Request: old.Request, Previous: &old, Fingerprint: fingerprint("retry", old.ID)}
	switch old.Kind {
	case "debate":
		var req debateRequest
		if json.Unmarshal(old.Request, &req) != nil || req.Subject == "" {
			if old.Debate == nil {
				writeError(w, 400, "旧任务缺少可恢复参数")
				return
			}
			req = debateRequest{old.Debate.Subject, old.Debate.Settings}
		}
		v, e := s.dailyResearch.store.Snapshot(r.Context(), old.SnapshotID)
		if e != nil {
			writeError(w, 404, "原证据不存在")
			return
		}
		if e = s.validateResearchSettings(req.Settings); e != nil {
			writeError(w, 400, e.Error())
			return
		}
		s.startDailyResearch(w, old.Kind, old.SnapshotID, func(ctx context.Context, j *dr.Job) error {
			return s.runResearchDebate(ctx, j, v, req.Subject, req.Settings)
		}, opt)
	case "agent-evidence":
		var req evidenceRequest
		if json.Unmarshal(old.Request, &req) != nil || req.EntityID == "" {
			writeError(w, 400, "旧任务缺少参数，请新建取证")
			return
		}
		if _, ok := s.agentGateway.(*agent.Service); !ok {
			writeError(w, 503, "取证运行时不可用")
			return
		}
		s.startDailyResearch(w, old.Kind, old.SnapshotID, func(ctx context.Context, j *dr.Job) error { return s.runAgentEvidence(ctx, j, req) }, opt)
	case "verification":
		v, e := s.dailyResearch.store.Snapshot(r.Context(), old.SnapshotID)
		if e != nil {
			writeError(w, 404, "原证据不存在")
			return
		}
		s.startDailyResearch(w, old.Kind, old.SnapshotID, func(ctx context.Context, j *dr.Job) error {
			v2 := dr.Verify(ctx, v, time.Now(), s.dailyResearchLines)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			j.Verification = &v2
			return s.dailyResearch.store.Append(ctx, j.ID, "verification", v.ID, v2)
		}, opt)
	default:
		writeError(w, 400, "请重新生成事实快照，原快照保持不变")
	}
}
