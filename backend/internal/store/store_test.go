package store

import "testing"

func TestDatabaseConfigUsesPrismaSchemaAsSearchPath(t *testing.T) {
	config, err := databaseConfig("postgresql://user:pass@localhost:5432/biotron?schema=oauth")
	if err != nil {
		t.Fatal(err)
	}
	if got := config.ConnConfig.RuntimeParams["search_path"]; got != "oauth,public" {
		t.Fatalf("search_path = %q", got)
	}
}

func TestDatabaseConfigRejectsInvalidSchema(t *testing.T) {
	if _, err := databaseConfig("postgresql://user:pass@localhost:5432/biotron?schema=oauth%2Cpublic"); err == nil {
		t.Fatal("expected invalid schema to be rejected")
	}
}
