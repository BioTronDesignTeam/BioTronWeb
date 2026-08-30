package catalog

import "testing"

func TestDefaultCatalogHasUniqueServices(t *testing.T) {
	catalog, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Applications()) != 6 {
		t.Fatalf("applications = %d", len(catalog.Applications()))
	}
	if !catalog.HasService("logger-api") {
		t.Fatal("default catalog is missing logger-api")
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
