package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9router/proxy/internal/providers"
)

func TestFetchSyntheticUsage(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		statusCode  int
		body        string
		wantPlan    string
		wantMessage string
		wantUsed    float64
		wantTotal   float64
	}{
		{
			name:        "missing api key",
			apiKey:      "  ",
			wantMessage: "Synthetic API key not available",
		},
		{
			name:        "unauthorized key",
			apiKey:      "syn-bad",
			statusCode:  http.StatusUnauthorized,
			body:        `{"error":"unauthorized"}`,
			wantPlan:    "Synthetic",
			wantMessage: "authentication failed",
		},
		{
			name:       "valid subscription quota",
			apiKey:     "syn-ok",
			statusCode: http.StatusOK,
			body:       `{"subscription":{"limit":135,"requests":27,"renewsAt":"2026-12-01T00:00:00Z"}}`,
			wantPlan:   "Synthetic",
			wantUsed:   27,
			wantTotal:  135,
		},
		{
			name:        "zero limit quota",
			apiKey:      "syn-zero",
			statusCode:  http.StatusOK,
			body:        `{"subscription":{"limit":0,"requests":0}}`,
			wantPlan:    "Synthetic",
			wantMessage: "No Synthetic subscription quota found",
		},
		{
			name:        "malformed json body",
			apiKey:      "syn-badjson",
			statusCode:  http.StatusOK,
			body:        `not-json`,
			wantPlan:    "Synthetic",
			wantMessage: "no subscription quota",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+strings.TrimSpace(tt.apiKey) {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			prev := syntheticUsageURL
			syntheticUsageURL = srv.URL
			defer func() { syntheticUsageURL = prev }()

			res := fetchSyntheticUsage(context.Background(), tt.apiKey)
			if tt.wantPlan != "" && res.plan != tt.wantPlan {
				t.Errorf("plan = %q, want %q", res.plan, tt.wantPlan)
			}
			if tt.wantMessage != "" {
				if !strings.Contains(res.message, tt.wantMessage) {
					t.Errorf("message = %q, want substring %q", res.message, tt.wantMessage)
				}
				return
			}
			q, ok := res.quotas["Requests"].(map[string]any)
			if !ok {
				t.Fatalf("missing Requests quota row in %+v", res.quotas)
			}
			if got := q["used"].(float64); got != tt.wantUsed {
				t.Errorf("used = %v, want %v", got, tt.wantUsed)
			}
			if got := q["total"].(float64); got != tt.wantTotal {
				t.Errorf("total = %v, want %v", got, tt.wantTotal)
			}
			if got := q["remainingPercentage"].(float64); got != 80 {
				t.Errorf("remainingPercentage = %v, want 80", got)
			}
		})
	}
}

func TestFetchClinePassUsage(t *testing.T) {
	tests := []struct {
		name        string
		accessToken string
		apiKey      string
		statusCode  int
		body        string
		wantAuthPfx string
		wantMessage string
		wantKeys    []string
	}{
		{
			name:        "missing credentials",
			wantMessage: "not available",
		},
		{
			name:        "api key bearer maps windows",
			apiKey:      "cp_live_key",
			statusCode:  http.StatusOK,
			wantAuthPfx: "Bearer cp_live_key",
			body:        `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":25,"resetsAt":"2026-12-01T05:00:00Z"},{"type":"weekly","percentUsed":10,"resetsAt":"2026-12-07T00:00:00Z"},{"type":"monthly","percentUsed":50,"resetsAt":"2026-12-31T00:00:00Z"}]}}`,
			wantKeys:    []string{"Session (5h)", "Weekly (7d)", "Monthly"},
		},
		{
			name:        "oauth token adds workos prefix and cline headers",
			accessToken: "eyJhbGciOiJub25lIn0.eyJzdWIiOiIxIn0.sig",
			statusCode:  http.StatusOK,
			wantAuthPfx: "Bearer workos:",
			body:        `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":40,"resetsAt":"2026-12-01T05:00:00Z"}]}}`,
			wantKeys:    []string{"Session (5h)"},
		},
		{
			name:        "expired auth",
			apiKey:      "cp_bad",
			statusCode:  http.StatusForbidden,
			wantAuthPfx: "Bearer cp_bad",
			wantMessage: "authentication expired",
		},
		{
			name:        "empty limits returns connected message",
			apiKey:      "cp_empty",
			statusCode:  http.StatusOK,
			wantAuthPfx: "Bearer cp_empty",
			body:        `{"success":true,"data":{"limits":[]}}`,
			wantMessage: "No usage limits reported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth, gotClientType string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				gotClientType = r.Header.Get("X-CLIENT-TYPE")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			prev := clinepassUsageURL
			clinepassUsageURL = srv.URL
			defer func() { clinepassUsageURL = prev }()

			res := fetchClinePassUsage(context.Background(), tt.accessToken, tt.apiKey)
			if tt.wantAuthPfx != "" && !strings.HasPrefix(gotAuth, tt.wantAuthPfx) {
				t.Errorf("Authorization header = %q, want prefix %q", gotAuth, tt.wantAuthPfx)
			}
			if tt.accessToken != "" && tt.apiKey == "" && gotClientType != "9router" {
				t.Errorf("expected X-CLIENT-TYPE=9router for OAuth flow, got %q", gotClientType)
			}
			if tt.wantMessage != "" {
				if !strings.Contains(res.message, tt.wantMessage) {
					t.Errorf("message = %q, want substring %q", res.message, tt.wantMessage)
				}
				return
			}
			if len(res.quotas) != len(tt.wantKeys) {
				t.Fatalf("quotas len = %d, want %d (%v)", len(res.quotas), len(tt.wantKeys), res.quotas)
			}
			for _, k := range tt.wantKeys {
				if _, ok := res.quotas[k]; !ok {
					t.Errorf("missing quota window %q", k)
				}
			}
		})
	}
}

func TestValidateGeneric_CustomValidateURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/quotas" {
			t.Errorf("unexpected probe path %q, want /v2/quotas", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") == "Bearer syn-good" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"subscription":{"limit":100}}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := providers.ProviderConfig{
		BaseURL:     srv.URL + "/openai/v1/chat/completions",
		AuthHeader:  "Authorization",
		AuthScheme:  "bearer",
		ValidateURL: srv.URL + "/v2/quotas",
	}

	okOut := validateGeneric(context.Background(), "synthetic", cfg, "syn-good")
	if !okOut.valid || !okOut.supported {
		t.Errorf("good key outcome = %+v, want valid=true supported=true", okOut)
	}
	badOut := validateGeneric(context.Background(), "synthetic", cfg, "syn-bad")
	if badOut.valid || !badOut.supported {
		t.Errorf("bad key outcome = %+v, want valid=false supported=true", badOut)
	}
}

