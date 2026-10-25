// Package telemetry sets up OpenTelemetry traces and logs exported over
// OTLP/HTTP (Logfire). It never touches the OTel globals: callers inject the
// returned providers.
package telemetry

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

const httpsScheme = "https"

var errEndpoint = errors.New("must be an https URL with a host")

// Config selects where telemetry goes. A blank Token disables it.
type Config struct {
	Token          string
	Endpoint       string
	ServiceName    string
	ServiceVersion string
	Environment    string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Token) != ""
}

// String never renders the token.
func (c Config) String() string {
	return fmt.Sprintf("telemetry.Config{Enabled: %t, Endpoint: %s, ServiceName: %s, ServiceVersion: %s, Environment: %s}",
		c.Enabled(), c.Endpoint, c.ServiceName, c.ServiceVersion, c.Environment)
}

// LogValue never renders the token.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("enabled", c.Enabled()),
		slog.String("endpoint", c.Endpoint),
		slog.String("service_name", c.ServiceName),
		slog.String("service_version", c.ServiceVersion),
		slog.String("environment", c.Environment),
	)
}

// ValidateEndpoint accepts only an https URL with a host. The error never
// echoes the input.
func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != httpsScheme || u.Host == "" {
		return errEndpoint
	}
	return nil
}
