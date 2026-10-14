package skillregistry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Feedback and candidates are append-only. A holdout entity is never eligible
// as a candidate's training example, even when a different job covers it.
type Feedback struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Hash       string    `json:"hash"`
	JobID      string    `json:"job_id"`
	EvidenceID string    `json:"evidence_id"`
	EntityID   string    `json:"entity_id"`
	Category   string    `json:"category"`
	Note       string    `json:"note"`
	Split      string    `json:"split"`
	Cutoff     time.Time `json:"cutoff"`
	CreatedAt  time.Time `json:"created_at"`
}
type Candidate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	BaseHash    string    `json:"base_hash"`
	Hash        string    `json:"hash"`
	Reason      string    `json:"reason"`
	FeedbackIDs []string  `json:"feedback_ids"`
	TrainCutoff time.Time `json:"train_cutoff"`
	CreatedAt   time.Time `json:"created_at"`
}
type Metrics struct {
	Structure  bool   `json:"structure"`
	Citations  bool   `json:"citations"`
	Missing    bool   `json:"missing"`
	Counter    bool   `json:"counter"`
	DurationMS int64  `json:"duration_ms"`
	Tokens     int    `json:"tokens"`
	Error      string `json:"error,omitempty"`
}
type EvaluationCase struct {
	FeedbackID      string          `json:"feedback_id"`
	EvidenceID      string          `json:"evidence_id"`
	EntityID        string          `json:"entity_id"`
	Cutoff          time.Time       `json:"cutoff"`
	Base            Metrics         `json:"base"`
	Candidate       Metrics         `json:"candidate"`
	BaseOutput      json.RawMessage `json:"base_output,omitempty"`
	CandidateOutput json.RawMessage `json:"candidate_output,omitempty"`
}
type Evaluation struct {
	ID          string           `json:"id"`
	CandidateID string           `json:"candidate_id"`
	JobID       string           `json:"job_id"`
	Policy      string           `json:"policy"`
	Cases       []EvaluationCase `json:"cases"`
	Passed      bool             `json:"passed"`
	Reasons     []string         `json:"reasons"`
	CreatedAt   time.Time        `json:"created_at"`
}
type Release struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Hash         string    `json:"hash"`
	Previous     string    `json:"previous"`
	EvaluationID string    `json:"evaluation_id,omitempty"`
	Kind         string    `json:"kind"`
	CreatedAt    time.Time `json:"created_at"`
}

func uid() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func (s *Store) event(ctx context.Context, id, kind, name string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO skill_events VALUES(?,?,?,?,?)`, id, kind, name, time.Now().UTC().Format(time.RFC3339Nano), raw)
	return e
}
func (s *Store) Events(ctx context.Context, kind string) ([]json.RawMessage, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT payload FROM skill_events WHERE kind=? ORDER BY created_at DESC LIMIT 500`, kind)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Store) ReadEvent(ctx context.Context, id, kind string, out any) error {
	var raw []byte
	e := s.db.QueryRowContext(ctx, `SELECT payload FROM skill_events WHERE id=? AND kind=?`, id, kind).Scan(&raw)
	if e != nil {
		return e
	}
	return json.Unmarshal(raw, out)
}
func EntitySplit(entity string) string {
	v := sha256.Sum256([]byte(entity))
	if v[0]%3 == 0 {
		return "holdout"
	}
	return "training"
}
func (s *Store) AddFeedback(ctx context.Context, f Feedback) (Feedback, error) {
	if _, e := s.Get(ctx, f.Name, f.Hash); e != nil {
		return f, e
	}
	if f.JobID == "" || f.EvidenceID == "" || f.EntityID == "" || f.Cutoff.IsZero() || strings.TrimSpace(f.Note) == "" || len(f.Note) > 6000 {
		return f, errors.New("反馈缺少原始任务、证据或说明")
	}
	if f.Category != "data" && f.Category != "calculation" && f.Category != "method" && f.Category != "model" {
		return f, errors.New("错误归因须为数据、计算、方法或模型")
	}
	f.ID = uid()
	f.CreatedAt = time.Now().UTC()
	f.Split = EntitySplit(f.EntityID)
	return f, s.event(ctx, f.ID, "feedback", f.Name, f)
}
func (s *Store) CreateCandidate(ctx context.Context, name, baseHash, text, reason string, feedbackIDs []string) (Candidate, error) {
	c := Candidate{ID: uid(), Name: name, BaseHash: baseHash, Reason: strings.TrimSpace(reason), FeedbackIDs: feedbackIDs, CreatedAt: time.Now().UTC()}
	if len(text) < 30 || len(text) > 128<<10 || len(c.Reason) < 5 || len(feedbackIDs) == 0 || len(feedbackIDs) > 30 {
		return c, errors.New("候选须有方法全文、修改理由和训练反馈")
	}
	b, e := s.Get(ctx, name, baseHash)
	if e != nil {
		return c, e
	}
	for _, id := range feedbackIDs {
		var f Feedback
		if e = s.ReadEvent(ctx, id, "feedback", &f); e != nil {
			return c, e
		}
		if f.Name != name || f.Hash != baseHash || f.Split != "training" || (f.Category!="method"&&f.Category!="model") {
			return c, errors.New("只能使用本版本的训练反馈，留出样本不得参与改写")
		}
		if f.Cutoff.After(c.TrainCutoff) {
			c.TrainCutoff = f.Cutoff
		}
	}
	original := string(b.Files["SKILL.md"])
	if strings.HasPrefix(original, "---\n") {
		if end := strings.Index(original[4:], "\n---"); end >= 0 {
			header := original[:4+end+4]
			if !strings.HasPrefix(text, header) {
				return c, errors.New("候选不可更改 Skill 元信息；本流程只改研究方法正文")
			}
		}
	}
	b.Files["SKILL.md"] = []byte(text)
	b, e = s.Register(ctx, b)
	if e != nil {
		return c, e
	}
	if b.Hash == baseHash {
		return c, errors.New("候选与原版本相同")
	}
	c.Hash = b.Hash
	return c, s.event(ctx, c.ID, "candidate", name, c)
}

// This is a contract-regression gate, not a return prediction score. A release
// still requires the user's explicit action in settings.
func Assess(c Candidate, cases []EvaluationCase) (bool, []string) {
	reasons := []string{}
	entities := map[string]bool{}
	evidence := map[string]bool{}
	if len(cases) < 5 {
		reasons = append(reasons, "至少需要 5 个不同对象的留出样本")
	}
	for _, v := range cases {
		if EntitySplit(v.EntityID) != "holdout" || !v.Cutoff.After(c.TrainCutoff) || v.Cutoff.After(c.CreatedAt) || entities[v.EntityID] || evidence[v.EvidenceID] {
			reasons = append(reasons, "留出对象重复、时间切分不合规或存在未来数据")
		}
		entities[v.EntityID] = true
		evidence[v.EvidenceID] = true
		a, b := v.Base, v.Candidate
		if b.Error != "" || !b.Structure || !b.Citations || !b.Missing || !b.Counter {
			reasons = append(reasons, "候选的结构、引用、缺失说明或反证未通过")
		}
		if a.Error != "" || a.Tokens <= 0 || b.Tokens <= 0 {
			reasons = append(reasons, "基线失败或真实用量缺失，不能作发布比较")
		}
		if b.DurationMS > a.DurationMS*3/2+1000 || b.Tokens > a.Tokens*3/2+500 {
			reasons = append(reasons, "延迟或 Token 超出基线 1.5 倍容差")
		}
	}
	return len(reasons) == 0, reasons
}
func (s *Store) SaveEvaluation(ctx context.Context, c Candidate, e Evaluation) (Evaluation, error) {
	e.ID = uid()
	e.CandidateID = c.ID
	e.Policy = "skill-contract-holdout-v1"
	e.CreatedAt = time.Now().UTC()
	e.Passed, e.Reasons = Assess(c, e.Cases)
	return e, s.event(ctx, e.ID, "evaluation", c.Name, e)
}
func (s *Store) Active(ctx context.Context, name string) (string, error) {
	var hash string
	e := s.db.QueryRowContext(ctx, `SELECT hash FROM skill_active WHERE name=?`, name).Scan(&hash)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil
	}
	return hash, e
}
func (s *Store) Resolve(ctx context.Context, b Bundle) (Bundle, error) {
	v, e := s.Register(ctx, b)
	if e != nil {
		return v, e
	}
	hash, e := s.Active(ctx, b.Name)
	if e != nil || hash == "" {
		return v, e
	}
	return s.Get(ctx, b.Name, hash)
}
func (s *Store) Release(ctx context.Context, candidateID, evaluationID, expected string) (Release, error) {
	var c Candidate
	var e Evaluation
	if err := s.ReadEvent(ctx, candidateID, "candidate", &c); err != nil {
		return Release{}, err
	}
	if err := s.ReadEvent(ctx, evaluationID, "evaluation", &e); err != nil {
		return Release{}, err
	}
	if ok, _ := Assess(c, e.Cases); !ok || !e.Passed || e.CandidateID != c.ID {
		return Release{}, errors.New("候选未通过留出评估，不能发布")
	}
	if expected == "" {
		expected = c.BaseHash
	}
	return s.activate(ctx, c.Name, c.Hash, expected, evaluationID, "publish", c.BaseHash)
}
func (s *Store) Rollback(ctx context.Context, releaseID, expected string) (Release, error) {
	var prior Release
	if err := s.ReadEvent(ctx, releaseID, "release", &prior); err != nil {
		return Release{}, err
	}
	if prior.Previous == "" {
		return Release{}, errors.New("没有可回退版本")
	}
	return s.activate(ctx, prior.Name, prior.Previous, expected, "", "rollback", "")
}
func (s *Store) activate(ctx context.Context, name, hash, expected, evaluation, kind, baseline string) (Release, error) {
	v := Release{ID: uid(), Name: name, Hash: hash, Previous: expected, EvaluationID: evaluation, Kind: kind, CreatedAt: time.Now().UTC()}
	if _, e := s.Get(ctx, name, hash); e != nil {
		return v, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return v, e
	}
	defer tx.Rollback()
	var current string
	e = tx.QueryRowContext(ctx, `SELECT hash FROM skill_active WHERE name=?`, name).Scan(&current)
	if errors.Is(e, sql.ErrNoRows) {
		current = baseline
		e = nil
	}
	if e != nil {
		return v, e
	}
	if current != expected {
		return v, fmt.Errorf("活动版本已变化，请刷新后比较")
	}
	if current == hash {
		return v, errors.New("该版本已经生效")
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO skill_active VALUES(?,?) ON CONFLICT(name) DO UPDATE SET hash=excluded.hash`, name, hash); e != nil {
		return v, e
	}
	raw, _ := json.Marshal(v)
	if _, e = tx.ExecContext(ctx, `INSERT INTO skill_events VALUES(?,?,?,?,?)`, v.ID, "release", name, v.CreatedAt.Format(time.RFC3339Nano), raw); e != nil {
		return v, e
	}
	return v, tx.Commit()
}
