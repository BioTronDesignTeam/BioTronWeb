package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/cache"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// fakeGitHubMembers answers membership checks from a map and counts them;
// set changes the answer for octo.
type fakeGitHubMembers struct {
	mu      sync.Mutex
	members map[string]bool
	err     error
	calls   int
}

func (f *fakeGitHubMembers) IsOrgMember(_ context.Context, login string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.members[login], f.err
}

func (f *fakeGitHubMembers) set(member bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members["octo"] = member
	f.err = err
}

// fakeOperators is the store behind the cache: every id but the guest's is
// the operator octo, and none is banned.
type fakeOperators struct{}

func (fakeOperators) GetOperator(_ context.Context, githubID int64) (*store.OrgMember, error) {
	login := "guest"
	if githubID != store.GuestGitHubID {
		login = "octo"
	}
	return &store.OrgMember{GitHubID: githubID, Login: login}, nil
}

func (fakeOperators) ListGrantsForOperator(context.Context, int64) ([]store.Grant, error) {
	return nil, nil
}

func TestMembershipKeepsAnAnswerForItsTTL(t *testing.T) {
	github := &fakeGitHubMembers{members: map[string]bool{"octo": true}}
	m := NewMembership(github.IsOrgMember)
	now := time.Unix(1_000_000, 0)
	m.now = func() time.Time { return now }
	ctx := context.Background()

	for range 3 {
		if member, err := m.Member(ctx, 42, "octo"); err != nil || !member {
			t.Fatalf("member = %v, err = %v", member, err)
		}
	}
	if github.calls != 1 {
		t.Fatalf("GitHub asked %d times within the TTL, want once", github.calls)
	}

	// Removed from the organization: the old answer holds until the TTL
	// ends, and not a moment longer.
	github.set(false, nil)
	now = now.Add(MembershipTTL - time.Second)
	if member, _ := m.Member(ctx, 42, "octo"); !member {
		t.Fatal("the kept answer was dropped before its TTL")
	}
	now = now.Add(time.Second)
	if member, err := m.Member(ctx, 42, "octo"); err != nil || member {
		t.Fatalf("after the TTL: member = %v, err = %v, want a refusal", member, err)
	}
}

// When GitHub cannot answer, an answer older than the TTL is never used: the
// check fails closed rather than stretch a removed member's access.
func TestMembershipFailsClosedWhenGitHubCannotAnswer(t *testing.T) {
	github := &fakeGitHubMembers{members: map[string]bool{"octo": true}}
	m := NewMembership(github.IsOrgMember)
	now := time.Unix(1_000_000, 0)
	m.now = func() time.Time { return now }
	ctx := context.Background()

	if member, err := m.Member(ctx, 42, "octo"); err != nil || !member {
		t.Fatalf("member = %v, err = %v", member, err)
	}
	github.set(true, errors.New("github unreachable"))
	now = now.Add(time.Minute)
	if member, err := m.Member(ctx, 42, "octo"); err != nil || !member {
		t.Fatalf("a fresh answer during an outage: member = %v, err = %v", member, err)
	}
	now = now.Add(MembershipTTL)
	if _, err := m.Member(ctx, 42, "octo"); err == nil {
		t.Fatal("an answer older than the TTL was used during an outage")
	}
	// A failed check is not kept: the next request asks again.
	github.set(true, nil)
	if member, err := m.Member(ctx, 42, "octo"); err != nil || !member {
		t.Fatalf("after the outage: member = %v, err = %v", member, err)
	}
}

func TestMembershipKeepsTheSignInAnswer(t *testing.T) {
	github := &fakeGitHubMembers{members: map[string]bool{}}
	m := NewMembership(github.IsOrgMember)
	m.Keep(42, true)
	if member, err := m.Member(context.Background(), 42, "octo"); err != nil || !member || github.calls != 0 {
		t.Fatalf("member = %v, err = %v, calls = %d, want the sign-in answer", member, err, github.calls)
	}
}

func TestMembershipSharesOneCheckPerOperator(t *testing.T) {
	gate := make(chan struct{})
	var calls int
	var mu sync.Mutex
	m := NewMembership(func(context.Context, string) (bool, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-gate
		return true, nil
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if member, err := m.Member(context.Background(), 42, "octo"); err != nil || !member {
				t.Errorf("member = %v, err = %v", member, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	if calls != 1 {
		t.Fatalf("GitHub asked %d times for one operator, want once", calls)
	}
}

// RequireSession applies the membership check to every route behind it,
// /v1/check included, and skips it for a guest, who has no GitHub identity.
func TestRequireSessionRechecksMembership(t *testing.T) {
	github := &fakeGitHubMembers{members: map[string]bool{"octo": true}}
	h := &Handler{
		Cache:   cache.New(fakeOperators{}),
		Members: NewMembership(github.IsOrgMember),
		Signer:  NewSigner(testSigningKey),
	}
	operator, err := h.Signer.Session(Session{GitHubID: 42}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	check := func(cookie string) int {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/v1/check?app=logger&permission=view", nil)
		request.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		response := httptest.NewRecorder()
		h.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(response, request)
		return response.Code
	}

	if got := check(operator); got != http.StatusOK {
		t.Fatalf("a member = %d, want 200", got)
	}

	h.Members = NewMembership(github.IsOrgMember)
	github.set(false, nil)
	if got := check(operator); got != http.StatusUnauthorized {
		t.Fatalf("a removed member = %d, want 401", got)
	}

	h.Members = NewMembership(github.IsOrgMember)
	github.set(true, errors.New("github unreachable"))
	if got := check(operator); got != http.StatusServiceUnavailable {
		t.Fatalf("GitHub unreachable = %d, want 503", got)
	}

	// The guest never reaches GitHub; it has no login to ask about.
	calls := github.calls
	guest := guestSession(t, h)
	h.RequireSession(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(httptest.NewRecorder(), guest)
	if github.calls != calls {
		t.Fatal("a guest session asked GitHub for a membership")
	}
}

// guestSession returns a request carrying a guest session of Exo.
func guestSession(t *testing.T, h *Handler) *http.Request {
	const appID = "exo-gui"
	t.Helper()
	value, err := h.Signer.Session(Session{GitHubID: store.GuestGitHubID, GuestAppID: appID}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/check?app="+appID+"&permission=live", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: value})
	return request
}
