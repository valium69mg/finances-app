// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	defaultHTTPAddr    = ":8080"
	defaultResendFrom  = "onboarding@resend.dev"
	defaultAppBaseURL  = "http://localhost:5173"
	minJWTSecretLength = 32
)

// Config holds the runtime configuration of the API.
type Config struct {
	DatabaseURL  string
	HTTPAddr     string
	JWTSecret    string
	ResendAPIKey string
	ResendFrom   string
	// AppBaseURL is the frontend origin used to build links in emails (no trailing slash).
	AppBaseURL string
}

// Load reads the configuration using getenv (typically os.Getenv) and fails fast
// on missing or invalid values, reporting every problem at once.
func Load(getenv func(string) string) (Config, error) {
	var errs []error

	cfg := Config{
		DatabaseURL: strings.TrimSpace(getenv("DATABASE_URL")),
		HTTPAddr:    strings.TrimSpace(getenv("HTTP_ADDR")),

		JWTSecret:    strings.TrimSpace(getenv("JWT_SECRET")),
		ResendAPIKey: strings.TrimSpace(getenv("RESEND_API_KEY")),
		ResendFrom:   strings.TrimSpace(getenv("RESEND_FROM")),
		AppBaseURL:   strings.TrimRight(strings.TrimSpace(getenv("APP_BASE_URL")), "/"),
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

	// Errors below name the variable and the rule, never the value: they are secrets.
	if cfg.JWTSecret == "" {
		errs = append(errs, errors.New("JWT_SECRET is required"))
	} else if len(cfg.JWTSecret) < minJWTSecretLength {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d bytes", minJWTSecretLength))
	}

	if cfg.ResendAPIKey == "" {
		errs = append(errs, errors.New("RESEND_API_KEY is required"))
	}

	if cfg.ResendFrom == "" {
		cfg.ResendFrom = defaultResendFrom
	}

	if cfg.AppBaseURL == "" {
		cfg.AppBaseURL = defaultAppBaseURL
	} else if err := validateBaseURL(cfg.AppBaseURL); err != nil {
		errs = append(errs, fmt.Errorf("APP_BASE_URL is invalid: %w", err))
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

func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("not a parsable URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New(`scheme must be "http" or "https"`)
	}
	if u.Host == "" {
		return errors.New("host is required")
	}
	return nil
}
