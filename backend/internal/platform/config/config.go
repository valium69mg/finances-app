// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const defaultHTTPAddr = ":8080"

// Config holds the runtime configuration of the API.
type Config struct {
	DatabaseURL string
	HTTPAddr    string
}

// Load reads the configuration using getenv (typically os.Getenv) and fails fast
// on missing or invalid values, reporting every problem at once.
func Load(getenv func(string) string) (Config, error) {
	var errs []error

	cfg := Config{
		DatabaseURL: strings.TrimSpace(getenv("DATABASE_URL")),
		HTTPAddr:    strings.TrimSpace(getenv("HTTP_ADDR")),
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	} else if err := validateDatabaseURL(cfg.DatabaseURL); err != nil {
		errs = append(errs, fmt.Errorf("DATABASE_URL is invalid: %w", err))
	}

	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	} else if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		errs = append(errs, fmt.Errorf("HTTP_ADDR is invalid: %w", err))
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		// url.Error echoes the raw input, which may contain the password.
		return errors.New("not a parsable URL")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return errors.New(`scheme must be "postgres" or "postgresql"`)
	}
	if u.Host == "" {
		return errors.New("host is required")
	}
	return nil
}
