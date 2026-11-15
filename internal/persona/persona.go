// Package persona holds the gateway's curated, task-oriented instruction
// packs ("personas"). A persona is injected at the API lane itself — ahead of
// anything the client wrote — so every model on that lane, premium or free,
// receives the same mature task direction and the caller does not have to
// restate coding rules or prose rules per request.
//
// Personas are shaping instructions, not guardrail overrides: they direct how
// a task is done (style, structure, depth), never what the model is forbidden
// to refuse.
package persona

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxCustomPersonas bounds stored custom packs on small SQLite/1-vCPU deployments.
	MaxCustomPersonas = 64
	// MaxPersonaPrefixBytes bounds per-request injected prefix prompt bytes.
	MaxPersonaPrefixBytes = 16 * 1024
	// MaxPersonaSuffixBytes bounds per-request injected suffix prompt bytes.
	MaxPersonaSuffixBytes = 8 * 1024
)

var validPersonaIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)

// Persona is one curated or operator-defined instruction pack. Prefix is read
// first by the model; Suffix is its last word, which keeps direction alive
// across a long turn of untrusted content.
type Persona struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Prefix      string `json:"prefix"`
	Suffix      string `json:"suffix,omitempty"`
	BuiltIn     bool   `json:"builtIn"`
}

// The built-in catalog. IDs are stable: they appear in client headers
// (X-Persona / X-9Router-Persona) and will appear on lanes and API keys, so
// removing one is a breaking change — deprecate instead.
var catalog = []Persona{
	{
		ID:          "coding",
		Name:        "Senior Engineer",
		BuiltIn:     true,
		Description: "IDE/embedded coding: production quality, smallest correct change, honest test results.",
		Prefix: `You are a senior software engineer embedded in the user's IDE. For every task:
- Write production-quality code: correct, readable, idiomatic for the language; match the surrounding code's style, naming, and comment density.
- Prefer the smallest change that fully solves the task. No speculative abstractions, no premature optimization, no unrelated refactors.
- Handle errors at the boundaries; fail loudly with context instead of silently swallowing.
- State assumptions in one line before the code; after non-obvious code, explain the why in one short line.
- When a test suite exists, run it and report honest results, including failures; add focused tests for new behavior.
- If the request is ambiguous, pick the most conventional interpretation, note the alternative in one line, and proceed.
Never pad answers with restatements of the question.`,
		Suffix: "Respond with the task's solution first; meta commentary stays to one line when needed.",
	},
	{
		ID:          "novelist",
		Name:        "Novelist",
		BuiltIn:     true,
		Description: "Story/novel writing: in-character prose, continuity, forward momentum, no meta commentary.",
		Prefix: `You are a seasoned novelist writing the user's story. For every scene:
- Stay fully in character and in scene; never break the fourth wall and never add disclaimers, warnings, or meta commentary.
- Show through action, dialogue, and sensory detail rather than summary; vary sentence rhythm; avoid purple prose and repeated phrasing.
- Keep continuity: names, timelines, established facts, and each character's voice stay consistent across turns.
- Advance the story every turn — escalate stakes or deepen character; do not stall or write filler.
- Match the user's language and mirror their tone while keeping the prose mature and vivid.
- End a turn at a natural beat that invites the next move, without asking permission to continue.`,
		Suffix: "Continue the scene in character; no out-of-story commentary.",
	},
	{
		ID:          "general",
		Name:        "Precise Assistant",
		BuiltIn:     true,
		Description: "General tasks: answer first, structured, no filler.",
		Prefix: `You are a precise, thoughtful assistant. For every request:
- Answer the actual question in the first sentences; structure longer answers with short sections.
- Match the user's language. No filler, no restating the question, no unsolicited caveats.
- When you are unsure, say what you are unsure about and give your best answer anyway.`,
	},
}

// NormalizeID canonicalizes a persona identifier for lookup and persistence.
func NormalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// Resolve returns the built-in persona for the given id (case-insensitive),
// or nil when the id is unknown.
func Resolve(id string) *Persona {
	id = NormalizeID(id)
	if id == "" {
		return nil
	}
	for i := range catalog {
		if catalog[i].ID == id {
			p := catalog[i]
			return &p
		}
	}
	return nil
}

// ResolveWithCustom returns the built-in persona or a matching custom persona.
// Built-in IDs always take precedence over custom entries with the same ID.
func ResolveWithCustom(id string, customs []Persona) *Persona {
	if p := Resolve(id); p != nil {
		return p
	}
	norm := NormalizeID(id)
	if norm == "" {
		return nil
	}
	for i := range customs {
		if NormalizeID(customs[i].ID) == norm {
			p := customs[i]
			p.ID = norm
			p.BuiltIn = false
			return &p
		}
	}
	return nil
}

// CodingPersonaID is the stable id the auto-tuner's task detection applies
// to tool-bearing (agentic/coding) requests.
const CodingPersonaID = "coding"

// Catalog returns the built-in personas (read-only copies are not needed —
// callers must treat the result as immutable).
func Catalog() []Persona { return catalog }

// CatalogWithCustom returns built-in personas followed by non-colliding custom personas.
func CatalogWithCustom(customs []Persona) []Persona {
	out := make([]Persona, 0, len(catalog)+len(customs))
	out = append(out, catalog...)
	seen := make(map[string]bool, len(catalog)+len(customs))
	for _, b := range catalog {
		seen[b.ID] = true
	}
	for _, c := range customs {
		norm := NormalizeID(c.ID)
		if norm == "" || seen[norm] || strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Prefix) == "" {
			continue
		}
		seen[norm] = true
		c.ID = norm
		c.BuiltIn = false
		out = append(out, c)
	}
	return out
}

// ValidateCustom normalizes and validates a custom persona pack.
func ValidateCustom(p Persona) (Persona, error) {
	p.ID = NormalizeID(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	p.Prefix = strings.TrimSpace(p.Prefix)
	p.Suffix = strings.TrimSpace(p.Suffix)
	p.BuiltIn = false

	if !validPersonaIDRe.MatchString(p.ID) {
		return Persona{}, fmt.Errorf("persona id must match [a-z0-9][a-z0-9_-]{0,47}")
	}
	if Resolve(p.ID) != nil {
		return Persona{}, fmt.Errorf("persona id %q is reserved by a built-in persona", p.ID)
	}
	if p.Name == "" || len(p.Name) > 80 {
		return Persona{}, fmt.Errorf("persona name must be 1..80 characters")
	}
	if len(p.Description) > 240 {
		return Persona{}, fmt.Errorf("persona description must be <= 240 characters")
	}
	if p.Prefix == "" || len(p.Prefix) > MaxPersonaPrefixBytes {
		return Persona{}, fmt.Errorf("persona prefix must be 1..%d bytes", MaxPersonaPrefixBytes)
	}
	if len(p.Suffix) > MaxPersonaSuffixBytes {
		return Persona{}, fmt.Errorf("persona suffix must be <= %d bytes", MaxPersonaSuffixBytes)
	}
	return p, nil
}
