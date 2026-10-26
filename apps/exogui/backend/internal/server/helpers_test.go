package server

import (
	"io"
	"log"
	"os"
	"testing"
)

// silenceLog keeps the deliberate fail-closed log lines out of test output.
func silenceLog(t *testing.T) {
	t.Helper()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
}
