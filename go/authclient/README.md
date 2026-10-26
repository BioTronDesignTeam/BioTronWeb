# authclient

The Go client and middleware every gated backend uses to ask Auth who is
signed in and what they may do. Import
`github.com/BioTronDesignTeam/BioTronWeb/go/authclient`. Like
`go/logclient`, each backend's `go.mod` replaces that path with
`../../../go/authclient` (and `go/logclient` with `../../../go/logclient`,
which this module uses), `go.work` lists it, and each backend Dockerfile
copies `go/authclient/` into the build.

Status and the site are public and must not depend on it.

```go
authz := authclient.New(cfg.OAuthManagerURL, "calendar", authclient.WithEvents(events))

// Every route under the prefix needs calendar/write.
mux.Handle("/v1/admin/", authz.Require("write")(admin))

// What the frontend reads to draw itself.
mux.Handle("GET /v1/auth/status", authz.StatusHandler(
	[]authclient.StatusField{{Permission: "write", Field: "can_write"}}, nil))

// Behind Require, the operator who passed.
operator := authclient.OperatorFrom(r)
```

## One call per request

`Lookup` asks `GET {OAUTH_MANAGER_URL}/v1/session?app=<id>&permission=<key>`
once, forwarding the request's whole `Cookie` header. Auth answers with the
operator and a verdict for each permission. A request with no cookie never
reaches Auth.

An Auth that predates `/v1/session` answers it with 404. The client then asks
`/auth/me` and one `/v1/check` per permission instead, and treats a session
that ends between those calls as signed out. That fallback exists only so a
backend deployed ahead of Auth keeps working.

There is no cache. Auth promises that a ban or a revoke refuses the very next
request, and a cache here would weaken that to "within the TTL".

## Require

| Request | Answer |
|---|---|
| Mutating method without `X-Requested-With: XMLHttpRequest` | 403, Auth not asked |
| No cookie, or Auth refuses the session (401, or 403 for a guest of another tool) | 401 |
| Signed in without the permission | 403 |
| Auth unreachable, `OAUTH_MANAGER_URL` unset, or an answer it cannot read | 503, logged and sent as an Error event |
| Signed in with the permission | passes, with the session on the request and the login as the request event's actor |

It never fails open. Wrap a whole route prefix, not single handlers, so a
route added under it later cannot forget the check.

The default refusal body is `{"error": "<message>"}`, with `"permission"`
added to a missing-permission 403.

## StatusHandler

Serves `/v1/auth/status`: the operator (left out when no one is signed in),
one boolean field per permission, `permissions` by name, and the tool's own
`extra` keys, which never replace those. An anonymous caller gets 200 with
every permission false; only an Auth that cannot answer is 503.

## Tests

`authclienttest.New(t)` starts a stand-in Auth that answers `/v1/session`.
`SignIn(operator, permissions...)`, `Answer(status)`, and `Calls()` drive it,
and `Client(appID)` returns a real client pointed at it.
