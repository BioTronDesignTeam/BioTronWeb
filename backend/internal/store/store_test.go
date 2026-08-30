package store

import (
	"context"
	"testing"
)

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

func TestGuestAccessIsScopedToProductView(t *testing.T) {
	st := &Store{}
	guest := &SessionOperator{GitHubID: GuestGitHubID, GuestAppID: "exo-gui"}

	allowed, err := st.Allowed(context.Background(), guest, "exo-gui", "view")
	if err != nil || !allowed {
		t.Fatalf("expected Exo guest view to be allowed, allowed=%v err=%v", allowed, err)
	}

	for _, tc := range []struct {
		appID      string
		permission string
	}{
		{appID: "calendar", permission: "view"},
		{appID: "exo-gui", permission: "write"},
	} {
		allowed, err := st.Allowed(context.Background(), guest, tc.appID, tc.permission)
		if err != nil {
			t.Fatalf("Allowed(%q, %q): %v", tc.appID, tc.permission, err)
		}
		if allowed {
			t.Errorf("guest unexpectedly allowed %s/%s", tc.appID, tc.permission)
		}
	}
}
