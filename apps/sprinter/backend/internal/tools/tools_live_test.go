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
		{RecentLogs{Reader: reader}, `{"limit": 5}`},
		{RecentLogs{Reader: reader}, `{"service": "logger-api", "level": "info", "search": "e", "limit": 3}`},
		{LogHistory{Reader: reader}, `{"limit": 2}`},
		{LogHistory{Reader: reader}, `{"limit": 2, "level": "error", "from": "2020-01-01T00:00:00Z"}`},
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
			// Every tool caps its own output, and the loosest cap is the log
			// history's. Nothing may come back longer than that plus its notice.
			if len(out) > logHistoryCap+200 {
				t.Fatalf("result of %d characters escaped the cap", len(out))
			}
		})
	}
}

// TestLiveLogHistoryPagesWithItsOwnCursor walks a second page with the cursor
// the first page returned, which is the only way to see that the cursor the
// model is handed actually works when it comes back.
func TestLiveLogHistoryPagesWithItsOwnCursor(t *testing.T) {
	reader, ctx := liveReader(t)
	tool := LogHistory{Reader: reader}
	first, err := tool.Run(ctx, []byte(`{"limit": 1}`))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	marker := "next_cursor: "
	at := strings.LastIndex(first, marker)
	if at < 0 {
		t.Fatal("a page must always report a cursor line")
	}
	cursor := strings.TrimSpace(first[at+len(marker):])
	if strings.HasPrefix(cursor, "none") {
		t.Skip("only one log row exists, so there is no second page")
	}
	second, err := tool.Run(ctx, []byte(`{"limit": 1, "cursor": "`+cursor+`"}`))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if second == first {
		t.Fatal("the cursor returned the same page")
	}
}
