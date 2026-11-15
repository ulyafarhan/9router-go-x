package chat

import (
	"context"
	"testing"

	"9router/proxy/internal/middleware"
	"9router/proxy/internal/models"
)

// The rescue binds to the model window only when the key carries no ceiling
// of its own; an operator ceiling always wins, and both fail closed to no
// rescue when the window is unknown.
func TestRescueTriggered(t *testing.T) {
	ctx := context.Background()
	if rescueTriggered(ctx, 0) {
		t.Fatal("limit 0 must not count as a rescue")
	}
	limit := 5000
	kctx := context.WithValue(ctx, middleware.ApiKeyContextKey, &models.APIKey{ContextLimit: &limit})
	if rescueTriggered(kctx, limit) {
		t.Fatal("operator ceiling must not count as rescue")
	}
	if !rescueTriggered(ctx, 128000) {
		t.Fatal("model-window budget with no key ceiling is a rescue")
	}
}

// rescueLimits: a model unknown to the catalogue falls back to the
// catalogue's default window, so the rescue still has a budget to bind to
// (GetCapabilitiesDetailForModel's own fallback, not the tuner's guess).
func TestRescueLimitsUnknownModel(t *testing.T) {
	limit, window, maxOut := rescueLimits(context.Background(), "no-such-provider", "no-such-model")
	if limit <= 0 || limit != window {
		t.Fatalf("unknown model must bind to the fallback window, got limit=%d window=%d", limit, window)
	}
	if maxOut <= 0 {
		t.Fatalf("fallback must carry an output allowance, got %d", maxOut)
	}
}

// The rescue detail rides ctx so route headers survive combo retries.
func TestContextRescueCarrier(t *testing.T) {
	ctx := context.Background()
	if got := ContextRescueFrom(ctx); got != "" {
		t.Fatalf("empty ctx = %q, want empty", got)
	}
	if got := ContextRescueFrom(WithContextRescue(ctx, "context-rescue")); got != "context-rescue" {
		t.Fatalf("carrier = %q, want context-rescue", got)
	}
}
