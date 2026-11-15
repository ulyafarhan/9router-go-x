package dashboard

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/persona"
)

const maxPersonaBodyBytes = 128 * 1024

// HandleListPersonas serves GET /api/personas: built-in plus custom personas
// paired with the operator's current activation state.
func (h *DashboardHandler) HandleListPersonas(w http.ResponseWriter, r *http.Request) {
	var customs []persona.Persona
	active, enabled, autoDetect := "", false, false
	if h.Repo != nil {
		if s, err := h.Repo.GetSettings(); err == nil && s != nil {
			active, enabled, autoDetect = s.PersonaActive, s.PersonaEnabled, s.PersonaAutoDetect
			customs = s.CustomPersonas
		}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"personas":   persona.CatalogWithCustom(customs),
		"active":     active,
		"enabled":    enabled,
		"autoDetect": autoDetect,
	})
}

// HandleCreatePersona serves POST /api/personas to save a new custom persona.
func (h *DashboardHandler) HandleCreatePersona(w http.ResponseWriter, r *http.Request) {
	if h.Repo == nil {
		handlerutil.WriteJSONError(w, http.StatusServiceUnavailable, "database repo unavailable")
		return
	}
	req, err := decodePersonaBody(r)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.Repo.CreateCustomPersona(req)
	if err != nil {
		writePersonaRepoError(w, err)
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, created)
}

// HandleUpdatePersona serves PUT /api/personas/{id} to update a custom persona.
func (h *DashboardHandler) HandleUpdatePersona(w http.ResponseWriter, r *http.Request) {
	if h.Repo == nil {
		handlerutil.WriteJSONError(w, http.StatusServiceUnavailable, "database repo unavailable")
		return
	}
	id := getURLParam(r, "id")
	req, err := decodePersonaBody(r)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.Repo.UpdateCustomPersona(id, req)
	if err != nil {
		writePersonaRepoError(w, err)
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, updated)
}

// HandleDeletePersona serves DELETE /api/personas/{id} to remove a custom persona.
func (h *DashboardHandler) HandleDeletePersona(w http.ResponseWriter, r *http.Request) {
	if h.Repo == nil {
		handlerutil.WriteJSONError(w, http.StatusServiceUnavailable, "database repo unavailable")
		return
	}
	id := getURLParam(r, "id")
	if err := h.Repo.DeleteCustomPersona(id); err != nil {
		writePersonaRepoError(w, err)
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "deleted",
		"id":     persona.NormalizeID(id),
	})
}

func decodePersonaBody(r *http.Request) (persona.Persona, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPersonaBodyBytes))
	if err != nil {
		return persona.Persona{}, fmt.Errorf("failed to read request body")
	}
	defer r.Body.Close()

	var p persona.Persona
	if err := json.Unmarshal(body, &p); err != nil {
		return persona.Persona{}, fmt.Errorf("invalid JSON")
	}
	return p, nil
}

func writePersonaRepoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrPersonaInvalid):
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, db.ErrPersonaNotFound):
		handlerutil.WriteJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrPersonaBuiltIn),
		errors.Is(err, db.ErrPersonaDuplicate),
		errors.Is(err, db.ErrPersonaLimit):
		handlerutil.WriteJSONError(w, http.StatusConflict, err.Error())
	default:
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
	}
}

// validatePersonaUpdate gates the settings boundary for the persona fields:
// customPersonas items must be valid and non-colliding, personaActive must
// name a known built-in or custom persona (or empty), and toggles must be bools.
func validatePersonaUpdate(repo *db.Repo, updates map[string]any) error {
	var customs []persona.Persona
	if rawCustoms, ok := updates["customPersonas"]; ok {
		parsed, err := validateCustomPersonasUpdate(rawCustoms)
		if err != nil {
			return err
		}
		customs = parsed
		updates["customPersonas"] = parsed
	} else if repo != nil {
		if s, err := repo.GetSettings(); err == nil && s != nil {
			customs = s.CustomPersonas
		}
	}

	if v, ok := updates["personaActive"]; ok && v != nil {
		s, okStr := v.(string)
		if !okStr {
			return fmt.Errorf("personaActive must be a string")
		}
		if s != "" && persona.ResolveWithCustom(s, customs) == nil {
			return fmt.Errorf("personaActive must be a known persona id or empty")
		}
	}
	for _, flag := range []string{"personaEnabled", "personaAutoDetect"} {
		if v, ok := updates[flag]; ok && v != nil {
			if _, okBool := v.(bool); !okBool {
				return fmt.Errorf("%s must be a boolean", flag)
			}
		}
	}
	return nil
}

func validateCustomPersonasUpdate(raw any) ([]persona.Persona, error) {
	if raw == nil {
		return []persona.Persona{}, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("customPersonas must be a valid array")
	}
	var parsed []persona.Persona
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil, fmt.Errorf("customPersonas must be an array of persona objects")
	}
	if len(parsed) > persona.MaxCustomPersonas {
		return nil, fmt.Errorf("customPersonas exceeds max %d entries", persona.MaxCustomPersonas)
	}
	seen := make(map[string]bool, len(parsed))
	cleanList := make([]persona.Persona, 0, len(parsed))
	for i, item := range parsed {
		clean, err := persona.ValidateCustom(item)
		if err != nil {
			return nil, fmt.Errorf("customPersonas[%d]: %w", i, err)
		}
		if seen[clean.ID] {
			return nil, fmt.Errorf("customPersonas[%d]: duplicate id %q", i, clean.ID)
		}
		seen[clean.ID] = true
		cleanList = append(cleanList, clean)
	}
	return cleanList, nil
}

