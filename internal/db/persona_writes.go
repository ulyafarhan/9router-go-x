package db

import (
	"errors"
	"fmt"
	"slices"

	"9router/proxy/internal/persona"
	"github.com/samber/lo"
)

var (
	// ErrPersonaInvalid indicates malformed custom persona fields or bounds violation.
	ErrPersonaInvalid = errors.New("invalid persona")
	// ErrPersonaBuiltIn indicates an attempt to overwrite, modify, or delete a built-in persona.
	ErrPersonaBuiltIn = errors.New("built-in persona cannot be modified")
	// ErrPersonaDuplicate indicates a custom persona with the same normalized ID already exists.
	ErrPersonaDuplicate = errors.New("persona id already exists")
	// ErrPersonaNotFound indicates no custom persona exists with the given ID.
	ErrPersonaNotFound = errors.New("persona not found")
	// ErrPersonaLimit indicates the custom persona storage cap has been reached.
	ErrPersonaLimit = errors.New("custom persona limit reached")
)

func prepareCustomPersona(p persona.Persona) (persona.Persona, error) {
	if norm := persona.NormalizeID(p.ID); norm != "" && persona.Resolve(norm) != nil {
		return persona.Persona{}, fmt.Errorf("%w: %s", ErrPersonaBuiltIn, norm)
	}
	clean, err := persona.ValidateCustom(p)
	if err != nil {
		return persona.Persona{}, fmt.Errorf("%w: %w", ErrPersonaInvalid, err)
	}
	return clean, nil
}

// CreateCustomPersona validates and persists a new operator-defined persona pack.
func (r *Repo) CreateCustomPersona(p persona.Persona) (persona.Persona, error) {
	clean, err := prepareCustomPersona(p)
	if err != nil {
		return persona.Persona{}, err
	}
	err = r.updateSettings(func(existing map[string]any) error {
		items := parseCustomPersonas(existing["customPersonas"])
		if lo.ContainsBy(items, func(it persona.Persona) bool { return it.ID == clean.ID }) {
			return fmt.Errorf("%w: %s", ErrPersonaDuplicate, clean.ID)
		}
		if len(items) >= persona.MaxCustomPersonas {
			return fmt.Errorf("%w (%d max)", ErrPersonaLimit, persona.MaxCustomPersonas)
		}
		items = append(items, clean)
		existing["customPersonas"] = items
		return nil
	})
	if err != nil {
		return persona.Persona{}, err
	}
	return clean, nil
}

// UpdateCustomPersona validates and replaces an existing custom persona pack.
func (r *Repo) UpdateCustomPersona(id string, p persona.Persona) (persona.Persona, error) {
	norm := persona.NormalizeID(id)
	if norm == "" {
		return persona.Persona{}, fmt.Errorf("%w: persona id is required", ErrPersonaInvalid)
	}
	if persona.Resolve(norm) != nil {
		return persona.Persona{}, fmt.Errorf("%w: %s", ErrPersonaBuiltIn, norm)
	}
	p.ID = norm
	clean, err := prepareCustomPersona(p)
	if err != nil {
		return persona.Persona{}, err
	}
	err = r.updateSettings(func(existing map[string]any) error {
		items := parseCustomPersonas(existing["customPersonas"])
		idx := slices.IndexFunc(items, func(it persona.Persona) bool { return it.ID == norm })
		if idx < 0 {
			return fmt.Errorf("%w: %s", ErrPersonaNotFound, norm)
		}
		items[idx] = clean
		existing["customPersonas"] = items
		return nil
	})
	if err != nil {
		return persona.Persona{}, err
	}
	return clean, nil
}

// DeleteCustomPersona removes a custom persona pack and resets personaActive if selected.
func (r *Repo) DeleteCustomPersona(id string) error {
	norm := persona.NormalizeID(id)
	if norm == "" {
		return fmt.Errorf("%w: persona id is required", ErrPersonaInvalid)
	}
	if persona.Resolve(norm) != nil {
		return fmt.Errorf("%w: %s", ErrPersonaBuiltIn, norm)
	}
	return r.updateSettings(func(existing map[string]any) error {
		items := parseCustomPersonas(existing["customPersonas"])
		filtered := lo.Filter(items, func(it persona.Persona, _ int) bool { return it.ID != norm })
		if len(filtered) == len(items) {
			return fmt.Errorf("%w: %s", ErrPersonaNotFound, norm)
		}
		existing["customPersonas"] = filtered
		if active, _ := existing["personaActive"].(string); persona.NormalizeID(active) == norm {
			existing["personaActive"] = ""
		}
		return nil
	})
}

