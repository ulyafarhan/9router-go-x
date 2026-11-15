package dashboard

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"

	"9router/proxy/internal/providers"
	"9router/proxy/internal/updater"
)

// Endpoint variables allow deterministic httptest substitution in unit tests.
var (
	syntheticUsageURL = "https://api.synthetic.new/v2/quotas"
	clinepassUsageURL = "https://api.cline.bot/api/v1/users/me/plan/usage-limits"
)

// fetchSyntheticUsage queries Synthetic's /v2/quotas endpoint (read-only; does
// not decrement quota) and normalizes subscription limits into dashboard rows.
func fetchSyntheticUsage(ctx context.Context, apiKey string) usageResult {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return usageResult{message: "Synthetic API key not available. Add a key to view usage."}
	}

	status, _, out, err := usageGet(ctx, syntheticUsageURL, map[string]string{
		"Authorization": "Bearer " + key,
		"Accept":        "application/json",
	})
	if err != nil {
		return usageResult{message: fmt.Sprintf("Synthetic error: %v", err)}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return usageResult{plan: "Synthetic", message: "Synthetic authentication failed. Check the API key."}
	}
	if status < 200 || status >= 300 {
		return usageResult{plan: "Synthetic", message: fmt.Sprintf("Synthetic usage API error (%d)", status)}
	}

	data := usageJSON(out)
	if data == nil {
		return usageResult{plan: "Synthetic", message: "Synthetic returned no subscription quota."}
	}
	sub, ok := data["subscription"].(map[string]any)
	if !ok || sub == nil {
		return usageResult{plan: "Synthetic", message: "Synthetic returned no subscription quota."}
	}

	total := usageNum(sub["limit"], 0)
	if total <= 0 {
		return usageResult{plan: "Synthetic", message: "No Synthetic subscription quota found for this key."}
	}
	used := math.Max(0, usageNum(sub["requests"], 0))
	remaining := math.Max(0, total-used)
	resetAt := usageResetTime(sub["renewsAt"])

	entry := usageQuota(used, total, resetAt)
	entry["name"] = "Subscription requests"
	entry["remaining"] = remaining
	entry["percentage"] = math.Round(used / total * 100)
	entry["colorPercentage"] = math.Round(remaining / total * 100)
	entry["percentageIsUsed"] = true

	return usageResult{
		plan: "Synthetic",
		quotas: map[string]any{
			"Requests": entry,
		},
	}
}

var clinepassWindowOrder = []struct {
	key   string
	label string
}{
	{key: "five_hour", label: "Session (5h)"},
	{key: "weekly", label: "Weekly (7d)"},
	{key: "monthly", label: "Monthly"},
}

func clinepassUsageHeaders(accessToken, apiKey string) map[string]string {
	if key := strings.TrimSpace(apiKey); key != "" {
		return map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer " + key,
		}
	}
	if tok := strings.TrimSpace(accessToken); tok != "" {
		return providers.ClineIdentityHeaders(updater.CurrentVersion, map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer " + clineAuthHeader(tok),
		})
	}
	return nil
}

// fetchClinePassUsage queries ClinePass's /api/v1/users/me/plan/usage-limits
// endpoint using either the configured API key or WorkOS OAuth token.
func fetchClinePassUsage(ctx context.Context, accessToken, apiKey string) usageResult {
	headers := clinepassUsageHeaders(accessToken, apiKey)
	if headers == nil {
		return usageResult{message: "ClinePass access token or API key not available."}
	}

	status, _, out, err := usageGet(ctx, clinepassUsageURL, headers)
	if err != nil {
		return usageResult{message: fmt.Sprintf("ClinePass usage error: %v", err)}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return usageResult{plan: "ClinePass", message: "ClinePass authentication expired. Please re-authorize."}
	}
	if status < 200 || status >= 300 {
		msg := fmt.Sprintf("ClinePass usage API error (%d)", status)
		if t := strings.TrimSpace(string(out)); t != "" {
			if len(t) > 120 {
				t = t[:120]
			}
			msg += ": " + t
		}
		return usageResult{plan: "ClinePass", message: msg}
	}

	body := usageJSON(out)
	if body == nil {
		return usageResult{message: "ClinePass usage response was not JSON."}
	}
	dataMap, _ := body["data"].(map[string]any)
	rawLimits, _ := dataMap["limits"].([]any)

	byType := make(map[string]map[string]any, len(rawLimits))
	for _, item := range rawLimits {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ := usageStr(m["type"])
		if typ != "" {
			byType[typ] = m
		}
	}

	quotas := make(map[string]any, len(clinepassWindowOrder))
	for _, w := range clinepassWindowOrder {
		item, ok := byType[w.key]
		if !ok {
			continue
		}
		pctUsed := math.Max(0, math.Min(100, usageNum(item["percentUsed"], 0)))
		resetAt := usageResetTime(item["resetsAt"])
		quotas[w.label] = usageQuota(pctUsed, 100, resetAt)
	}
	if len(quotas) == 0 {
		return usageResult{
			plan:    "ClinePass",
			message: "ClinePass connected. No usage limits reported for this plan.",
		}
	}
	return usageResult{plan: "ClinePass", quotas: quotas}
}

