package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

func TestMeRequiresMatchingProductForGuest(t *testing.T) {
	h := &Handler{}
	app := fiber.New()
	app.Get("/me", func(c *fiber.Ctx) error {
		c.Locals(OperatorLocal, &store.SessionOperator{
			GitHubID:   store.GuestGitHubID,
			Login:      store.GuestLogin,
			GuestAppID: "exo-gui",
		})
		return h.Me(c)
	})

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/me?app=exo-gui", want: fiber.StatusOK},
		{path: "/me?app=calendar", want: fiber.StatusForbidden},
		{path: "/me", want: fiber.StatusForbidden},
	} {
		response, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		if response.StatusCode != tc.want {
			t.Errorf("GET %s status = %d, want %d", tc.path, response.StatusCode, tc.want)
		}
	}
}
