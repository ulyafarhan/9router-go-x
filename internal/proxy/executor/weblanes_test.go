package executor

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"9router/proxy/internal/proxy"
)

func TestIsWebLane(t *testing.T) {
	for _, p := range []string{"deepseek-web", "gemini-web", "grok-web", "perplexity-web"} {
		if !IsWebLane(p) {
			t.Errorf("IsWebLane(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"deepseek", "gemini", "grok-cli", "perplexity", "openai", "deepseek-webb", ""} {
		if IsWebLane(p) {
			t.Errorf("IsWebLane(%q) = true, want false", p)
		}
	}
}

func TestWebLaneFactoryFollowsPolicy(t *testing.T) {
	realPtr := reflect.ValueOf(Executor(ForwardDeepSeekWeb)).Pointer()
	stubPtr := reflect.ValueOf(Executor(ForwardDisabledWebLane)).Pointer()
	if realPtr == stubPtr {
		t.Fatal("ForwardDeepSeekWeb and ForwardDisabledWebLane must be distinct functions")
	}

	factory := webLaneFactory(ForwardDeepSeekWeb)

	SetWebLanesEnabled(true)
	if got := reflect.ValueOf(factory()).Pointer(); got != realPtr {
		t.Fatal("with web lanes enabled, the factory must return the real executor")
	}

	SetWebLanesEnabled(false)
	if got := reflect.ValueOf(factory()).Pointer(); got != stubPtr {
		t.Fatal("with web lanes disabled, the factory must return the policy stub")
	}

	// Restore the library default so other tests see the historical behavior.
	SetWebLanesEnabled(true)
}

func TestRegisterAllDisabledPolicyStub(t *testing.T) {
	SetWebLanesEnabled(false)
	t.Cleanup(func() { SetWebLanesEnabled(true) })

	RegisterAll()
	for _, p := range []string{"deepseek-web", "gemini-web", "grok-web", "perplexity-web"} {
		exec := Get(p)
		if exec == nil {
			t.Fatalf("Get(%q) = nil; a disabled lane must still register the stub", p)
		}
		if reflect.ValueOf(exec).Pointer() != reflect.ValueOf(Executor(ForwardDisabledWebLane)).Pointer() {
			t.Fatalf("Get(%q) did not return the policy stub while disabled", p)
		}
	}
	// A non-web provider must be untouched by the gate.
	if exec := Get("openai"); exec == nil {
		t.Fatal("Get(openai) = nil after RegisterAll")
	}
}

func TestForwardDisabledWebLane(t *testing.T) {
	err := ForwardDisabledWebLane(nil, &Request{Provider: "gemini-web"})
	if err == nil {
		t.Fatal("disabled web lane must refuse, not succeed")
	}
	var uf *proxy.UpstreamError
	if !errors.As(err, &uf) {
		t.Fatalf("error type = %T, want *proxy.UpstreamError", err)
	}
	if uf.StatusCode != http.StatusNotImplemented {
		t.Errorf("code = %d, want 501", uf.StatusCode)
	}
}
