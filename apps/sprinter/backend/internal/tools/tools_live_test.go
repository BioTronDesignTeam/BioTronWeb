package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/readstore"
)

// The four SQL tools run end to end against the read-only role here, because a
// stub reader proves the argument checks and nothing about the SQL. Set
// SPRINTER_TEST_READ_DATABASE_URL; without it these skip.
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
	cases := []struct {
		tool Tool
		args string
	}{
		{HealthHistory{Reader: reader}, `{"service": "logger-api", "hours": 24}`},
		{WhoHasAccess{Reader: reader}, `{"app": "sprinter"}`},
	}
	for _, testCase := range cases {
		name := testCase.tool.Spec().Name + " " + testCase.args
		t.Run(name, func(t *testing.T) {
			out, err := testCase.tool.Run(ctx, []byte(testCase.args))
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if strings.TrimSpace(out) == "" {
				t.Fatal("a tool must never answer with nothing")
			}
			// Every tool caps its own output, and the loosest cap is 6000
			// characters. Nothing may come back longer than that plus its notice.
			if len(out) > max(healthCap, accessCap)+200 {
				t.Fatalf("result of %d characters escaped the cap", len(out))
			}
		})
	}
}
