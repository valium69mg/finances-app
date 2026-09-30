// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed the zone database: the distroless image ships none

	"github.com/valium69mg/finances-app/backend/internal/platform/clientip"
)

const (
	defaultHTTPAddr    = ":8080"
	defaultResendFrom  = "onboarding@resend.dev"
	defaultAppBaseURL  = "http://localhost:5173"
	defaultS3Endpoint  = "localhost:9100" // MinIO S3 API host port in docker-compose
	defaultS3Bucket    = "finances-invoices"
	defaultS3Region    = "us-east-1"
	minJWTSecretLength = 32

	defaultTZName        = "America/Mexico_City"
	defaultDiskAlertPct  = 80
	defaultDiskProbePath = "/probe"
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
	// TrustedProxies are the peers (CIDRs or IPs) whose X-Real-IP header is
	// believed as the client address. Empty (the default) trusts nothing.
	TrustedProxies []netip.Prefix
	// Production is true when APP_ENV=production. It turns on the stricter startup
	// checks (an explicit https APP_BASE_URL).
	Production bool
	// LogJSON selects JSON log lines (LOG_FORMAT=json) instead of text.
	LogJSON bool
	// RemindersEnabled turns the in-process email reminders on (REMINDERS_ENABLED,
	// default true).
	RemindersEnabled bool
	// TZName is the IANA zone (TZ_NAME) that defines "today" for due dates and
	// reminders. It is validated at load time.
	TZName string
	// DiskAlertPct is the used-space percentage (1..99) of the probed volume that
	// triggers the disk alert email.
	DiskAlertPct int
	// DiskProbePath is the directory whose filesystem is measured for the alert.
	DiskProbePath string
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

	if isPlaceholder(cfg.ResendAPIKey) {
		errs = append(errs, errors.New("RESEND_API_KEY still holds a placeholder value"))
	}
	if isPlaceholder(cfg.JWTSecret) {
		errs = append(errs, errors.New("JWT_SECRET still holds a placeholder value"))
	}
	if isPlaceholder(cfg.DatabaseURL) {
		errs = append(errs, errors.New("DATABASE_URL still holds a placeholder value"))
	}

	trusted, err := clientip.ParseTrusted(getenv("TRUSTED_PROXIES"))
	if err != nil {
		errs = append(errs, fmt.Errorf("TRUSTED_PROXIES is invalid: %w", err))
	}
	cfg.TrustedProxies = trusted

	switch env := strings.ToLower(strings.TrimSpace(getenv("APP_ENV"))); env {
	case "", "development":
	case "production":
		cfg.Production = true
	default:
		errs = append(errs, errors.New(`APP_ENV is invalid: use "development" or "production"`))
	}
	switch format := strings.ToLower(strings.TrimSpace(getenv("LOG_FORMAT"))); format {
	case "", "text":
	case "json":
		cfg.LogJSON = true
	default:
		errs = append(errs, errors.New(`LOG_FORMAT is invalid: use "text" or "json"`))
	}

	if cfg.ResendFrom == "" {
		cfg.ResendFrom = defaultResendFrom
	}

	if cfg.AppBaseURL == "" {
		if cfg.Production {
			// The default is the local dev server: in production it would put
			// localhost links in the verification emails and in the CORS origin.
			errs = append(errs, errors.New("APP_BASE_URL is required in production"))
		}
		cfg.AppBaseURL = defaultAppBaseURL
	} else if err := validateBaseURL(cfg.AppBaseURL); err != nil {
		errs = append(errs, fmt.Errorf("APP_BASE_URL is invalid: %w", err))
	} else if cfg.Production && !strings.HasPrefix(cfg.AppBaseURL, "https://") {
		errs = append(errs, errors.New("APP_BASE_URL must use https in production"))
	}

	s3, s3Errs := loadS3(getenv)
	cfg.S3 = s3
	errs = append(errs, s3Errs...)

	errs = append(errs, loadReminders(getenv, &cfg)...)

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadReminders reads the timezone and reminder settings into cfg. The timezone
// applies even with the reminders off: it defines "today" for the bill flags.
func loadReminders(getenv func(string) string, cfg *Config) []error {
	var errs []error
	get := func(key string) string { return strings.TrimSpace(getenv(key)) }

	cfg.RemindersEnabled = true
	if raw := get("REMINDERS_ENABLED"); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, errors.New("REMINDERS_ENABLED is invalid: use true or false"))
		}
		cfg.RemindersEnabled = enabled
	}

	cfg.TZName = get("TZ_NAME")
	if cfg.TZName == "" {
		cfg.TZName = defaultTZName
	} else if _, err := time.LoadLocation(cfg.TZName); err != nil {
		errs = append(errs, errors.New("TZ_NAME is invalid: use an IANA zone such as America/Mexico_City"))
	}

	cfg.DiskAlertPct = defaultDiskAlertPct
	if raw := get("DISK_ALERT_PCT"); raw != "" {
		pct, err := strconv.Atoi(raw)
		if err != nil || pct < 1 || pct > 99 {
			errs = append(errs, errors.New("DISK_ALERT_PCT is invalid: use a whole number from 1 to 99"))
		} else {
			cfg.DiskAlertPct = pct
		}
	}

	cfg.DiskProbePath = get("DISK_PROBE_PATH")
	if cfg.DiskProbePath == "" {
		cfg.DiskProbePath = defaultDiskProbePath
	}
	return errs
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
	if isPlaceholder(s3.AccessKey) || isPlaceholder(s3.SecretKey) {
		errs = append(errs, errors.New("S3_ACCESS_KEY/S3_SECRET_KEY (or the MinIO root credentials) still hold a placeholder value"))
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

// placeholderMarker is what .env.prod.example puts in every secret. A value that
// still contains it was copied without being filled in, so the API refuses to
// start instead of running with a publicly known secret.
const placeholderMarker = "change_me"

func isPlaceholder(v string) bool {
	return strings.Contains(strings.ToLower(v), placeholderMarker)
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
