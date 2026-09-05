package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOAuthAuthorizerForwardsSessionAndChecksLoggerView(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/check" || request.URL.Query().Get("app") != "logger" || request.URL.Query().Get("permission") != "view" {
			t.Errorf("unexpected permission request: %s", request.URL.String())
		}
		if request.Header.Get("Cookie") != "oauth_session=session-token" {
			t.Errorf("cookie = %q", request.Header.Get("Cookie"))
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"allowed":true}`))
	}))
	defer server.Close()

	decision, err := NewOAuthAuthorizer(server.URL).Authorize(context.Background(), "oauth_session=session-token")
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Authenticated || !decision.Allowed {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestOAuthAuthorizerTreatsMissingCookieAsAnonymous(t *testing.T) {
	decision, err := NewOAuthAuthorizer("http://unused").Authorize(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Authenticated || decision.Allowed {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestOAuthAuthorizerDistinguishesUnauthenticatedFromUnpermitted(t *testing.T) {
	cases := []struct {
		name              string
		status            int
		body              string
		wantAuthenticated bool
		wantAllowed       bool
	}{
		{name: "no session", status: http.StatusUnauthorized, wantAuthenticated: false, wantAllowed: false},
		{name: "session without logger view", status: http.StatusForbidden, wantAuthenticated: true, wantAllowed: false},
		{name: "session denied in body", status: http.StatusOK, body: `{"allowed":false}`, wantAuthenticated: true, wantAllowed: false},
		{name: "session granted", status: http.StatusOK, body: `{"allowed":true}`, wantAuthenticated: true, wantAllowed: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(testCase.status)
				_, _ = response.Write([]byte(testCase.body))
			}))
			defer server.Close()

			decision, err := NewOAuthAuthorizer(server.URL).Authorize(context.Background(), "oauth_session=token")
			if err != nil {
				t.Fatal(err)
			}
			if decision.Authenticated != testCase.wantAuthenticated || decision.Allowed != testCase.wantAllowed {
				t.Fatalf("decision = %#v, want authenticated=%v allowed=%v", decision, testCase.wantAuthenticated, testCase.wantAllowed)
			}
		})
	}
}

func TestOAuthAuthorizerReportsTransportFailure(t *testing.T) {
	// A dead permission service must surface as an error so the caller can answer 503
	// rather than silently presenting the portal as signed out.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()

	if _, err := NewOAuthAuthorizer(server.URL).Authorize(context.Background(), "oauth_session=token"); err == nil {
		t.Fatal("expected a transport error")
	}
}
