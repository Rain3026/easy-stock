package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTaskToolAuthorizationBudgetAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var count atomic.Int32
	s, err := NewToolSession(ctx, []ToolDefinition{{Name: "quote"}}, 2, func(context.Context, string, json.RawMessage) (ToolResult, error) {
		count.Add(1)
		return ToolResult{}, errors.New("secret-provider-token")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	call := func(auth, body string) int {
		req, _ := http.NewRequest("POST", s.Endpoint.URL+"/call", strings.NewReader(body))
		req.Header.Set("Authorization", auth)
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Error(e)
			return 0
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if strings.Contains(string(raw), "secret-provider-token") {
			t.Error("provider error leaked")
		}
		return res.StatusCode
	}
	auth := "Bearer " + s.Endpoint.Token
	if got := call("", `{"name":"quote","arguments":{}}`); got != 401 {
		t.Fatal(got)
	}
	for _, body := range []string{`{"name":"terminal","arguments":{}}`, `{"name":"quote","arguments":{}} {}`} {
		if got := call(auth, body); got != 400 {
			t.Fatal(got)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); call(auth, `{"name":"quote","arguments":{}}`) }()
	}
	wg.Wait()
	if count.Load() != 2 || len(s.Calls()) != 2 {
		t.Fatal("budget not enforced", count.Load())
	}
	for _, v := range s.Calls() {
		if v.Status != "failed" || v.Error == "" {
			t.Fatal(v)
		}
	}
	cancel()
	if got := call(auth, `{"name":"quote","arguments":{}}`); got != 410 {
		t.Fatal("ended task accepted", got)
	}
}
