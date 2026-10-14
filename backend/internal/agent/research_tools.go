package agent

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"inputSchema"`
}
type ToolResult struct {
	Data       any    `json:"data"`
	EvidenceID string `json:"evidence_id,omitempty"`
}
type ToolCallRecord struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
	StartedAt  time.Time       `json:"started_at"`
	DurationMS int64           `json:"duration_ms"`
	Status     string          `json:"status"`
	EvidenceID string          `json:"evidence_id,omitempty"`
	Error      string          `json:"error,omitempty"`
}
type ToolEndpoint struct {
	URL   string `json:"-"`
	Token string `json:"-"`
}
type ToolSession struct {
	Endpoint *ToolEndpoint
	server   *http.Server
	mu       sync.Mutex
	calls    []ToolCallRecord
	count    int
	cancel   context.CancelFunc
}
type ResearchToolHandler func(context.Context, string, json.RawMessage) (ToolResult, error)

func NewToolSession(parent context.Context, defs []ToolDefinition, maxCalls int, handler ResearchToolHandler, checkpoints ...func([]ToolCallRecord) error) (*ToolSession, error) {
	ctx, cancel := context.WithCancel(parent)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		cancel()
		return nil, e
	}
	key := make([]byte, 24)
	if _, e = rand.Read(key); e != nil {
		ln.Close()
		cancel()
		return nil, e
	}
	s := &ToolSession{Endpoint: &ToolEndpoint{URL: "http://" + ln.Addr().String(), Token: hex.EncodeToString(key)}, calls: []ToolCallRecord{}, cancel: cancel}
	allowed := map[string]bool{}
	for _, d := range defs {
		allowed[d.Name] = true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tools", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(defs) })
	mux.HandleFunc("POST /call", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF || !allowed[in.Name] {
			http.Error(w, "invalid task tool", 400)
			return
		}
		s.mu.Lock()
		if s.count >= maxCalls {
			s.mu.Unlock()
			http.Error(w, "task tool budget exceeded", 429)
			return
		}
		s.count++
		seq := s.count
		s.mu.Unlock()
		c, stop := context.WithTimeout(ctx, 40*time.Second)
		defer stop()
		started := time.Now().UTC()
		result, e := handler(c, in.Name, in.Arguments)
		rec := ToolCallRecord{ID: fmt.Sprintf("%s-%d", started.Format("20060102T150405.000000000"), seq), Name: in.Name, Arguments: in.Arguments, StartedAt: started, DurationMS: time.Since(started).Milliseconds(), Status: "succeeded", EvidenceID: result.EvidenceID}
		if e != nil {
			rec.Status = "failed"
			rec.Error = "工具未完成，请检查来源时效、权限或输入范围"
		}
		s.mu.Lock()
		s.calls = append(s.calls, rec)
		for _, checkpoint := range checkpoints {
			if checkpoint(append([]ToolCallRecord{}, s.calls...)) != nil {
				e = errors.New("checkpoint failed")
				rec.Error = "工具记录保存失败，本次结果不进入分析"
			}
		}
		s.mu.Unlock()
		if e != nil {
			json.NewEncoder(w).Encode(map[string]any{"isError": true, "error": rec.Error})
			return
		}
		json.NewEncoder(w).Encode(result)
	})
	s.server = &http.Server{ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Endpoint.Token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if ctx.Err() != nil {
			http.Error(w, "task ended", 410)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		mux.ServeHTTP(w, r)
	})}
	go s.server.Serve(ln)
	return s, nil
}
func (s *ToolSession) Close() {
	s.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}
func (s *ToolSession) Calls() []ToolCallRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ToolCallRecord{}, s.calls...)
}
func (e *ToolEndpoint) spec() (string, error) {
	if e == nil || e.Token == "" {
		return "", errors.New("missing task tools")
	}
	b, err := json.Marshal(map[string]any{"transport": "app_data", "url": e.URL, "token": e.Token})
	return string(b), err
}
