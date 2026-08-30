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

type Component struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HealthURL string `json:"health_url"`
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
		for _, component := range app.Components {
			if component.ID == "" || component.Name == "" || component.HealthURL == "" {
				return nil, fmt.Errorf("application %q has an incomplete component", app.ID)
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
