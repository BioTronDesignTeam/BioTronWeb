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
	if !catalog.HasService("logger-api") {
		t.Fatal("default catalog is missing logger-api")
	}
	for _, shared := range []string{"postgres", "redis", "edge-proxy"} {
		if !catalog.HasService(shared) {
			t.Fatalf("default catalog is missing the shared %s", shared)
		}
	}
}

// The database and cache speak no HTTP, so they are pinged through Logger's
// own connections and carry no URL. Everything else still needs one, and a
// URL on a ping check would never be used, so it is refused rather than
// silently ignored.
func TestCatalogChecksDefaultToHTTPAndValidateTheirShape(t *testing.T) {
	catalog, err := Load(`[
      {"id":"infra","name":"Infra","components":[
        {"id":"db","name":"Database","check":"postgres"},
        {"id":"cache","name":"Cache","check":"redis"},
        {"id":"web","name":"Web","health_url":"http://web/"}
      ]}
    ]`)
	if err != nil {
		t.Fatal(err)
	}
	components := catalog.Applications()[0].Components
	if components[0].Check != CheckPostgres || components[1].Check != CheckRedis {
		t.Fatalf("dependency checks = %q %q", components[0].Check, components[1].Check)
	}
	if components[2].Check != CheckHTTP {
		t.Fatalf("an unspecified check should default to http, got %q", components[2].Check)
	}
	if app, _ := catalog.Application("infra"); app.Components[2].Check != CheckHTTP {
		t.Fatal("the normalised check must be visible through Application() too")
	}

	rejected := map[string]string{
		"http without a url":      `[{"id":"a","name":"A","components":[{"id":"x","name":"X"}]}]`,
		"unknown check":           `[{"id":"a","name":"A","components":[{"id":"x","name":"X","check":"mysql"}]}]`,
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
