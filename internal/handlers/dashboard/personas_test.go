package dashboard

import (
	"bytes"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"9router/proxy/internal/persona"
	"github.com/go-chi/chi/v5"
)

func doPersonaRequest(t *testing.T, r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.MarshalWrite(&buf, body); err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestPersonasCRUDAndSettingsIntegration(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	h := NewDashboardHandler(repo)
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	// 1. Initial GET lists only the 3 built-in packs with BuiltIn=true.
	rec := doPersonaRequest(t, r, http.MethodGet, "/api/personas", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/personas status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Personas   []persona.Persona `json:"personas"`
		Active     string            `json:"active"`
		Enabled    bool              `json:"enabled"`
		AutoDetect bool              `json:"autoDetect"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResp.Personas) != 3 || !listResp.Personas[0].BuiltIn {
		t.Fatalf("expected 3 built-in personas initially, got %+v", listResp.Personas)
	}

	// 2. Reject colliding built-in ID or invalid ID on create.
	if r409 := doPersonaRequest(t, r, http.MethodPost, "/api/personas", persona.Persona{
		ID: "coding", Name: "Dup", Prefix: "sys",
	}); r409.Code != http.StatusConflict {
		t.Fatalf("expected 409 for built-in collision, got %d (%s)", r409.Code, r409.Body.String())
	}
	if r400 := doPersonaRequest(t, r, http.MethodPost, "/api/personas", persona.Persona{
		ID: "Bad Space!", Name: "Bad", Prefix: "sys",
	}); r400.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid id, got %d (%s)", r400.Code, r400.Body.String())
	}

	// 3. Create a valid custom persona.
	rec = doPersonaRequest(t, r, http.MethodPost, "/api/personas", persona.Persona{
		ID:          "sql-auditor",
		Name:        "SQL Auditor",
		Description: "Checks SQLite query plans and index coverage.",
		Prefix:      "Always inspect EXPLAIN QUERY PLAN before adding queries.",
		Suffix:      "Keep output SQLite-specific.",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/personas status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created persona.Persona
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID != "sql-auditor" || created.BuiltIn {
		t.Fatalf("unexpected created persona: %+v", created)
	}

	// Duplicate custom ID returns 409 Conflict.
	if dup := doPersonaRequest(t, r, http.MethodPost, "/api/personas", persona.Persona{
		ID: "sql-auditor", Name: "Dup", Prefix: "sys",
	}); dup.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate custom persona, got %d", dup.Code)
	}

	// 4. Activate the custom persona via PUT /api/settings.
	if putSet := doPersonaRequest(t, r, http.MethodPut, "/api/settings", map[string]any{
		"personaEnabled": true,
		"personaActive":  "sql-auditor",
	}); putSet.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings with custom personaActive failed: %d (%s)", putSet.Code, putSet.Body.String())
	}
	if badSet := doPersonaRequest(t, r, http.MethodPut, "/api/settings", map[string]any{
		"personaActive": "nonexistent-pack",
	}); badSet.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown personaActive, got %d", badSet.Code)
	}

	// 5. Update custom persona; reject editing built-in.
	if upBuiltIn := doPersonaRequest(t, r, http.MethodPut, "/api/personas/coding", persona.Persona{
		Name: "Hack", Prefix: "hack",
	}); upBuiltIn.Code != http.StatusConflict {
		t.Fatalf("expected 409 editing built-in, got %d", upBuiltIn.Code)
	}
	up := doPersonaRequest(t, r, http.MethodPut, "/api/personas/sql-auditor", persona.Persona{
		Name:        "SQL Auditor v2",
		Description: "Updated description.",
		Prefix:      "Prefer covering indexes.",
		Suffix:      "Zero sequential scans.",
	})
	if up.Code != http.StatusOK {
		t.Fatalf("PUT /api/personas/sql-auditor status = %d, body = %s", up.Code, up.Body.String())
	}

	// Verify list shows 4 personas with updated v2 and active state.
	rec = doPersonaRequest(t, r, http.MethodGet, "/api/personas", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list after update: %v", err)
	}
	if len(listResp.Personas) != 4 || listResp.Active != "sql-auditor" || !listResp.Enabled {
		t.Fatalf("unexpected list after update: len=%d active=%q enabled=%v", len(listResp.Personas), listResp.Active, listResp.Enabled)
	}

	// 6. Delete custom persona clears personaActive back to "".
	if delBuiltIn := doPersonaRequest(t, r, http.MethodDelete, "/api/personas/coding", nil); delBuiltIn.Code != http.StatusConflict {
		t.Fatalf("expected 409 deleting built-in, got %d", delBuiltIn.Code)
	}
	if del := doPersonaRequest(t, r, http.MethodDelete, "/api/personas/sql-auditor", nil); del.Code != http.StatusOK {
		t.Fatalf("DELETE /api/personas/sql-auditor status = %d, body = %s", del.Code, del.Body.String())
	}
	if delMissing := doPersonaRequest(t, r, http.MethodDelete, "/api/personas/sql-auditor", nil); delMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting removed persona, got %d", delMissing.Code)
	}

	rec = doPersonaRequest(t, r, http.MethodGet, "/api/personas", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list after delete: %v", err)
	}
	if len(listResp.Personas) != 3 || listResp.Active != "" {
		t.Fatalf("expected 3 personas and empty active after delete, got len=%d active=%q", len(listResp.Personas), listResp.Active)
	}
}

