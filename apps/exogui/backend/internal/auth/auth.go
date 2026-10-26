// Package auth names what Exo is in Auth: its app id and the permissions it
// declares. The gate itself is go/authclient.
//
// Exo is the only product with a guest tier, so the invariant to hold is: a
// daily guest may use Live and Historical, never Commands. Auth enforces it
// (a guest's session answers false for `commands`), and Exo holds it by
// mounting every command route behind Require(PermissionCommands).
package auth

// AppID is Exo's application id in Auth. It must match the row in Auth's
// `apps` table.
const AppID = "exo-gui"

// The permission keys Exo declares. A daily guest key grants Live and
// Historical only.
const (
	PermissionLive       = "live"
	PermissionHistorical = "historical"
	PermissionCommands   = "commands"
)
