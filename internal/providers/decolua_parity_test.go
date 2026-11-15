package providers

import (
	"slices"
	"testing"
)

func TestDecoluaParity_Synthetic(t *testing.T) {
	if got := ResolveAlias("syn"); got != "synthetic" {
		t.Errorf("ResolveAlias(syn) = %q, want synthetic", got)
	}
	if got := ResolveAlias("synthetic-new"); got != "synthetic" {
		t.Errorf("ResolveAlias(synthetic-new) = %q, want synthetic", got)
	}

	cfg, ok := KnownProviders["synthetic"]
	if !ok {
		t.Fatal("KnownProviders[synthetic] missing")
	}
	if cfg.BaseURL != "https://api.synthetic.new/openai/v1/chat/completions" {
		t.Errorf("synthetic BaseURL = %q", cfg.BaseURL)
	}
	if cfg.ValidateURL != "https://api.synthetic.new/v2/quotas" {
		t.Errorf("synthetic ValidateURL = %q, want /v2/quotas", cfg.ValidateURL)
	}
	if cfg.UsageURL != "https://api.synthetic.new/v2/quotas" {
		t.Errorf("synthetic UsageURL = %q, want /v2/quotas", cfg.UsageURL)
	}

	meta, ok := ProviderRegistry["synthetic"]
	if !ok {
		t.Fatal("ProviderRegistry[synthetic] missing")
	}
	if !meta.PassthroughModels || !meta.Usage || meta.Alias != "syn" {
		t.Errorf("synthetic meta = %+v, want PassthroughModels=true, Usage=true, Alias=syn", meta)
	}

	models := ProviderModels["synthetic"]
	wantModels := []string{
		"hf:zai-org/GLM-5.3",
		"hf:zai-org/GLM-5.3-Flash",
		"hf:zai-org/GLM-4.7-Flash",
		"hf:moonshotai/Kimi-K3",
		"hf:deepseek-ai/DeepSeek-V4.1-Flash",
		"hf:deepseek-ai/DeepSeek-V4.1-Flash-1017",
		"hf:Qwen/Qwen3.8-27B",
		"hf:openai/gpt-oss-120b",
		"hf:nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4",
	}
	for _, want := range wantModels {
		if !slices.Contains(models, want) {
			t.Errorf("ProviderModels[synthetic] missing %q", want)
		}
	}

	if caps := GetCapabilitiesForModel("synthetic", "hf:deepseek-ai/DeepSeek-V4.1-Flash"); !caps.Reasoning {
		t.Errorf("expected hf:deepseek-ai/DeepSeek-V4.1-Flash to resolve reasoning capability, got %+v", caps)
	}
}

func TestDecoluaParity_SenseNovaIntlIsolated(t *testing.T) {
	cnCfg, ok := KnownProviders["sensenova"]
	if !ok {
		t.Fatal("KnownProviders[sensenova] missing")
	}
	intlCfg, ok := KnownProviders["sensenova-intl"]
	if !ok {
		t.Fatal("KnownProviders[sensenova-intl] missing")
	}
	if cnCfg.BaseURL == intlCfg.BaseURL {
		t.Fatalf("sensenova and sensenova-intl must remain isolated: both use %q", intlCfg.BaseURL)
	}
	if intlCfg.BaseURL != "https://token.sensenova.ai/v1/chat/completions" {
		t.Errorf("sensenova-intl BaseURL = %q", intlCfg.BaseURL)
	}
	if intlCfg.ImageURL != "https://token.sensenova.ai/v1/images/generations" {
		t.Errorf("sensenova-intl ImageURL = %q", intlCfg.ImageURL)
	}
	if !intlCfg.ReasoningAliasFields {
		t.Error("sensenova-intl must enable ReasoningAliasFields")
	}
	if ResolveAlias("sensenova") != "sensenova" || ResolveAlias("sensenova-intl") != "sensenova-intl" {
		t.Error("sensenova and sensenova-intl aliases must stay distinct")
	}

	for _, imgModel := range []string{"sensenova-u1.5-lite", "sensenova-u1-fast"} {
		if kind := GetProviderModelKind("sensenova-intl", imgModel); kind != "image" {
			t.Errorf("GetProviderModelKind(sensenova-intl, %q) = %q, want image", imgModel, kind)
		}
	}
	if kind := GetProviderModelKind("sensenova-intl", "sensenova-6.7-flash-lite"); kind == "image" {
		t.Errorf("sensenova-6.7-flash-lite must remain an LLM model, got kind=%q", kind)
	}
	if caps := GetCapabilitiesForModel("sensenova-intl", "sensenova-6.7-flash-lite"); !caps.Vision || !caps.Reasoning {
		t.Errorf("expected Vision+Reasoning for sensenova-6.7-flash-lite, got %+v", caps)
	}
}

func TestDecoluaParity_ClinePassUsageFlag(t *testing.T) {
	meta, ok := ProviderRegistry["clinepass"]
	if !ok {
		t.Fatal("ProviderRegistry[clinepass] missing")
	}
	if !meta.Usage {
		t.Error("expected ProviderRegistry[clinepass].Usage == true")
	}
}

