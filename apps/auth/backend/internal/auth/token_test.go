package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testSigningKey = []byte("test-signing-key-that-is-at-least-32-bytes")

func newTestSigner(now time.Time) *Signer {
	s := NewSigner(testSigningKey)
	s.now = func() time.Time { return now }
	return s
}

func TestSessionRoundTrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := newTestSigner(now)
	for _, session := range []Session{
		{GitHubID: 42},
		{GitHubID: 0, GuestAppID: "exo-gui"},
	} {
		value, err := s.Session(session, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		got, ok := s.ReadSession(value)
		if !ok || got != session {
			t.Fatalf("ReadSession = %+v, %v; want %+v", got, ok, session)
		}
	}
}

func TestSessionExpires(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := newTestSigner(now)
	value, err := s.Session(Session{GitHubID: 42}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(time.Hour) }
	if _, ok := s.ReadSession(value); ok {
		t.Fatal("an expired session was accepted")
	}
}

func TestSessionRefusals(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := newTestSigner(now)
	sign := func(method jwt.SigningMethod, key any, claims jwt.Claims) string {
		t.Helper()
		value, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	valid := func() sessionClaims {
		return sessionClaims{RegisteredClaims: s.registered("42", sessionAudience, now.Add(time.Hour))}
	}

	state, err := s.State("random", "http://localhost:5173")
	if err != nil {
		t.Fatal(err)
	}
	other := valid()
	other.Issuer = "someone-else"
	noExpiry := valid()
	noExpiry.ExpiresAt = nil
	future := valid()
	future.IssuedAt = jwt.NewNumericDate(now.Add(time.Hour))
	guestWithoutApp := valid()
	guestWithoutApp.Subject = "0"
	memberWithApp := valid()
	memberWithApp.GuestApp = "exo-gui"
	notANumber := valid()
	notANumber.Subject = "042"

	for name, value := range map[string]string{
		"a state cookie":            state,
		"another key":               sign(jwt.SigningMethodHS256, []byte("another-key-that-is-at-least-32-bytes"), valid()),
		"another algorithm":         sign(jwt.SigningMethodHS512, testSigningKey, valid()),
		"no algorithm":              sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid()),
		"another issuer":            sign(jwt.SigningMethodHS256, testSigningKey, other),
		"no expiry":                 sign(jwt.SigningMethodHS256, testSigningKey, noExpiry),
		"issued in the future":      sign(jwt.SigningMethodHS256, testSigningKey, future),
		"a guest without a tool":    sign(jwt.SigningMethodHS256, testSigningKey, guestWithoutApp),
		"an operator with a tool":   sign(jwt.SigningMethodHS256, testSigningKey, memberWithApp),
		"a subject that is unusual": sign(jwt.SigningMethodHS256, testSigningKey, notANumber),
		"an old opaque session":     "Zm9vYmFyYmF6cXV4cXV1eGNvcmdlZ3JhdWx0Z2FycGx5",
		"empty":                     "",
	} {
		if _, ok := s.ReadSession(value); ok {
			t.Errorf("%s was accepted as a session", name)
		}
	}
}

func TestStateRoundTripAndAudience(t *testing.T) {
	s := newTestSigner(time.Unix(1_800_000_000, 0))
	value, err := s.State("random", "http://localhost:5174")
	if err != nil {
		t.Fatal(err)
	}
	state, returnTo, ok := s.ReadState(value)
	if !ok || state != "random" || returnTo != "http://localhost:5174" {
		t.Fatalf("ReadState = %q, %q, %v", state, returnTo, ok)
	}
	session, err := s.Session(Session{GitHubID: 42}, s.now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := s.ReadState(session); ok {
		t.Fatal("a session cookie was accepted as a state cookie")
	}
	s.now = func() time.Time { return time.Unix(1_800_000_000, 0).Add(stateLifetime) }
	if _, _, ok := s.ReadState(value); ok {
		t.Fatal("an expired state was accepted")
	}
	if strings.Count(value, ".") != 2 {
		t.Fatalf("state cookie is not a JWT: %q", value)
	}
}
