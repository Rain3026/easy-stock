// Package dailyresearch stores observations separately from later judgements.
package dailyresearch

import (
	"easy-stock/backend/internal/agent"
	"easy-stock/backend/internal/foundation"
	"easy-stock/backend/internal/marketemotion"
	"easy-stock/backend/internal/marketpulse"
	"easy-stock/backend/internal/skillregistry"
	"encoding/json"
	"time"
)

const Version = "daily-research-v1"

type Theme struct {
	Overview   foundation.ThemeOverview `json:"overview"`
	Members    []foundation.BoardStock  `json:"members"`
	Candidates []foundation.BoardStock  `json:"candidates"`
	Meta       foundation.SourceMeta    `json:"meta"`
	Error      string                   `json:"error,omitempty"`
}
type Snapshot struct {
	EntityID           string                           `json:"entity_id,omitempty"`
	ParentID           string                           `json:"parent_id,omitempty"`
	ContextLabel       string                           `json:"context_label,omitempty"`
	ContextData        json.RawMessage                  `json:"context_data,omitempty"`
	ContextEvidenceIDs []string                         `json:"context_evidence_ids,omitempty"`
	Judgement          *marketpulse.Judgement           `json:"judgement,omitempty"`
	EvidenceID         string                           `json:"evidence_id,omitempty"`
	MarketPulse        *marketpulse.Snapshot            `json:"market_pulse,omitempty"`
	ID                 string                           `json:"id"`
	Version            string                           `json:"version"`
	TradeDate          string                           `json:"trade_date"`
	StartedAt          time.Time                        `json:"started_at"`
	Cutoff             time.Time                        `json:"cutoff"`
	Status             string                           `json:"status"`
	Indexes            []foundation.MarketIndexSnapshot `json:"indexes"`
	IndexMeta          foundation.SourceMeta            `json:"index_meta"`
	Emotion            *marketemotion.Snapshot          `json:"emotion,omitempty"`
	Themes             []Theme                          `json:"themes"`
	Baselines          map[string]foundation.KLine      `json:"baselines"`
	Gaps               []string                         `json:"gaps"`
}
type Role struct {
	Runtime   string `json:"runtime"`
	ProfileID string `json:"profile_id"`
}
type Settings struct {
	Bull           Role `json:"bull"`
	Bear           Role `json:"bear"`
	Reviewer       Role `json:"reviewer"`
	Rounds         int  `json:"rounds"`
	TimeoutSeconds int  `json:"timeout_seconds"`
	MaxCalls       int  `json:"max_calls"`
	KeepResponses  bool `json:"keep_responses"`
}

func DefaultSettings() Settings {
	return Settings{Rounds: 1, TimeoutSeconds: 180, MaxCalls: 5, KeepResponses: true}
}

type Argument struct {
	Thesis      string   `json:"thesis"`
	Evidence    []string `json:"evidence"`
	Risks       []string `json:"risks"`
	Conditions  []string `json:"conditions"`
	Uncertainty string   `json:"uncertainty"`
}
type Response struct {
	SkillRefs     []skillregistry.Reference `json:"skill_refs,omitempty"`
	Role          string                    `json:"role"`
	Stage         string                    `json:"stage"`
	Runtime       string                    `json:"runtime"`
	Model         string                    `json:"model"`
	ModelIdentity string                    `json:"model_identity,omitempty"`
	SessionID     string                    `json:"session_id,omitempty"`
	Argument      Argument                  `json:"argument"`
	Raw           string                    `json:"raw,omitempty"`
}
type Debate struct {
	SnapshotID string     `json:"snapshot_id"`
	Subject    string     `json:"subject"`
	Settings   Settings   `json:"settings"`
	Responses  []Response `json:"responses"`
	Calls      int        `json:"calls"`
}
type Outcome struct {
	Symbol        string            `json:"symbol"`
	Name          string            `json:"name"`
	Days          int               `json:"days"`
	DueDate       string            `json:"due_date"`
	Status        string            `json:"status"`
	Reason        string            `json:"reason,omitempty"`
	ChangePercent *float64          `json:"change_percent,omitempty"`
	Baseline      *foundation.KLine `json:"baseline,omitempty"`
	Observation   *foundation.KLine `json:"observation,omitempty"`
}
type Verification struct {
	SnapshotID string    `json:"snapshot_id"`
	CheckedAt  time.Time `json:"checked_at"`
	Outcomes   []Outcome `json:"outcomes"`
	Note       string    `json:"note"`
}

type Job struct {
	Fingerprint  string                    `json:"fingerprint,omitempty"`
	Request      json.RawMessage           `json:"request,omitempty"`
	PreviousID   string                    `json:"previous_id,omitempty"`
	SkillCatalog []skillregistry.Reference `json:"skill_catalog,omitempty"`
	ToolCalls    []agent.ToolCallRecord    `json:"tool_calls,omitempty"`
	SkillRefs    []skillregistry.Reference `json:"skill_refs,omitempty"`
	EvidenceIDs  []string                  `json:"evidence_ids,omitempty"`
	Summary      string                    `json:"summary,omitempty"`
	Runtime      string                    `json:"runtime,omitempty"`
	Model        string                    `json:"model,omitempty"`

	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	SnapshotID   string        `json:"snapshot_id,omitempty"`
	Status       string        `json:"status"`
	Stage        string        `json:"stage"`
	Error        string        `json:"error,omitempty"`
	StartedAt    time.Time     `json:"started_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Snapshot     *Snapshot     `json:"snapshot,omitempty"`
	Debate       *Debate       `json:"debate,omitempty"`
	Verification *Verification `json:"verification,omitempty"`
}
