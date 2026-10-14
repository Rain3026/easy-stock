package agent

import (
	"context"
	"easy-stock/backend/internal/appsettings"
	"testing"
)

func TestResearchRolesDoNotMutateGlobalOrEachOther(t *testing.T) {
	s := testService(t)
	key := "reviewer-key"
	if err := s.StoreLLMProfileKey("reviewer", &key); err != nil {
		t.Fatal(err)
	}
	base, release, err := s.BindTask(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	profile := appsettings.LLMProfile{ID: "reviewer", Provider: "custom", Model: "reviewer-model", BaseURL: "https://example.com/v1", APIMode: "responses"}
	role, cleanup, err := s.BindResearchRole(base, Codex, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if BoundRuntime(role) != Codex || BoundModel(role) != "reviewer-model" {
		t.Fatal("role not mapped")
	}
	if BoundRuntime(base) != Hermes || BoundModel(base) != "old" || s.ActiveRuntime() != Hermes {
		t.Fatal("global/nested task modified")
	}
	if got, _ := s.ModelAPIKey(); got != "old-key" {
		t.Fatal("global key changed")
	}
	binding := role.Value(bindingKey{}).(*taskBinding)
	if got, _ := binding.hermes.ModelAPIKey(); got != key {
		t.Fatal("role secret not selected")
	}
	if binding.hermes.home == s.home || binding.hermes.home == base.Value(bindingKey{}).(*taskBinding).hermes.home {
		t.Fatal("role shares mutable config")
	}
}
