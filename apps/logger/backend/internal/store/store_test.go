package store

import "testing"

func TestDatabaseConfigUsesPrismaSchemaAsSearchPath(t *testing.T) {
	config, err := databaseConfig("postgresql://user:pass@localhost:5432/biotron?schema=logger")
	if err != nil {
		t.Fatal(err)
	}
	if got := config.ConnConfig.RuntimeParams["search_path"]; got != "logger,public" {
		t.Fatalf("search_path = %q", got)
	}
}
