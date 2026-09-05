package catalog

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

//go:embed default.json
var defaultJSON []byte

// Check names describe how the monitor probes a component.
const (
	// CheckHTTP is the default: an HTTP GET of HealthURL answering 2xx or 3xx.
	CheckHTTP = "http"
	// CheckPostgres pings the shared database through Logger's own pool.
	CheckPostgres = "postgres"
	// CheckRedis pings the shared cache through Logger's own client.
	CheckRedis = "redis"
)

type Component struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Check is one of the Check constants; empty means CheckHTTP. The shared
	// database and cache speak no HTTP, so they are pinged over the connections
	// Logger already holds rather than given a URL that would have to carry
	// credentials.
	Check     string `json:"check,omitempty"`
	HealthURL string `json:"health_url,omitempty"`
}

type Application struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Components  []Component `json:"components"`
}

type Catalog struct {
	applications []Application
	byID         map[string]Application
	services     map[string]struct{}
}

func Load(override string) (*Catalog, error) {
	raw := defaultJSON
	if strings.TrimSpace(override) != "" {
		raw = []byte(override)
	}

	var applications []Application
	if err := json.Unmarshal(raw, &applications); err != nil {
		return nil, fmt.Errorf("parse service catalog: %w", err)
	}
	if len(applications) == 0 {
		return nil, errors.New("service catalog must contain at least one application")
	}

	c := &Catalog{
		applications: applications,
		byID:         make(map[string]Application, len(applications)),
		services:     make(map[string]struct{}),
	}
	for _, app := range applications {
		if app.ID == "" || app.Name == "" || len(app.Components) == 0 {
			return nil, errors.New("every application needs an id, name, and component")
		}
		if _, exists := c.byID[app.ID]; exists {
			return nil, fmt.Errorf("duplicate application id %q", app.ID)
		}
		for i := range app.Components {
			component := &app.Components[i]
			if component.ID == "" || component.Name == "" {
				return nil, fmt.Errorf("application %q has an incomplete component", app.ID)
			}
			if err := normaliseCheck(component); err != nil {
				return nil, fmt.Errorf("application %q: %w", app.ID, err)
			}
			if _, exists := c.services[component.ID]; exists {
				return nil, fmt.Errorf("duplicate component id %q", component.ID)
			}
			c.services[component.ID] = struct{}{}
		}
		c.byID[app.ID] = app
	}
	return c, nil
}

// normaliseCheck fills in the default check and refuses the combinations that
// would silently probe nothing: an HTTP check without a URL, or a URL on a
// dependency check where it could never be used.
func normaliseCheck(component *Component) error {
	switch component.Check {
	case "", CheckHTTP:
		component.Check = CheckHTTP
		if component.HealthURL == "" {
			return fmt.Errorf("component %q needs a health_url", component.ID)
		}
	case CheckPostgres, CheckRedis:
		if component.HealthURL != "" {
			return fmt.Errorf("component %q is a %s check and takes no health_url", component.ID, component.Check)
		}
	default:
		return fmt.Errorf("component %q has unknown check %q", component.ID, component.Check)
	}
	return nil
}

func (c *Catalog) Applications() []Application {
	return append([]Application(nil), c.applications...)
}

func (c *Catalog) Application(id string) (Application, bool) {
	app, ok := c.byID[id]
	return app, ok
}

func (c *Catalog) HasService(id string) bool {
	_, ok := c.services[id]
	return ok
}

func ServiceIDs(app Application) []string {
	ids := make([]string, 0, len(app.Components))
	for _, component := range app.Components {
		ids = append(ids, component.ID)
	}
	return ids
}
