package store

import "testing"

func TestDatabaseConfigSetsCalendarSearchPath(t *testing.T) {
	config, err := databaseConfig("postgresql://user:pass@localhost:5432/biotron?schema=calendar")
	if err != nil {
		t.Fatal(err)
	}
	if got := config.ConnConfig.RuntimeParams["search_path"]; got != "calendar,public" {
		t.Fatalf("unexpected search path %q", got)
	}
}

func TestSlug(t *testing.T) {
	if got := Slug(" Exo: Controls & Signals "); got != "exo-controls-signals" {
		t.Fatalf("unexpected slug %q", got)
	}
}
