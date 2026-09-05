package monitor

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

type Store interface {
	SetLatestHealth(context.Context, model.Health) error
	InsertHealth(context.Context, model.Health) error
	// PingPostgres and PingRedis probe the shared infrastructure through the
	// connections Logger already holds, so the database and cache can sit in
	// the catalog without the monitor learning their wire protocols or being
	// handed a second set of credentials.
	PingPostgres(context.Context) error
	PingRedis(context.Context) error
}

type Monitor struct {
	store           Store
	catalog         *catalog.Catalog
	client          *http.Client
	timeout         time.Duration
	interval        time.Duration
	historyInterval time.Duration
	last            map[string]persistedState
}

type persistedState struct {
	ok          bool
	persistedAt time.Time
}

func New(store Store, serviceCatalog *catalog.Catalog, interval, historyInterval, timeout time.Duration) *Monitor {
	return &Monitor{
		store:           store,
		catalog:         serviceCatalog,
		client:          &http.Client{Timeout: timeout},
		timeout:         timeout,
		interval:        interval,
		historyInterval: historyInterval,
		last:            make(map[string]persistedState),
	}
}

func (m *Monitor) Run(ctx context.Context) {
	m.poll(ctx)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *Monitor) poll(ctx context.Context) {
	type result struct {
		health model.Health
	}

	components := make([]catalog.Component, 0)
	for _, app := range m.catalog.Applications() {
		components = append(components, app.Components...)
	}
	results := make(chan result, len(components))
	var wait sync.WaitGroup
	for _, component := range components {
		component := component
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- result{health: m.probe(ctx, component)}
		}()
	}
	wait.Wait()
	close(results)

	for result := range results {
		health := result.health
		if err := m.store.SetLatestHealth(ctx, health); err != nil {
			log.Printf("cache health for %s: %v", health.Service, err)
		}

		previous, exists := m.last[health.Service]
		shouldPersist := !exists || previous.ok != health.OK || health.CheckedAt.Sub(previous.persistedAt) >= m.historyInterval
		if shouldPersist {
			if err := m.store.InsertHealth(ctx, health); err != nil {
				log.Printf("persist health for %s: %v", health.Service, err)
				continue
			}
			m.last[health.Service] = persistedState{ok: health.OK, persistedAt: health.CheckedAt}
		}
	}
}

func (m *Monitor) probe(ctx context.Context, component catalog.Component) model.Health {
	switch component.Check {
	case catalog.CheckPostgres:
		return m.probeDependency(ctx, component, m.store.PingPostgres)
	case catalog.CheckRedis:
		return m.probeDependency(ctx, component, m.store.PingRedis)
	default:
		return m.probeHTTP(ctx, component)
	}
}

// probeDependency judges a shared dependency by whether Logger's own connection
// to it answers a ping inside the health timeout. The detail keeps the same
// "in Nms" shape as an HTTP probe so the explorer reads the same either way.
func (m *Monitor) probeDependency(ctx context.Context, component catalog.Component, ping func(context.Context) error) model.Health {
	started := time.Now()
	health := model.Health{Service: component.ID, CheckedAt: started}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	if err := ping(ctx); err != nil {
		health.Detail = err.Error()
		return health
	}
	health.OK = true
	health.Detail = fmt.Sprintf("%s ping in %s", component.Check, time.Since(started).Round(time.Millisecond))
	return health
}

func (m *Monitor) probeHTTP(ctx context.Context, component catalog.Component) model.Health {
	started := time.Now()
	health := model.Health{Service: component.ID, CheckedAt: started}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, component.HealthURL, nil)
	if err != nil {
		health.Detail = err.Error()
		return health
	}
	response, err := m.client.Do(req)
	if err != nil {
		health.Detail = err.Error()
		return health
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()

	elapsed := time.Since(started).Round(time.Millisecond)
	health.OK = response.StatusCode >= 200 && response.StatusCode < 400
	health.Detail = fmt.Sprintf("HTTP %d in %s", response.StatusCode, elapsed)
	return health
}
