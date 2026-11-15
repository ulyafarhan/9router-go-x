package persona

import (
	"strings"
	"testing"
)

func TestResolveCaseInsensitiveAndTrim(t *testing.T) {
	if p := Resolve("  CODING "); p == nil || p.ID != "coding" {
		t.Fatalf("Resolve(  CODING ) = %v, want the coding persona", p)
	}
}

func TestResolveUnknownAndEmpty(t *testing.T) {
	if p := Resolve("no-such-persona"); p != nil {
		t.Fatalf("Resolve(no-such-persona) = %v, want nil", p)
	}
	if p := Resolve(""); p != nil {
		t.Fatalf("Resolve(\"\") = %v, want nil", p)
	}
	if p := Resolve("   "); p != nil {
		t.Fatalf("Resolve(whitespace) = %v, want nil", p)
	}
}

func TestCatalogEntriesAreShaped(t *testing.T) {
	if len(Catalog()) < 2 {
		t.Fatalf("catalog has %d personas, want at least the coding/novelist pair", len(Catalog()))
	}
	seen := map[string]bool{}
	for _, p := range Catalog() {
		if p.ID == "" || p.Name == "" {
			t.Errorf("persona %+v has an empty id or name", p)
		}
		if strings.TrimSpace(p.Prefix) == "" {
			t.Errorf("persona %q has an empty prefix", p.ID)
		}
		if seen[p.ID] {
			t.Errorf("duplicate persona id %q", p.ID)
		}
		seen[p.ID] = true
	}
	if !seen["coding"] || !seen["novelist"] {
		t.Fatalf("catalog is missing the coding/novelist pair: %v", seen)
	}
}

func TestCustomPersonasValidationAndResolution(t *testing.T) {
	if _, err := ValidateCustom(Persona{ID: "coding", Name: "Override", Prefix: "x"}); err == nil {
		t.Fatal("expected collision with built-in id to fail")
	}
	if _, err := ValidateCustom(Persona{ID: "Bad Space", Name: "X", Prefix: "y"}); err == nil {
		t.Fatal("expected invalid id slug to fail")
	}
	custom, err := ValidateCustom(Persona{
		ID:          "  SEC-Audit ",
		Name:        "Security Auditor",
		Description: "Strict vulnerability review",
		Prefix:      "Review every diff for security flaws.",
		Suffix:      "Cite CWE ids.",
	})
	if err != nil {
		t.Fatalf("ValidateCustom failed: %v", err)
	}
	if custom.ID != "sec-audit" || custom.BuiltIn {
		t.Fatalf("unexpected normalized persona: %+v", custom)
	}
	customs := []Persona{
		custom,
		{ID: "coding", Name: "Fake Override", Prefix: "evil"},
	}
	if got := ResolveWithCustom("coding", customs); got == nil || !got.BuiltIn || got.Name != "Senior Engineer" {
		t.Fatalf("built-in persona must shadow custom with same id, got: %+v", got)
	}
	if got := ResolveWithCustom("SEC-AUDIT", customs); got == nil || got.BuiltIn || got.Prefix != custom.Prefix {
		t.Fatalf("ResolveWithCustom failed for custom id: %+v", got)
	}
	combined := CatalogWithCustom(customs)
	if len(combined) != len(Catalog())+1 {
		t.Fatalf("CatalogWithCustom len = %d, want %d", len(combined), len(Catalog())+1)
	}
}
