package store

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	want := logCursor{CreatedAt: time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC), ID: 42}
	got, err := decodeCursor(encodeCursor(want))
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.ID != want.ID {
		t.Fatalf("cursor = %#v, want %#v", got, want)
	}
}

func TestDatabaseConfigUsesPrismaSchemaAsSearchPath(t *testing.T) {
	config, err := databaseConfig("postgresql://user:pass@localhost:5432/biotron?schema=logger")
	if err != nil {
		t.Fatal(err)
	}
	if got := config.ConnConfig.RuntimeParams["search_path"]; got != "logger,public" {
		t.Fatalf("search_path = %q", got)
	}
}
