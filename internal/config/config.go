package config

import (
	"errors"
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string
	HTTPAddr string
}


func Load() (Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8000"
	}
	return Config{
		DatabaseURL: dbURL,
		HTTPAddr: addr,
	}, nil
}

func (c Config) String() string {
	return fmt.Sprintf("Config{HTTPAddr: %s, DatabaseURL: exists}", c.HTTPAddr)
}