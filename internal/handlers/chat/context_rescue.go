package chat

import (
	"bytes"
	"context"

	"9router/proxy/internal/log"
	"9router/proxy/internal/middleware"
	"9router/proxy/internal/translator"
)

// The context rescue rides the request context so its evidence survives the
// combo loop: route headers are cleared between failed attempts and re-set by
// the served one, and the rescue itself happens once per request, before any
// dispatch. Carrying the decision on ctx keeps the two in sync.

type contextRescueKey struct{}

// WithContextRescue records that the auto-tuner trimmed this request to fit
// the model window, with the shape of the trim for the evidence header.
func WithContextRescue(ctx context.Context, detail string) context.Context {
	return context.WithValue(ctx, contextRescueKey{}, detail)
}

// ContextRescueFrom returns the rescue detail recorded on the request, or ""
// when the request was served as it arrived.
func ContextRescueFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(contextRescueKey{}).(string)
	return v
}

// applyContextBudgetBytes trims a JSON request body to the binding limit (key
// ceiling or model window rescue), reserving estimated Prompt Envelope overhead
// so injected system instructions still fit inside the ceiling.
func (h *ChatHandler) applyContextBudgetBytes(ctx context.Context, body []byte, modelInfo *ModelInfo, requestedModel string) ([]byte, context.Context, error) {
	limit, modelWindow, modelMaxOutput := rescueLimits(ctx, modelInfo.Provider, modelInfo.Model)
	if limit <= 0 {
		return body, ctx, nil
	}
	overhead := h.estimateEnvelopeOverhead(ctx, requestedModel, jailbreakLegIDs(modelInfo.Provider, modelInfo.Model), body)
	adjLimit, adjWindow, exhausted := adjustBudgetForEnvelope(limit, modelWindow, overhead)
	if exhausted {
		if keyContextLimit(ctx) > 0 {
			return body, ctx, &ContextLimitError{Window: limit, Used: overhead}
		}
		log.Info("autotune", "context rescue could not fit the window", "model", modelInfo.Model, "window", modelWindow)
		return body, ctx, nil
	}
	trimmed, limitErr := applyKeyContextLimit(body, adjLimit, adjWindow, modelMaxOutput)
	if limitErr != nil {
		if keyContextLimit(ctx) > 0 {
			return body, ctx, limitErr
		}
		log.Info("autotune", "context rescue could not fit the window", "model", modelInfo.Model, "window", modelWindow)
		return body, ctx, nil
	}
	if trimmed != nil && !bytes.Equal(trimmed, body) {
		rescued := middleware.EstimatePromptTokens(body) - middleware.EstimatePromptTokens(trimmed)
		ctx = WithContextRescue(ctx, "context-rescue")
		log.Info("autotune", "context rescue trimmed the request", "model", modelInfo.Model, "window", modelWindow, "tokensSavedEstimate", rescued)
		return trimmed, ctx, nil
	}
	return body, ctx, nil
}

// applyContextBudgetMap is applyContextBudgetBytes for an already-decoded body
// (/v1/messages, /v1/responses). Returns workingBody untouched when no trim ran.
func (h *ChatHandler) applyContextBudgetMap(ctx context.Context, workingBody map[string]any, rawBody []byte, modelInfo *ModelInfo, requestedModel string) (map[string]any, context.Context, error) {
	limit, modelWindow, modelMaxOutput := rescueLimits(ctx, modelInfo.Provider, modelInfo.Model)
	if limit <= 0 {
		return workingBody, ctx, nil
	}
	overhead := h.estimateEnvelopeOverhead(ctx, requestedModel, jailbreakLegIDs(modelInfo.Provider, modelInfo.Model), rawBody)
	adjLimit, adjWindow, exhausted := adjustBudgetForEnvelope(limit, modelWindow, overhead)
	if exhausted {
		if keyContextLimit(ctx) > 0 {
			return workingBody, ctx, &ContextLimitError{Window: limit, Used: overhead}
		}
		log.Info("autotune", "context rescue could not fit the window", "model", modelInfo.Model, "window", modelWindow)
		return workingBody, ctx, nil
	}
	trimmed, limitErr := applyKeyContextLimitMap(workingBody, adjLimit, adjWindow, modelMaxOutput)
	if limitErr != nil {
		if keyContextLimit(ctx) > 0 {
			return workingBody, ctx, limitErr
		}
		log.Info("autotune", "context rescue could not fit the window", "model", modelInfo.Model, "window", modelWindow)
		return workingBody, ctx, nil
	}
	if trimmed != nil {
		ctx = WithContextRescue(ctx, "context-rescue")
		log.Info("autotune", "context rescue trimmed the request", "model", modelInfo.Model, "window", modelWindow)
		return trimmed, ctx, nil
	}
	return workingBody, ctx, nil
}

// withEnvelopeCacheKey isolates a SemanticCache lookup/store copy when an
// active Prompt Envelope modifier changes the system prompt or compression mode.
func withEnvelopeCacheKey(req *translator.OpenAIRequest, envTag string) *translator.OpenAIRequest {
	if req == nil || envTag == "" {
		return req
	}
	cloned := *req
	msgs := make([]translator.OpenAIMessage, 0, len(req.Messages)+1)
	msgs = append(msgs, translator.OpenAIMessage{Role: "system", Content: envTag})
	msgs = append(msgs, req.Messages...)
	cloned.Messages = msgs
	return &cloned
}
