package store

import (
	"context"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/model"
)

func TestDatabaseConfigUsesPrismaSchemaAsSearchPath(t *testing.T) {
	config, err := databaseConfig("postgresql://user:pass@localhost:5432/biotron?schema=logger")
	if err != nil {
		t.Fatal(err)
	}
	if got := config.ConnConfig.RuntimeParams["search_path"]; got != "logger,public" {
		t.Fatalf("search_path = %q", got)
	}
}

// Once the monitor has probed every component, the latest health comes from
// memory alone. The store here has no database, so a query would panic.
func TestLatestHealthAnswersFromMemory(t *testing.T) {
	store := &Store{latest: make(map[string]model.Health)}
	health := model.Health{Service: "calendar-api", OK: true, Detail: "HTTP 200 in 3ms", CheckedAt: time.Now()}
	store.SetLatestHealth(health)

	latest, err := store.LatestHealth(context.Background(), []string{"calendar-api"})
	if err != nil {
		t.Fatal(err)
	}
	if latest["calendar-api"] != health {
		t.Fatalf("latest = %#v, want %#v", latest["calendar-api"], health)
	}
}
