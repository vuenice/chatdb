package services

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolManager struct {
	mu    sync.Mutex
	pools map[string]*pgxpool.Pool
}

func NewPoolManager() *PoolManager {
	return &PoolManager{pools: make(map[string]*pgxpool.Pool)}
}

func poolKey(connectionID uint, read bool) string {
	if read {
		return fmt.Sprintf("%d-read", connectionID)
	}
	return fmt.Sprintf("%d-write", connectionID)
}

func (m *PoolManager) GetOrCreate(ctx context.Context, connectionID uint, read bool, host string, port int, database, user, password, sslmode string) (*pgxpool.Pool, error) {
	key := poolKey(connectionID, read)
	m.mu.Lock()
	if p, ok := m.pools[key]; ok {
		m.mu.Unlock()
		return p, nil
	}
	m.mu.Unlock()

	if sslmode == "" {
		sslmode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + database,
	}
	q := u.Query()
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()

	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "60000"
	cfg.MaxConns = 8
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "60000" // ms; runner also uses context

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.pools[key]; ok {
		pool.Close()
		return existing, nil
	}
	m.pools[key] = pool
	return pool, nil
}

func (m *PoolManager) Invalidate(connectionID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, read := range []bool{true, false} {
		key := poolKey(connectionID, read)
		if p, ok := m.pools[key]; ok {
			p.Close()
			delete(m.pools, key)
		}
	}
}
