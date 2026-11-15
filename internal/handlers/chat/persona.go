package chat

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"

	"9router/proxy/internal/persona"
	"9router/proxy/internal/tokensaver"
)

// The client asks for a persona with the X-9Router-Persona (or X-Persona)
// request header: the gateway resolves it at the lane and injects its
// instruction pack around the system prompt, so the caller never restates
// task rules per request. Unknown ids resolve to no persona — a caller that
// misspells one gets the plain lane, not an error.
type personaCtxKey struct{}

// WithPersona records the persona id the request resolved to.
func WithPersona(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, personaCtxKey{}, id)
}

// PersonaFromContext returns the persona id carried by the request, if any.
func PersonaFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(personaCtxKey{}).(string)
	return id
}

// PersonaFromHeader extracts the persona id from the request headers.
// X-9Router-Persona is the canonical name; X-Persona is accepted as the
// client-friendly alias. Empty string when neither is set.
func PersonaFromHeader(get func(string) string) string {
	for _, name := range []string{"X-9Router-Persona", "X-Persona"} {
		if id := strings.TrimSpace(get(name)); id != "" {
			return id
		}
	}
	return ""
}

// personaByID resolves a persona id against built-in and custom personas.
func (h *ChatHandler) personaByID(id string) *persona.Persona {
	if p := persona.Resolve(id); p != nil {
		return p
	}
	if h != nil && h.Repo != nil {
		if s, err := h.Repo.GetSettings(); err == nil && s != nil {
			return persona.ResolveWithCustom(id, s.CustomPersonas)
		}
	}
	return nil
}

// autoDetectPersona is the auto-tuner's opt-in signal: a request carrying tools
// is agentic or coding work, so the coding pack fits it when personaAutoDetect
// is enabled. Returns "" when nothing names a task or body is unparseable.
func autoDetectPersona(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var probe struct {
		Tools json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || len(probe.Tools) == 0 {
		return ""
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(probe.Tools, &tools); err != nil || len(tools) == 0 {
		return ""
	}
	return persona.CodingPersonaID
}

// resolveEffectivePersona resolves the active persona pack and source for a
// request using exact precedence:
//  1. Explicit request header (X-9Router-Persona / X-Persona); unknown IDs stop at none.
//  2. Dashboard active persona when personaEnabled is true.
//  3. Opt-in auto-detection on tool-bearing requests when personaAutoDetect is true.
func (h *ChatHandler) resolveEffectivePersonaPack(ctx context.Context, body []byte) (*persona.Persona, string) {
	if TokenSaverBypassEnabled(ctx) {
		return nil, ""
	}
	if hid := PersonaFromContext(ctx); hid != "" {
		if p := h.personaByID(hid); p != nil {
			return p, "header"
		}
		return nil, ""
	}
	if h == nil || h.Repo == nil {
		return nil, ""
	}
	s, err := h.Repo.GetSettings()
	if err != nil || s == nil {
		return nil, ""
	}
	if s.PersonaEnabled && s.PersonaActive != "" {
		if p := persona.ResolveWithCustom(s.PersonaActive, s.CustomPersonas); p != nil {
			return p, "dashboard"
		}
	}
	if s.PersonaAutoDetect {
		if auto := autoDetectPersona(body); auto != "" {
			if p := persona.ResolveWithCustom(auto, s.CustomPersonas); p != nil {
				return p, "auto"
			}
		}
	}
	return nil, ""
}

func (h *ChatHandler) resolveEffectivePersona(ctx context.Context, body []byte) (string, string) {
	if p, src := h.resolveEffectivePersonaPack(ctx, body); p != nil {
		return p.ID, src
	}
	return "", ""
}

// promptEnvelopeTag returns a short deterministic cache-isolation tag for any
// active Prompt Envelope layer (Instruction Preset, Task Persona, TokenSaver
// prompts/compression). Returns "" when no modifier applies so un-modified traffic
// keeps legacy cache keys untouched.
func (h *ChatHandler) promptEnvelopeTag(ctx context.Context, modelName string, body []byte) string {
	if h == nil {
		return ""
	}
	doc := h.resolveJailbreak(modelName)
	bypass := TokenSaverBypassEnabled(ctx)
	personaPack, _ := h.resolveEffectivePersonaPack(ctx, body)
	var pid string
	if personaPack != nil {
		pid = personaPack.ID
	}
	hasTS := !bypass && h.hasTokenSaverEnvelope()
	if doc == nil && pid == "" && !hasTS {
		return ""
	}

	hasher := sha256.New()
	writeTagPart(hasher, "bypass", boolStr(bypass))
	if doc != nil {
		writeTagPart(hasher, "jb", doc.ID+"|"+doc.UpdatedAt+"|"+boolStr(doc.GeminiSafety)+"|"+doc.Prompt)
	}
	if personaPack != nil {
		writeTagPart(hasher, "persona", personaPack.ID+"|"+personaPack.Prefix+"|"+personaPack.Suffix)
	}
	if hasTS {
		h.writeTokenSaverTagParts(hasher)
	}
	sum := hasher.Sum(nil)
	return "#env=" + hex.EncodeToString(sum[:6])
}

func (h *ChatHandler) hasTokenSaverEnvelope() bool {
	if h == nil || h.TokenSaver == nil {
		return false
	}
	ts := h.TokenSaver
	return ts.RTKEnabled() ||
		ts.CavemanEnabled() ||
		ts.CavemanInputMode() ||
		ts.PonytailEnabled() ||
		ts.ADHDEnabled() ||
		ts.HeadroomEnabled() ||
		(ts.SystemPromptEnabled() && (ts.SystemPromptPrefix() != "" || ts.SystemPromptSuffix() != ""))
}

func (h *ChatHandler) writeTokenSaverTagParts(hasher interface{ Write([]byte) (int, error) }) {
	ts := h.TokenSaver
	if ts == nil {
		return
	}
	writeTagPart(hasher, "rtk", boolStr(ts.RTKEnabled()))
	if ts.CavemanEnabled() {
		writeTagPart(hasher, "caveman", ts.CavemanLevel()+"|"+ts.CavemanLanguage()+"|"+boolStr(ts.CavemanAutoClarity()))
	}
	writeTagPart(hasher, "caveman_in", boolStr(ts.CavemanInputMode())+"|"+ts.CavemanPreserveKeywords())
	if ts.PonytailEnabled() {
		writeTagPart(hasher, "ponytail", ts.PonytailLevel())
	}
	if ts.ADHDEnabled() {
		writeTagPart(hasher, "adhd", ts.ADHDLevel())
	}
	if ts.HeadroomEnabled() {
		writeTagPart(hasher, "headroom", ts.HeadroomEngineName())
	}
	if ts.SystemPromptEnabled() {
		writeTagPart(hasher, "sysprompt", ts.SystemPromptPrefix()+"|"+ts.SystemPromptSuffix())
	}
}

func writeTagPart(hasher interface{ Write([]byte) (int, error) }, key, val string) {
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(key)))
	_, _ = hasher.Write(lenBuf[:])
	_, _ = hasher.Write([]byte(key))
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(val)))
	_, _ = hasher.Write(lenBuf[:])
	_, _ = hasher.Write([]byte(val))
}

func boolStr(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// estimateEnvelopeOverhead estimates added tokens from Instruction Presets,
// Task Personas, and TokenSaver system prompts before context-budget trimming runs.
func (h *ChatHandler) estimateEnvelopeOverhead(ctx context.Context, requestedModel string, legIDs []string, body []byte) int {
	if h == nil {
		return 0
	}
	extraBytes := 0
	if doc := h.resolveJailbreak(append([]string{requestedModel}, legIDs...)...); doc != nil {
		extraBytes += len(doc.Prompt)
	}
	if !TokenSaverBypassEnabled(ctx) {
		if p, _ := h.resolveEffectivePersonaPack(ctx, body); p != nil {
			extraBytes += len(p.Prefix) + len(p.Suffix)
		}
		if h.TokenSaver != nil {
			ts := h.TokenSaver
			if ts.CavemanEnabled() {
				extraBytes += len(tokensaver.GetCavemanPromptWithLang(ts.CavemanLevel(), ts.CavemanLanguage()))
			}
			if ts.PonytailEnabled() {
				extraBytes += len(tokensaver.GetPonytailPrompt(ts.PonytailLevel()))
			}
			if ts.ADHDEnabled() {
				extraBytes += len(tokensaver.GetADHDPrompt(ts.ADHDLevel()))
			}
			if ts.SystemPromptEnabled() {
				extraBytes += len(ts.SystemPromptPrefix()) + len(ts.SystemPromptSuffix())
			}
		}
	}
	if extraBytes <= 0 {
		return 0
	}
	return (extraBytes + 3) / 4
}

