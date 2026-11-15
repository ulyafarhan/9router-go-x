package chat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlers/shared"
	"9router/proxy/internal/middleware"
	"9router/proxy/internal/models"
)

// openPersonaRepo opens a database with the production schema and returns a
// repository over it (same shape as the app wiring test's helper).
func openPersonaRepo(t *testing.T) *db.Repo {
	t.Helper()
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "persona.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.EnsureCoreSchema(conn); err != nil {
		t.Fatalf("ensure core schema: %v", err)
	}
	return db.NewRepo(conn)
}

// The dashboard's active persona applies when no explicit header is present,
// and an explicit header wins over it.
func TestApplyTokenSaversPersonaFromSettings(t *testing.T) {
	repo := openPersonaRepo(t)
	if err := repo.UpdateSettingsRaw(map[string]any{"personaEnabled": true, "personaActive": "novelist"}); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	h := &ChatHandler{Repo: repo, TokenSaver: shared.NewTokenSaverConfig(false, false, false)}
	body, _ := json.Marshal(map[string]any{
		"model": "gpt-test",
		"messages": []map[string]any{
			{"role": "system", "content": "base instructions"},
			{"role": "user", "content": "hello"},
		},
	})

	out, _, _, _, _, _ := h.applyTokenSavers(context.Background(), body, false)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	sys := m["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "seasoned novelist") {
		t.Fatalf("dashboard-active persona not injected: %q", sys)
	}

	// An explicit header overrides the dashboard selection.
	out2, _, _, _, _, _ := h.applyTokenSavers(WithPersona(context.Background(), "coding"), body, false)
	var m2 map[string]any
	_ = json.Unmarshal(out2, &m2)
	sys2 := m2["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys2, "senior software engineer") || strings.Contains(sys2, "seasoned novelist") {
		t.Fatalf("header must override the dashboard persona: %q", sys2)
	}

	// With the dashboard switch off, no persona without a header.
	_ = repo.UpdateSettingsRaw(map[string]any{"personaEnabled": false})
	out3, _, _, _, _, _ := h.applyTokenSavers(context.Background(), body, false)
	var m3 map[string]any
	_ = json.Unmarshal(out3, &m3)
	sys3 := m3["messages"].([]any)[0].(map[string]any)["content"].(string)
	if sys3 != "base instructions" {
		t.Fatalf("disabled persona must not inject: %q", sys3)
	}
}

func TestPersonaContextRoundTrip(t *testing.T) {
	if got := PersonaFromContext(context.Background()); got != "" {
		t.Fatalf("empty ctx PersonaFromContext = %q, want \"\"", got)
	}
	ctx := WithPersona(context.Background(), "coding")
	if got := PersonaFromContext(ctx); got != "coding" {
		t.Fatalf("PersonaFromContext = %q, want coding", got)
	}
}

func TestPersonaFromHeader(t *testing.T) {
	headers := map[string]string{}
	get := func(name string) string { return headers[name] }

	if id := PersonaFromHeader(get); id != "" {
		t.Fatalf("no headers PersonaFromHeader = %q, want \"\"", id)
	}
	headers["X-Persona"] = "novelist"
	if id := PersonaFromHeader(get); id != "novelist" {
		t.Fatalf("alias header = %q, want novelist", id)
	}
	headers["X-9Router-Persona"] = "coding"
	if id := PersonaFromHeader(get); id != "coding" {
		t.Fatalf("canonical header must win, got %q", id)
	}
	headers["X-Persona"] = "   "
	if id := PersonaFromHeader(get); id != "coding" {
		t.Fatalf("whitespace-only alias must fall through, got %q", id)
	}
}

// applyTokenSavers with a persona must inject the pack around the system
// prompt: the prefix lands ahead of the existing system text and the suffix
// after it, and an unknown id changes nothing.
func TestApplyTokenSaversPersonaInjection(t *testing.T) {
	h := &ChatHandler{TokenSaver: shared.NewTokenSaverConfig(false, false, false)}
	body, _ := json.Marshal(map[string]any{
		"model": "gpt-test",
		"messages": []map[string]any{
			{"role": "system", "content": "base instructions"},
			{"role": "user", "content": "hello"},
		},
	})

	out, _, _, _, _, _ := h.applyTokenSavers(WithPersona(context.Background(), "coding"), body, false)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	sys := m["messages"].([]any)[0].(map[string]any)["content"].(string)
	first := strings.Index(sys, "senior software engineer")
	base := strings.Index(sys, "base instructions")
	last := strings.Index(sys, "meta commentary stays to one line")
	if first == -1 || base == -1 || last == -1 {
		t.Fatalf("persona not wrapped around system prompt: %q", sys)
	}
	if !(first < base && base < last) {
		t.Fatalf("persona prefix/suffix order wrong: prefix=%d base=%d suffix=%d", first, base, last)
	}

	// The novelist pack must not leak into a coding-tagged request.
	if strings.Contains(sys, "seasoned novelist") {
		t.Fatalf("wrong persona injected: %q", sys)
	}

	// An unknown id resolves to no persona: the body stays untouched.
	plain, _, _, _, _, _ := h.applyTokenSavers(WithPersona(context.Background(), "no-such"), body, false)
	var pm map[string]any
	_ = json.Unmarshal(plain, &pm)
	psys := pm["messages"].([]any)[0].(map[string]any)["content"].(string)
	if psys != "base instructions" {
		t.Fatalf("unknown persona id modified the body: %q", psys)
	}
}

// The auto-tuner's own signal: a tool-bearing request gets the coding pack
// without any header or dashboard configuration; everything else stays plain.
func TestAutoDetectPersona(t *testing.T) {
	withTools, _ := json.Marshal(map[string]any{
		"model": "x",
		"tools": []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file"}}},
	})
	if got := autoDetectPersona(withTools); got != "coding" {
		t.Fatalf("tool-bearing request = %q, want coding", got)
	}
	plain, _ := json.Marshal(map[string]any{"model": "x", "messages": []any{}})
	if got := autoDetectPersona(plain); got != "" {
		t.Fatalf("plain request = %q, want empty", got)
	}
	emptyTools, _ := json.Marshal(map[string]any{"model": "x", "tools": []any{}})
	if got := autoDetectPersona(emptyTools); got != "" {
		t.Fatalf("empty tools = %q, want empty", got)
	}
	if got := autoDetectPersona([]byte("not json")); got != "" {
		t.Fatalf("garbage body = %q, want empty", got)
	}
}

// Header beats dashboard beats auto (opt-in only): tool-bearing requests stay
// untouched unless personaAutoDetect is explicitly enabled.
func TestPersonaResolutionOrderAuto(t *testing.T) {
	repo := openPersonaRepo(t)
	h := &ChatHandler{Repo: repo, TokenSaver: shared.NewTokenSaverConfig(false, false, false)}
	toolBody, _ := json.Marshal(map[string]any{
		"model": "x",
		"tools": []map[string]any{{"type": "function", "function": map[string]any{"name": "f"}}},
		"messages": []map[string]any{
			{"role": "system", "content": "base"},
			{"role": "user", "content": "hi"},
		},
	})

	// Default: personaAutoDetect is false -> untouched.
	outPlain, _, _, _, pidPlain, srcPlain := h.applyTokenSavers(context.Background(), toolBody, false)
	if pidPlain != "" || srcPlain != "" {
		t.Fatalf("default auto-detect must be off, got pid=%q src=%q", pidPlain, srcPlain)
	}
	var mPlain map[string]any
	_ = json.Unmarshal(outPlain, &mPlain)
	if sys := mPlain["messages"].([]any)[0].(map[string]any)["content"].(string); sys != "base" {
		t.Fatalf("tool request mutated with auto-detect disabled: %q", sys)
	}

	// Opt-in: personaAutoDetect enabled applies coding when no dashboard/header set.
	_ = repo.UpdateSettingsRaw(map[string]any{"personaAutoDetect": true})
	outAuto, _, _, _, pidAuto, srcAuto := h.applyTokenSavers(context.Background(), toolBody, false)
	if pidAuto != "coding" || srcAuto != "auto" {
		t.Fatalf("opt-in auto-detect got (%q, %q), want (coding, auto)", pidAuto, srcAuto)
	}
	var mAuto map[string]any
	_ = json.Unmarshal(outAuto, &mAuto)
	if sys := mAuto["messages"].([]any)[0].(map[string]any)["content"].(string); !strings.Contains(sys, "senior software engineer") {
		t.Fatalf("coding persona not injected in opt-in auto mode: %q", sys)
	}

	// Dashboard active persona beats auto-detect.
	_ = repo.UpdateSettingsRaw(map[string]any{"personaEnabled": true, "personaActive": "novelist", "personaAutoDetect": true})
	out, _, _, _, _, source := h.applyTokenSavers(context.Background(), toolBody, false)
	if source != "dashboard" {
		t.Fatalf("source = %q, want dashboard to beat auto", source)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	sys := m["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "seasoned novelist") || strings.Contains(sys, "senior software engineer") {
		t.Fatalf("dashboard persona must win over auto: %q", sys)
	}
}

func TestPromptEnvelopeTagIsolatesVariants(t *testing.T) {
	repo := openPersonaRepo(t)
	h := &ChatHandler{Repo: repo, TokenSaver: shared.NewTokenSaverConfig(false, false, false)}
	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)

	if got := h.promptEnvelopeTag(context.Background(), "gpt-test", body); got != "" {
		t.Fatalf("unmodified baseline must return empty envelope tag, got %q", got)
	}
	codingTag := h.promptEnvelopeTag(WithPersona(context.Background(), "coding"), "gpt-test", body)
	novelistTag := h.promptEnvelopeTag(WithPersona(context.Background(), "novelist"), "gpt-test", body)
	if codingTag == "" || novelistTag == "" || codingTag == novelistTag {
		t.Fatalf("distinct personas must produce distinct non-empty cache tags: coding=%q novelist=%q", codingTag, novelistTag)
	}
	bypassTag := h.promptEnvelopeTag(WithTokenSaverBypass(WithPersona(context.Background(), "coding")), "gpt-test", body)
	if bypassTag == codingTag {
		t.Fatalf("bypass must not share cache tag with active persona: %q vs %q", bypassTag, codingTag)
	}
}

func TestApplyContextBudgetBytesReservesEnvelopeOverhead(t *testing.T) {
	repo := openPersonaRepo(t)
	h := &ChatHandler{Repo: repo, TokenSaver: shared.NewTokenSaverConfig(false, false, false)}

	if err := repo.UpdateSettingsRaw(map[string]any{"personaEnabled": true, "personaActive": "coding"}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	limit := 150
	ctx := context.WithValue(context.Background(), middleware.ApiKeyContextKey, &models.APIKey{ContextLimit: &limit})
	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello world"}]}`)
	info := &ModelInfo{Provider: "openai", Model: "gpt-4o"}

	// Coding persona overhead exceeds 150 tokens, so explicit key ceiling must fail closed.
	if _, _, err := h.applyContextBudgetBytes(ctx, body, info, "gpt-test"); err == nil {
		t.Fatal("expected ContextLimitError when key ceiling cannot fit Prompt Envelope overhead")
	}

	// Without an active envelope and with higher limit (500 tokens), it fits cleanly.
	_ = repo.UpdateSettingsRaw(map[string]any{"personaEnabled": false, "personaActive": ""})
	out, _, err := h.applyContextBudgetBytes(ctx, body, info, "gpt-test")
	if err != nil {
		t.Fatalf("unexpected error without envelope overhead: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty body")
	}
}

func TestCustomPersonaInjectionAndCacheTagIsolation(t *testing.T) {
	repo := openPersonaRepo(t)
	h := &ChatHandler{Repo: repo, TokenSaver: shared.NewTokenSaverConfig(false, false, false)}

	if err := repo.UpdateSettingsRaw(map[string]any{
		"personaEnabled": true,
		"personaActive":  "go-reviewer",
		"customPersonas": []map[string]any{
			{
				"id":          "go-reviewer",
				"name":        "Go Reviewer",
				"description": "Concise Go diffs.",
				"prefix":      "CUSTOM_GO_PREFIX_V1",
				"suffix":      "CUSTOM_GO_SUFFIX_V1",
			},
		},
	}); err != nil {
		t.Fatalf("seed custom persona settings: %v", err)
	}

	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"review this diff"}]}`)
	out, _, _, _, _, _ := h.applyTokenSavers(context.Background(), body, false)
	if !strings.Contains(string(out), "CUSTOM_GO_PREFIX_V1") || !strings.Contains(string(out), "CUSTOM_GO_SUFFIX_V1") {
		t.Fatalf("expected custom persona prefix/suffix injected, got %s", string(out))
	}

	tagV1 := h.promptEnvelopeTag(context.Background(), "gpt-test", body)
	if tagV1 == "" {
		t.Fatal("expected non-empty promptEnvelopeTag for active custom persona")
	}

	// Updating a custom persona's prompt content must invalidate the cache tag.
	if err := repo.UpdateSettingsRaw(map[string]any{
		"customPersonas": []map[string]any{
			{
				"id":     "go-reviewer",
				"name":   "Go Reviewer",
				"prefix": "CUSTOM_GO_PREFIX_V2",
				"suffix": "CUSTOM_GO_SUFFIX_V1",
			},
		},
	}); err != nil {
		t.Fatalf("update custom persona settings: %v", err)
	}
	tagV2 := h.promptEnvelopeTag(context.Background(), "gpt-test", body)
	if tagV2 == "" || tagV2 == tagV1 {
		t.Fatalf("editing custom persona must produce distinct cache tag: v1=%q v2=%q", tagV1, tagV2)
	}

	// Header override also resolves custom persona even when dashboard toggle is off.
	_ = repo.UpdateSettingsRaw(map[string]any{"personaEnabled": false, "personaActive": ""})
	outHdr, _, _, _, _, _ := h.applyTokenSavers(WithPersona(context.Background(), "go-reviewer"), body, false)
	if !strings.Contains(string(outHdr), "CUSTOM_GO_PREFIX_V2") {
		t.Fatalf("expected X-Persona header to resolve custom persona, got %s", string(outHdr))
	}
}
