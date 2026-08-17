package installations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RobertWHurst/navaros"
)

func TestInstallationUsesCamelCaseJSON(t *testing.T) {
	raw, err := json.Marshal(Installation{ID: "1", StartedAt: time.Unix(1, 0).UTC()})
	if err != nil {
		t.Fatalf("marshal installation: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode installation: %v", err)
	}
	if _, ok := body["startedAt"]; !ok {
		t.Fatalf("startedAt missing from %#v", body)
	}
	if _, ok := body["started_at"]; ok {
		t.Fatalf("retired started_at leaked into %#v", body)
	}
}

func TestInstallationLogUsesDollarOffsetOnly(t *testing.T) {
	manager := NewManager(t.TempDir())
	manager.installations["1"] = &Installation{ID: "1", log: []string{"first", "second"}}

	dispatch := func(target string) *navaros.Context {
		t.Helper()
		ctx := navaros.NewContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
		ctx.Set("manager", manager)
		navaros.CtxSetParam(ctx, "id", "1")
		t.Cleanup(func() { navaros.CtxFree(ctx) })
		getLog(ctx)
		return ctx
	}

	canonical := dispatch("/installations/1/log?$offset=1")
	if canonical.Status != http.StatusOK {
		t.Fatalf("canonical status = %d, want 200", canonical.Status)
	}
	canonicalBody := canonical.Body.(map[string]any)
	lines := canonicalBody["lines"].([]string)
	if len(lines) != 1 || lines[0] != "second" {
		t.Fatalf("canonical lines = %#v, want [second]", lines)
	}

	legacy := dispatch("/installations/1/log?offset=1")
	legacyLines := legacy.Body.(map[string]any)["lines"].([]string)
	if len(legacyLines) != 2 {
		t.Fatalf("legacy offset alias affected result: %#v", legacyLines)
	}

	invalid := dispatch("/installations/1/log?$offset=-1")
	if invalid.Status != http.StatusBadRequest {
		t.Fatalf("invalid offset status = %d, want 400", invalid.Status)
	}
}

func TestInstallationCollectionUsesCanonicalPagination(t *testing.T) {
	manager := NewManager(t.TempDir())
	base := time.Unix(1, 0).UTC()
	manager.installations["1"] = &Installation{ID: "1", Payload: "z", StartedAt: base}
	manager.installations["2"] = &Installation{ID: "2", Payload: "b", StartedAt: base.Add(time.Second)}
	manager.installations["3"] = &Installation{ID: "3", Payload: "a", StartedAt: base.Add(2 * time.Second)}

	dispatch := func(target string) *navaros.Context {
		t.Helper()
		ctx := navaros.NewContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
		ctx.Set("manager", manager)
		t.Cleanup(func() { navaros.CtxFree(ctx) })
		installationPaginationMiddleware(ctx)
		if ctx.Status == 0 {
			list(ctx)
		}
		return ctx
	}

	canonical := dispatch("/installations?$sort[payload]=asc&$offset=1&$limit=1")
	if canonical.Status != http.StatusOK {
		t.Fatalf("canonical status = %d, body = %#v", canonical.Status, canonical.Body)
	}
	page := canonical.Body.([]*Installation)
	if len(page) != 1 || page[0].ID != "2" {
		t.Fatalf("canonical page = %#v, want installation 2", page)
	}
	if got := canonical.Headers.Get("Total-Records-Count"); got != "3" {
		t.Fatalf("total header = %q, want 3", got)
	}

	countOnly := dispatch("/installations?$limit=0")
	if got := len(countOnly.Body.([]*Installation)); got != 0 {
		t.Fatalf("count-only body length = %d, want 0", got)
	}

	legacy := dispatch("/installations?limit=1&offset=2&sort=payload")
	if got := len(legacy.Body.([]*Installation)); got != 3 {
		t.Fatalf("legacy aliases changed result length to %d, want 3", got)
	}

}
