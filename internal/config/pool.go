package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmofactory/recetas-backend/internal/platform/database"
)

const (
	maxConnsVar = "DB_MAX_CONNS"
	minConnsVar = "DB_MIN_CONNS"

	// maxConnsLimit keeps one pool well inside Postgres' default
	// max_connections of 100.
	maxConnsLimit = 100
	minMaxConns   = 1
	minMinConns   = 0
)

type connsBounds struct {
	lowest, highest int32
}

// parsePool reads DB_MAX_CONNS and then DB_MIN_CONNS, which may not exceed it.
func parsePool(getenv func(string) string) (database.PoolSize, error) {
	defaults := database.DefaultPoolSize()
	maxConns, err := parseConns(getenv, maxConnsVar, defaults.MaxConns, connsBounds{minMaxConns, maxConnsLimit})
	if err != nil {
		return database.PoolSize{}, err
	}
	minConns, err := parseConns(getenv, minConnsVar, defaults.MinConns, connsBounds{minMinConns, maxConns})
	if err != nil {
		return database.PoolSize{}, err
	}
	return database.PoolSize{MaxConns: maxConns, MinConns: minConns}, nil
}

// parseConns returns fallback for a blank value.
func parseConns(getenv func(string) string, name string, fallback int32, bounds connsBounds) (int32, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < int64(bounds.lowest) || n > int64(bounds.highest) {
		return 0, fmt.Errorf("%s %q is invalid (want an integer from %d to %d)", name, raw, bounds.lowest, bounds.highest)
	}
	return int32(n), nil
}
