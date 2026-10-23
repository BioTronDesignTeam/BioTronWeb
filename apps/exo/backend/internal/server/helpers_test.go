package server

import (
	"io"
	"log"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// One console line per request is noise in test output.
	accessLogOutput = io.Discard
	os.Exit(m.Run())
}

// silenceLog keeps the deliberate fail-closed log lines out of test output.
func silenceLog(t *testing.T) {
	t.Helper()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
}

// silenceStderr keeps a deliberate panic's stack out of test output.
func silenceStderr(t *testing.T) {
	t.Helper()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	os.Stderr = devNull
	t.Cleanup(func() {
		os.Stderr = stderr
		_ = devNull.Close()
	})
}
