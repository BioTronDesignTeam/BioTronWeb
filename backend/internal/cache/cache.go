package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

const grantsKeyPrefix = "grants:"

// Cache wraps Redis for grant-set lookups only.
// Session / role flags (superuser, manager, banned) always come from Postgres so
// privilege changes are never sticky in Redis.
type Cache struct {
	rdb *redis.Client
	st  *store.Store
	ttl time.Duration
}

func New(ctx context.Context, redisURL string, st *store.Store, ttl time.Duration) (*Cache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Cache{rdb: rdb, st: st, ttl: ttl}, nil
}

func (c *Cache) Close() error {
	return c.rdb.Close()
}

func grantsKey(operatorID int64) string {
	return fmt.Sprintf("%s%d", grantsKeyPrefix, operatorID)
}

// GetSessionOperator always reads Postgres (roles must not be Redis-cached).
func (c *Cache) GetSessionOperator(ctx context.Context, idHash string) (*store.SessionOperator, error) {
	return c.st.GetSessionOperator(ctx, idHash)
}

func (c *Cache) InvalidateSession(ctx context.Context, idHash string) error {
	// No session cache; kept so call sites stay stable.
	return nil
}

// ListGrantsForOperator returns cached grants or loads from Postgres.
func (c *Cache) ListGrantsForOperator(ctx context.Context, operatorID int64) ([]store.Grant, error) {
	raw, err := c.rdb.Get(ctx, grantsKey(operatorID)).Bytes()
	if err == nil {
		var grants []store.Grant
		if err := json.Unmarshal(raw, &grants); err == nil {
			return grants, nil
		}
		_ = c.rdb.Del(ctx, grantsKey(operatorID)).Err()
	} else if !errors.Is(err, redis.Nil) {
		// degrade to Postgres
	}

	grants, err := c.st.ListGrantsForOperator(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	if grants == nil {
		grants = []store.Grant{}
	}
	if b, err := json.Marshal(grants); err == nil {
		_ = c.rdb.Set(ctx, grantsKey(operatorID), b, c.ttl).Err()
	}
	return grants, nil
}

func (c *Cache) InvalidateGrants(ctx context.Context, operatorID int64) error {
	return c.rdb.Del(ctx, grantsKey(operatorID)).Err()
}
