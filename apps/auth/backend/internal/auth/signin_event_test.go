package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

// A refused sign-in leaves no other trace: the operator is never stored, and
// the browser only sees ?auth=denied. This event is the whole record, so its
// message and its reason are pinned here.
func TestSignInRefusedIsReportedWhenTheCallerIsNotAnOrgMember(t *testing.T) {
	events, received := logclienttest.New(t, "oauth-manager")

	// GitHub answers 404 for the membership of a caller who is not in the
	// org, which the client reports as "not a member" and not as a failure of
	// the handshake.
	github := newStubGitHub(t, http.StatusNotFound)

	h := &Handler{
		GitHub: github.client(),
		Signer: NewSigner(testSigningKey),
		Events: events,
		Cfg:    Config{FrontendURL: "http://localhost:5173"},
	}
	request := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=s&code=good", nil)
	state, err := h.Signer.State(SignIn{State: "s", Verifier: "v"})
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(&http.Cookie{Name: stateCookie, Value: state})
	response := httptest.NewRecorder()
	h.Callback(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302 back to the frontend", response.Code)
	}
	if got := response.Header().Get("Location"); got != "http://localhost:5173/?auth=denied" {
		t.Errorf("Location = %q, want the frontend with auth=denied", got)
	}

	select {
	case e := <-received:
		if e.Message != "Sign-in refused" {
			t.Errorf("message = %q, want %q", e.Message, "Sign-in refused")
		}
		if e.Level != logclient.Warning {
			t.Errorf("level = %q, want warning: a refusal is not a failure of the service", e.Level)
		}
		if e.Service != "oauth-manager" {
			t.Errorf("service = %q, want oauth-manager", e.Service)
		}
		if e.Payload["reason"] != "not a member" {
			t.Errorf("payload reason = %v, want %q", e.Payload["reason"], "not a member")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event was exported; a refused sign-in went unrecorded")
	}
}

// The state cookie is signed: a bare value that matches the query, as the
// cookie of a forged sign-in would, is refused before GitHub is called.
func TestCallbackRefusesAnUnsignedState(t *testing.T) {
	h := &Handler{Signer: NewSigner(testSigningKey), Cfg: Config{FrontendURL: "http://localhost:5173"}}
	request := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=s&code=good", nil)
	request.AddCookie(&http.Cookie{Name: stateCookie, Value: "s"})
	response := httptest.NewRecorder()
	h.Callback(response, request)
	if response.Code != http.StatusBadRequest || response.Body.String() != "invalid OAuth state" {
		t.Fatalf("callback = %d %q, want 400 invalid OAuth state", response.Code, response.Body.String())
	}
}
