package catalog

import "testing"

func TestDefaultCatalogHasUniqueServices(t *testing.T) {
	catalog, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Applications()) != 7 {
		t.Fatalf("applications = %d", len(catalog.Applications()))
	}
	services := make(map[string]bool)
	for _, app := range catalog.Applications() {
		for _, id := range ServiceIDs(app) {
			services[id] = true
		}
	}
	if !services["logger-api"] {
		t.Fatal("default catalog is missing logger-api")
	}
	for _, shared := range []string{"postgres", "edge-proxy"} {
		if !services[shared] {
			t.Fatalf("default catalog is missing the shared %s", shared)
		}
	}
}

// The database speaks no HTTP, so it is pinged through Status's own pool
// and carries no URL. Everything else still needs one, and a
// URL on a ping check would never be used, so it is refused rather than
// silently ignored.
func TestCatalogChecksDefaultToHTTPAndValidateTheirShape(t *testing.T) {
	catalog, err := Load(`[
      {"id":"infra","name":"Infra","components":[
        {"id":"db","name":"Database","check":"postgres"},
        {"id":"web","name":"Web","health_url":"http://web/"}
      ]}
    ]`)
	if err != nil {
		t.Fatal(err)
	}
	components := catalog.Applications()[0].Components
	if components[0].Check != CheckPostgres {
		t.Fatalf("dependency check = %q", components[0].Check)
	}
	if components[1].Check != CheckHTTP {
		t.Fatalf("an unspecified check should default to http, got %q", components[1].Check)
	}

	rejected := map[string]string{
		"http without a url":      `[{"id":"a","name":"A","components":[{"id":"x","name":"X"}]}]`,
		"unknown check":           `[{"id":"a","name":"A","components":[{"id":"x","name":"X","check":"redis"}]}]`,
		"url on a postgres check": `[{"id":"a","name":"A","components":[{"id":"x","name":"X","check":"postgres","health_url":"http://x/"}]}]`,
	}
	for name, raw := range rejected {
		if _, err := Load(raw); err == nil {
			t.Fatalf("%s: expected the catalog to be rejected", name)
		}
	}
}

func TestCatalogRejectsDuplicateComponents(t *testing.T) {
	_, err := Load(`[
      {"id":"one","name":"One","components":[{"id":"api","name":"API","health_url":"http://one"}]},
      {"id":"two","name":"Two","components":[{"id":"api","name":"API","health_url":"http://two"}]}
    ]`)
	if err == nil {
		t.Fatal("expected duplicate component error")
	}
}
