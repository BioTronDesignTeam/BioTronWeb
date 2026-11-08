// Package readstoretest seeds the rows the read pool's live tests assert on.
//
// The read role cannot write, so the rows go in through a second connection
// that can: SPRINTER_TEST_OWNER_DATABASE_URL, the role that owns the oauth
// tables (the cluster's POSTGRES_USER in development). Every row gets a name
// no real row has, and is removed when the test ends, so the tests can run
// against a development database without disturbing it.
package readstoretest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// GrantedAt is when the seeded grant was made, so a test can assert the
// exact line a tool writes for it.
var GrantedAt = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// Seeded names what Seed wrote.
type Seeded struct {
	// Member holds sprinter/admin and is banned.
	Member string
	// Manager is a manager with no grant.
	Manager string
}

// Seed writes one banned grant holder and one manager, or skips the test when
// SPRINTER_TEST_OWNER_DATABASE_URL is unset.
func Seed(t *testing.T) Seeded {
	t.Helper()
	ownerURL := os.Getenv("SPRINTER_TEST_OWNER_DATABASE_URL")
	if ownerURL == "" {
		t.Skip("SPRINTER_TEST_OWNER_DATABASE_URL is unset")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		t.Fatalf("connect as owner: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	suffix := randomSuffix(t)
	seeded := Seeded{
		Member:  "live-member-" + suffix,
		Manager: "live-manager-" + suffix,
	}
	memberID, managerID := githubID(t), githubID(t)

	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO oauth.operators (github_id, login, name, is_banned) VALUES ($1, $2, 'Live Member', true)`,
			[]any{memberID, seeded.Member}},
		{`INSERT INTO oauth.operators (github_id, login, name, is_manager) VALUES ($1, $2, 'Live Manager', true)`,
			[]any{managerID, seeded.Manager}},
		{`INSERT INTO oauth.grants (operator_id, app_id, permission_key, created_at) VALUES ($1, 'sprinter', 'admin', $2)`,
			[]any{memberID, GrantedAt}},
	}
	t.Cleanup(func() {
		ctx := context.Background()
		// Grants go with their operator (ON DELETE CASCADE).
		if _, err := conn.Exec(ctx, `DELETE FROM oauth.operators WHERE github_id = ANY($1)`, []int64{memberID, managerID}); err != nil {
			t.Errorf("clean up operators: %v", err)
		}
	})
	for _, statement := range statements {
		if _, err := conn.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed %q: %v", statement.sql, err)
		}
	}
	return seeded
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("random: %v", err)
	}
	return hex.EncodeToString(raw[:])
}

// githubID is far above any real GitHub id, so a seeded operator can never
// collide with one who signed in.
func githubID(t *testing.T) int64 {
	t.Helper()
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("random: %v", err)
	}
	return 9_000_000_000_000 + (int64(raw[0])<<24 | int64(raw[1])<<16 | int64(raw[2])<<8 | int64(raw[3]))
}
