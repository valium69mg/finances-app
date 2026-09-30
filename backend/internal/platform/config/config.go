// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultHTTPAddr    = ":8080"
	defaultResendFrom  = "onboarding@resend.dev"
	defaultAppBaseURL  = "http://localhost:5173"
	defaultS3Endpoint  = "localhost:9100" // MinIO S3 API host port in docker-compose
	defaultS3Bucket    = "finances-invoices"
	defaultS3Region    = "us-east-1"
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
	// S3 is the S3-compatible object storage of the issued CFDI files.
	S3 S3
}

// S3 configures the object storage. Endpoint is host:port without a scheme.
type S3 struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
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

	s3, s3Errs := loadS3(getenv)
	cfg.S3 = s3
	errs = append(errs, s3Errs...)

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadS3 reads the storage settings. The credentials default to the MinIO root
// credentials of docker-compose (MINIO_ROOT_USER, MINIO_ROOT_PASSWORD), so a
// local .env needs no extra keys; a real deployment sets S3_ACCESS_KEY and
// S3_SECRET_KEY for a dedicated identity. When neither pair is set the storage
// is left unconfigured (S3.Configured is false) instead of failing the load.
func loadS3(getenv func(string) string) (S3, []error) {
	var errs []error
	get := func(key string) string { return strings.TrimSpace(getenv(key)) }
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := get(k); v != "" {
				return v
			}
		}
		return ""
	}
	s3 := S3{
		Endpoint:  first("S3_ENDPOINT"),
		AccessKey: first("S3_ACCESS_KEY", "MINIO_ROOT_USER"),
		SecretKey: first("S3_SECRET_KEY", "MINIO_ROOT_PASSWORD"),
		Bucket:    first("S3_BUCKET"),
		Region:    first("S3_REGION"),
	}
	if s3.Endpoint == "" {
		s3.Endpoint = defaultS3Endpoint
	} else if strings.Contains(s3.Endpoint, "://") || strings.Contains(s3.Endpoint, "/") {
		errs = append(errs, errors.New("S3_ENDPOINT is invalid: use host:port without a scheme or path"))
	}
	if s3.Bucket == "" {
		s3.Bucket = defaultS3Bucket
	}
	if s3.Region == "" {
		s3.Region = defaultS3Region
	}
	if raw := get("S3_USE_SSL"); raw != "" {
		ssl, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, errors.New("S3_USE_SSL is invalid: use true or false"))
		}
		s3.UseSSL = ssl
	}
	// Missing credentials are not an error: the object storage is optional at
	// boot (see S3.Configured) so a missing or unreachable MinIO never takes the
	// whole API down; only the invoice file routes answer 503.
	return s3, errs
}

// Configured reports whether the credentials needed to talk to the object
// storage are set. Without them the API still starts and the file routes
// answer 503 storage_unavailable.
func (s S3) Configured() bool { return s.AccessKey != "" && s.SecretKey != "" }

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
