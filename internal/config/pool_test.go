package config_test

import (
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/config"
	"github.com/cosmofactory/recetas-backend/internal/platform/database"
)

// Config.Pool is a database.PoolSize read from DB_MAX_CONNS (default 10,
// 1..100) and DB_MIN_CONNS (default 1, 0..DB_MAX_CONNS). A blank value means
// the default; anything else that isn't a valid integer in range is an error
// naming the variable.

const (
	maxConnsVar = "DB_MAX_CONNS"
	minConnsVar = "DB_MIN_CONNS"
)

type poolCase struct {
	name     string
	env      map[string]string
	want     database.PoolSize
	errNames string // the variable the error must name; "" means no error
}

func TestFromEnv_PoolDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := config.FromEnv(envFrom(validEnv()))
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	want := database.PoolSize{MaxConns: 10, MinConns: 1}
	if cfg.Pool != want {
		t.Errorf("got Pool %+v, want %+v", cfg.Pool, want)
	}
}

func TestFromEnv_DBMaxConns(t *testing.T) {
	t.Parallel()
	runPoolCases(t, []poolCase{
		{name: "blank is the default", env: map[string]string{maxConnsVar: ""}, want: database.PoolSize{MaxConns: 10, MinConns: 1}},
		{name: "whitespace only is the default", env: map[string]string{maxConnsVar: "   "}, want: database.PoolSize{MaxConns: 10, MinConns: 1}},
		{name: "lower bound", env: map[string]string{maxConnsVar: "1"}, want: database.PoolSize{MaxConns: 1, MinConns: 1}},
		{name: "typical", env: map[string]string{maxConnsVar: "25"}, want: database.PoolSize{MaxConns: 25, MinConns: 1}},
		{name: "upper bound", env: map[string]string{maxConnsVar: "100"}, want: database.PoolSize{MaxConns: 100, MinConns: 1}},
		{name: "surrounding whitespace is trimmed", env: map[string]string{maxConnsVar: " 20 "}, want: database.PoolSize{MaxConns: 20, MinConns: 1}},
		{name: "zero", env: map[string]string{maxConnsVar: "0"}, errNames: maxConnsVar},
		{name: "negative", env: map[string]string{maxConnsVar: "-1"}, errNames: maxConnsVar},
		{name: "above the upper bound", env: map[string]string{maxConnsVar: "101"}, errNames: maxConnsVar},
		{name: "overflows int32", env: map[string]string{maxConnsVar: "4294967306"}, errNames: maxConnsVar},
		{name: "not a number", env: map[string]string{maxConnsVar: "ten"}, errNames: maxConnsVar},
		{name: "decimal", env: map[string]string{maxConnsVar: "10.5"}, errNames: maxConnsVar},
		{name: "exponent", env: map[string]string{maxConnsVar: "1e1"}, errNames: maxConnsVar},
		{name: "trailing junk", env: map[string]string{maxConnsVar: "10conns"}, errNames: maxConnsVar},
	})
}

func TestFromEnv_DBMinConns(t *testing.T) {
	t.Parallel()
	runPoolCases(t, []poolCase{
		{name: "blank is the default", env: map[string]string{minConnsVar: ""}, want: database.PoolSize{MaxConns: 10, MinConns: 1}},
		{name: "whitespace only is the default", env: map[string]string{minConnsVar: "  "}, want: database.PoolSize{MaxConns: 10, MinConns: 1}},
		{name: "zero is allowed", env: map[string]string{minConnsVar: "0"}, want: database.PoolSize{MaxConns: 10, MinConns: 0}},
		{name: "typical", env: map[string]string{minConnsVar: "3"}, want: database.PoolSize{MaxConns: 10, MinConns: 3}},
		{name: "equal to the default max", env: map[string]string{minConnsVar: "10"}, want: database.PoolSize{MaxConns: 10, MinConns: 10}},
		{name: "negative", env: map[string]string{minConnsVar: "-1"}, errNames: minConnsVar},
		{name: "not a number", env: map[string]string{minConnsVar: "one"}, errNames: minConnsVar},
		{name: "decimal", env: map[string]string{minConnsVar: "1.0"}, errNames: minConnsVar},
		{name: "above the default max", env: map[string]string{minConnsVar: "11"}, errNames: minConnsVar},
	})
}

func TestFromEnv_DBMinConnsAgainstMax(t *testing.T) {
	t.Parallel()
	runPoolCases(t, []poolCase{
		{name: "min equals max", env: map[string]string{maxConnsVar: "5", minConnsVar: "5"}, want: database.PoolSize{MaxConns: 5, MinConns: 5}},
		{name: "min below max", env: map[string]string{maxConnsVar: "50", minConnsVar: "4"}, want: database.PoolSize{MaxConns: 50, MinConns: 4}},
		{name: "zero min with max one", env: map[string]string{maxConnsVar: "1", minConnsVar: "0"}, want: database.PoolSize{MaxConns: 1, MinConns: 0}},
		{name: "default min fits max one", env: map[string]string{maxConnsVar: "1"}, want: database.PoolSize{MaxConns: 1, MinConns: 1}},
		{name: "min above max", env: map[string]string{maxConnsVar: "5", minConnsVar: "6"}, errNames: minConnsVar},
		{name: "min above max at the upper bound", env: map[string]string{maxConnsVar: "100", minConnsVar: "101"}, errNames: minConnsVar},
		{name: "invalid max is reported, not the comparison", env: map[string]string{maxConnsVar: "0", minConnsVar: "1"}, errNames: maxConnsVar},
	})
}

func runPoolCases(t *testing.T, cases []poolCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.FromEnv(envFrom(withRaw(tt.env)))
			if tt.errNames != "" {
				assertErrorNames(t, err, tt.errNames)
				return
			}
			if err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if cfg.Pool != tt.want {
				t.Errorf("got Pool %+v, want %+v", cfg.Pool, tt.want)
			}
		})
	}
}

// withRaw is like with, but keeps whitespace-only values instead of
// treating them as unset, so blank-handling is tested through FromEnv.
func withRaw(overrides map[string]string) map[string]string {
	m := validEnv()
	for k, v := range overrides {
		m[k] = v
	}
	return m
}

func assertErrorNames(t *testing.T, err error, variable string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want one naming %s", variable)
	}
	if !strings.Contains(err.Error(), variable) {
		t.Errorf("got error %q, want it to name %s", err, variable)
	}
}
