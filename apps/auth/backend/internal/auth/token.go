package auth

// The session cookie and the state cookie of a sign-in are JWTs signed with
// HMAC-SHA256 and SESSION_SIGNING_KEY. There is no session store: the
// signature is the check. A session names only the operator's GitHub id (and,
// for a guest, the tool whose key it used); roles, bans, and grants are read
// on every request through the cache, so a JWT never carries a privilege
// that could outlive its revocation.
//
// The two kinds have their own audience, so one can never pass as the other.
// Only HS256 is accepted, and the issuer, the audience, and the expiry are
// always checked. A new SESSION_SIGNING_KEY signs everyone out.
//
// Never log or return a cookie value or the key.

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenIssuer     = "biotron-auth"
	sessionAudience = "biotron-auth:session"
	stateAudience   = "biotron-auth:oauth-state"
	// stateLifetime is how long one round trip to GitHub may take.
	stateLifetime = 10 * time.Minute
)

// Signer signs and reads both cookies with one key. Its methods are safe for
// use by many goroutines.
type Signer struct {
	key []byte
	now func() time.Time
}

func NewSigner(key []byte) *Signer {
	return &Signer{key: key, now: time.Now}
}

// Session is what a session cookie holds.
type Session struct {
	GitHubID int64
	// GuestAppID is the tool of a guest session; empty for an operator.
	GuestAppID string
}

type sessionClaims struct {
	GuestApp string `json:"guest_app,omitempty"`
	jwt.RegisteredClaims
}

type stateClaims struct {
	State  string `json:"state"`
	Return string `json:"return"`
	jwt.RegisteredClaims
}

// Session returns a session cookie value that ends at expires.
func (s *Signer) Session(session Session, expires time.Time) (string, error) {
	return s.sign(&sessionClaims{
		GuestApp:         session.GuestAppID,
		RegisteredClaims: s.registered(strconv.FormatInt(session.GitHubID, 10), sessionAudience, expires),
	})
}

// ReadSession returns the session of a cookie value; ok is false when it is
// not one this key signed, or has expired. A guest session must name its
// tool, and an operator's must not.
func (s *Signer) ReadSession(value string) (Session, bool) {
	var claims sessionClaims
	if !s.read(value, sessionAudience, &claims) {
		return Session{}, false
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id < 0 || strconv.FormatInt(id, 10) != claims.Subject {
		return Session{}, false
	}
	if (id == 0) != (claims.GuestApp != "") {
		return Session{}, false
	}
	return Session{GitHubID: id, GuestAppID: claims.GuestApp}, true
}

// State returns a state cookie value for one sign-in: the random state and
// the origin to return to.
func (s *Signer) State(state, returnTo string) (string, error) {
	return s.sign(&stateClaims{
		State:            state,
		Return:           returnTo,
		RegisteredClaims: s.registered("", stateAudience, s.now().Add(stateLifetime)),
	})
}

// ReadState returns what a state cookie holds; ok is false when it is not
// valid or has expired.
func (s *Signer) ReadState(value string) (state, returnTo string, ok bool) {
	var claims stateClaims
	if !s.read(value, stateAudience, &claims) || claims.State == "" {
		return "", "", false
	}
	return claims.State, claims.Return, true
}

func (s *Signer) registered(subject, audience string, expires time.Time) jwt.RegisteredClaims {
	now := s.now()
	return jwt.RegisteredClaims{
		Issuer:    tokenIssuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
	}
}

func (s *Signer) sign(claims jwt.Claims) (string, error) {
	if s == nil || len(s.key) == 0 {
		return "", errors.New("no session signing key")
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
	if err != nil {
		return "", errors.New("cannot sign a cookie")
	}
	return signed, nil
}

func (s *Signer) read(value, audience string, claims jwt.Claims) bool {
	if s == nil || len(s.key) == 0 || value == "" {
		return false
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(s.now),
	)
	_, err := parser.ParseWithClaims(value, claims, func(*jwt.Token) (any, error) {
		return s.key, nil
	})
	return err == nil
}
