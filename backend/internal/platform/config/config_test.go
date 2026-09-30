package config_test

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/platform/config"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestLoad(t *testing.T) {
	const validURL = "postgres://u:p@localhost:5442/db?sslmode=disable"
	const secret = "0123456789abcdef0123456789abcdef"

	base := func(extra map[string]string) map[string]string {
		vars := map[string]string{
			"DATABASE_URL": validURL, "JWT_SECRET": secret, "RESEND_API_KEY": "re_key",
			"MINIO_ROOT_USER": "minio-user", "MINIO_ROOT_PASSWORD": "minio-password",
		}
		for k, v := range extra {
			vars[k] = v
		}
		return vars
	}
	want := func(mod func(*config.Config)) config.Config {
		c := config.Config{
			DatabaseURL: validURL, HTTPAddr: ":8080", JWTSecret: secret, ResendAPIKey: "re_key",
			ResendFrom: "onboarding@resend.dev", AppBaseURL: "http://localhost:5173",
			S3: config.S3{
				Endpoint: "localhost:9100", AccessKey: "minio-user", SecretKey: "minio-password",
				Bucket: "finances-invoices", Region: "us-east-1",
			},
		}
		mod(&c)
		return c
	}

	tests := []struct {
		name    string
		vars    map[string]string
		want    config.Config
		wantErr []string
	}{
		{
			name: "valid values",
			vars: base(map[string]string{
				"HTTP_ADDR": ":9090", "RESEND_FROM": "me@x.io", "APP_BASE_URL": "https://app.example.com/",
			}),
			want: want(func(c *config.Config) {
				c.HTTPAddr, c.ResendFrom, c.AppBaseURL = ":9090", "me@x.io", "https://app.example.com"
			}),
		},
		{
			name: "defaults",
			vars: base(nil),
			want: want(func(*config.Config) {}),
		},
		{
			// The object storage is optional at boot: without credentials the API
			// still loads and the file routes answer 503.
			name: "missing storage credentials still load",
			vars: base(map[string]string{"MINIO_ROOT_USER": "", "MINIO_ROOT_PASSWORD": ""}),
			want: want(func(c *config.Config) { c.S3.AccessKey, c.S3.SecretKey = "", "" }),
		},
		{
			name: "dedicated storage credentials win over the MinIO root ones",
			vars: base(map[string]string{"S3_ACCESS_KEY": "app-key", "S3_SECRET_KEY": "app-secret"}),
			want: want(func(c *config.Config) { c.S3.AccessKey, c.S3.SecretKey = "app-key", "app-secret" }),
		},
		{
			name:    "malformed storage endpoint is still an error",
			vars:    base(map[string]string{"S3_ENDPOINT": "http://localhost:9100"}),
			wantErr: []string{"S3_ENDPOINT is invalid"},
		},
		{
			name:    "missing database url",
			vars:    map[string]string{"JWT_SECRET": secret, "RESEND_API_KEY": "k"},
			wantErr: []string{"DATABASE_URL is required"},
		},
		{
			name:    "wrong scheme",
			vars:    base(map[string]string{"DATABASE_URL": "mysql://u:p@localhost/db"}),
			wantErr: []string{"DATABASE_URL is invalid", "scheme"},
		},
		{
			name:    "missing host",
			vars:    base(map[string]string{"DATABASE_URL": "postgres:///db"}),
			wantErr: []string{"DATABASE_URL is invalid", "host is required"},
		},
		{
			name:    "invalid http addr",
			vars:    base(map[string]string{"HTTP_ADDR": "not-an-addr"}),
			wantErr: []string{"HTTP_ADDR is invalid"},
		},
		{
			name:    "missing jwt secret",
			vars:    base(map[string]string{"JWT_SECRET": ""}),
			wantErr: []string{"JWT_SECRET is required"},
		},
		{
			name:    "short jwt secret",
			vars:    base(map[string]string{"JWT_SECRET": "too-short"}),
			wantErr: []string{"JWT_SECRET must be at least 32 bytes"},
		},
		{
			name:    "missing resend api key",
			vars:    base(map[string]string{"RESEND_API_KEY": ""}),
			wantErr: []string{"RESEND_API_KEY is required"},
		},
		{
			name: "trusted proxies",
			vars: base(map[string]string{"TRUSTED_PROXIES": "172.18.0.0/16, 10.0.0.5"}),
			want: want(func(c *config.Config) {
				c.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("172.18.0.0/16"), netip.MustParsePrefix("10.0.0.5/32")}
			}),
		},
		{
			name:    "invalid trusted proxies",
			vars:    base(map[string]string{"TRUSTED_PROXIES": "nope"}),
			wantErr: []string{"TRUSTED_PROXIES is invalid"},
		},
		{
			name:    "trusting every peer is rejected",
			vars:    base(map[string]string{"TRUSTED_PROXIES": "0.0.0.0/0"}),
			wantErr: []string{"TRUSTED_PROXIES is invalid", "every peer"},
		},
		{
			name:    "placeholder jwt secret",
			vars:    base(map[string]string{"JWT_SECRET": "CHANGE_ME_generate_with_openssl_rand_hex_32"}),
			wantErr: []string{"JWT_SECRET still holds a placeholder"},
		},
		{
			name:    "placeholder resend key and database password",
			vars:    base(map[string]string{"RESEND_API_KEY": "CHANGE_ME", "DATABASE_URL": "postgres://u:CHANGE_ME@db:5432/x"}),
			wantErr: []string{"RESEND_API_KEY still holds a placeholder", "DATABASE_URL still holds a placeholder"},
		},
		{
			name:    "placeholder storage credentials",
			vars:    base(map[string]string{"S3_ACCESS_KEY": "app", "S3_SECRET_KEY": "change_me_please"}),
			wantErr: []string{"still hold a placeholder"},
		},
		{
			name: "production with an https base url",
			vars: base(map[string]string{"APP_ENV": "production", "APP_BASE_URL": "https://app.example.com", "LOG_FORMAT": "json"}),
			want: want(func(c *config.Config) {
				c.Production, c.LogJSON, c.AppBaseURL = true, true, "https://app.example.com"
			}),
		},
		{
			name:    "production requires an explicit base url",
			vars:    base(map[string]string{"APP_ENV": "production"}),
			wantErr: []string{"APP_BASE_URL is required in production"},
		},
		{
			name:    "production rejects a plain http base url",
			vars:    base(map[string]string{"APP_ENV": "production", "APP_BASE_URL": "http://app.example.com"}),
			wantErr: []string{"APP_BASE_URL must use https in production"},
		},
		{
			name:    "unknown environment and log format",
			vars:    base(map[string]string{"APP_ENV": "prod", "LOG_FORMAT": "xml"}),
			wantErr: []string{"APP_ENV is invalid", "LOG_FORMAT is invalid"},
		},
		{
			name:    "invalid app base url",
			vars:    base(map[string]string{"APP_BASE_URL": "ftp://x"}),
			wantErr: []string{"APP_BASE_URL is invalid", "scheme"},
		},
		{
			name: "reports every problem",
			vars: map[string]string{"HTTP_ADDR": "nope"},
			wantErr: []string{
				"DATABASE_URL is required", "HTTP_ADDR is invalid", "JWT_SECRET is required", "RESEND_API_KEY is required",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Load(env(tt.vars))

			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("got %+v, want %+v", got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			for _, fragment := range tt.wantErr {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q does not contain %q", err, fragment)
				}
			}
		})
	}
}

func TestS3Configured(t *testing.T) {
	for _, tc := range []struct {
		s3   config.S3
		want bool
	}{
		{config.S3{AccessKey: "a", SecretKey: "b"}, true},
		{config.S3{AccessKey: "a"}, false},
		{config.S3{SecretKey: "b"}, false},
		{config.S3{}, false},
	} {
		if got := tc.s3.Configured(); got != tc.want {
			t.Errorf("%+v Configured() = %v, want %v", tc.s3, got, tc.want)
		}
	}
}

func TestLoadDoesNotLeakPassword(t *testing.T) {
	_, err := config.Load(env(map[string]string{"DATABASE_URL": "mysql://u:s3cret@localhost/db"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error leaks the password: %v", err)
	}
}

func TestLoadDoesNotLeakJWTSecret(t *testing.T) {
	_, err := config.Load(env(map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost/db", "RESEND_API_KEY": "k", "JWT_SECRET": "short-s3cret",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "short-s3cret") {
		t.Fatalf("error leaks the JWT secret: %v", err)
	}
}
