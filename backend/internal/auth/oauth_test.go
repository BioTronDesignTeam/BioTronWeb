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
