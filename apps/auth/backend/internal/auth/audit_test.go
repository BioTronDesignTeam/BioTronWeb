package auth

import (
	"testing"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// A Handler built without an event client, as every test and a Logger-less
// environment does, must still ban, grant, and flag without a nil dereference.
func TestAuditWithoutAnEventClientIsANoOp(t *testing.T) {
	h := &Handler{}
	h.audit("Permission granted", &store.SessionOperator{GitHubID: 1, Login: "manager"}, map[string]any{"target_id": int64(2)})
	h.audit("Operator banned", nil, map[string]any{})
}
