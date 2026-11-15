package executor

import (
	"net/http"
	"sync/atomic"

	"9router/proxy/internal/proxy"
)

// Web-lane providers talk to consumer websites (gemini.google.com, grok.com,
// deepseek chat, perplexity.ai) instead of APIs. They are an arms race by
// nature — the sites change their wire shape and defenses without notice — so
// this fork ships them disabled by default (ENABLE_WEB_LANES=true to opt in),
// keeping the supported surface on real API providers.
var webLaneProviders = map[string]bool{
	"deepseek-web":   true,
	"gemini-web":     true,
	"grok-web":       true,
	"perplexity-web": true,
}

// webLanesEnabled is the registration policy for the web lanes. The library
// default is enabled so embedders and tests see the historical behavior; the
// app wiring applies the operator's config before RegisterAll runs.
var webLanesEnabled atomic.Bool

func init() { webLanesEnabled.Store(true) }

// SetWebLanesEnabled decides how RegisterAll registers the web-lane
// executors: enabled registers the real ones, disabled registers stubs that
// answer with a clear policy error instead of an opaque upstream failure.
// Call before RegisterAll (the app wiring does, at startup).
func SetWebLanesEnabled(v bool) { webLanesEnabled.Store(v) }

// WebLanesEnabled reports the current registration policy.
func WebLanesEnabled() bool { return webLanesEnabled.Load() }

// IsWebLane reports whether the provider talks to a consumer website
// rather than an API.
func IsWebLane(provider string) bool { return webLaneProviders[provider] }

// ForwardDisabledWebLane is the executor registered for a web-lane provider
// when the policy has them disabled. It refuses with 501 so callers get a
// clear, actionable error instead of a passthrough that garbles the site's
// non-API wire shape. 501 is classified request-scoped, so a combo moves on
// to its next member instead of stalling on a lane the operator disabled.
func ForwardDisabledWebLane(w http.ResponseWriter, req *Request) error {
	return proxy.UpstreamFailure(http.StatusNotImplemented,
		"provider "+req.Provider+" is a website lane and is disabled by policy (set ENABLE_WEB_LANES=true to enable)")
}

// webLaneFactory picks the executor for a website lane: the real one when
// web lanes are enabled, the policy stub otherwise.
func webLaneFactory(real Executor) executorFactory {
	return func() Executor {
		if webLanesEnabled.Load() {
			return real
		}
		return ForwardDisabledWebLane
	}
}
