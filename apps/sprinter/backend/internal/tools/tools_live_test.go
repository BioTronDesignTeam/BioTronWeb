package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/readstore"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/readstore/readstoretest"
)

// The SQL tool runs end to end against the read-only role here, because a
// stub reader proves the argument checks and nothing about the SQL. Set
// SPRINTER_TEST_READ_DATABASE_URL, and SPRINTER_TEST_OWNER_DATABASE_URL to
// seed the rows it reports; without them these skip.
//
//	SPRINTER_TEST_READ_DATABASE_URL='postgresql://sprinter_reader:change-me-reader@127.0.0.1:15433/biotron' \
//	  go test ./internal/tools/ -run Live -v
func liveReader(t *testing.T) (Reader, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("SPRINTER_TEST_READ_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SPRINTER_TEST_READ_DATABASE_URL is unset")
	}
	ctx := context.Background()
	store, err := readstore.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(store.Close)
	return store, ctx
}

func TestLiveSQLToolsAnswer(t *testing.T) {
	reader, ctx := liveReader(t)
	seeded := readstoretest.Seed(t)
	cases := []struct {
		tool  Tool
		args  string
		lines []string
	}{
		{WhoHasAccess{Reader: reader}, `{"app": "sprinter"}`, []string{
			"App sprinter (Sprinter).",
			"- admin (",
			"- " + seeded.Member + " (Live Member): admin, granted " + stamp(readstoretest.GrantedAt) +
				" [BANNED, refused everywhere]",
			"- " + seeded.Manager + " (Live Manager)",
		}},
	}
	for _, testCase := range cases {
		name := testCase.tool.Spec().Name
		t.Run(name, func(t *testing.T) {
			out, err := testCase.tool.Run(ctx, []byte(testCase.args))
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			for _, line := range testCase.lines {
				if !strings.Contains(out, line) {
					t.Errorf("output has no %q:\n%s", line, out)
				}
			}
			// Every tool caps its own output. Nothing may come back longer than
			// its cap plus the notice.
			if len(out) > accessCap+200 {
				t.Fatalf("result of %d characters escaped the cap", len(out))
			}
		})
	}
}
