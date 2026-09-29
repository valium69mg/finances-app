package config_test

import (
	"strings"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/platform/config"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestLoad(t *testing.T) {
	const validURL = "postgres://u:p@localhost:5442/db?sslmode=disable"

	tests := []struct {
		name    string
		vars    map[string]string
		want    config.Config
		wantErr []string
	}{
		{
			name: "valid values",
			vars: map[string]string{"DATABASE_URL": validURL, "HTTP_ADDR": ":9090"},
			want: config.Config{DatabaseURL: validURL, HTTPAddr: ":9090"},
		},
		{
			name: "http addr defaults",
			vars: map[string]string{"DATABASE_URL": validURL},
			want: config.Config{DatabaseURL: validURL, HTTPAddr: ":8080"},
		},
		{
			name:    "missing database url",
			vars:    map[string]string{},
			wantErr: []string{"DATABASE_URL is required"},
		},
		{
			name:    "wrong scheme",
			vars:    map[string]string{"DATABASE_URL": "mysql://u:p@localhost/db"},
			wantErr: []string{"DATABASE_URL is invalid", "scheme"},
		},
		{
			name:    "missing host",
			vars:    map[string]string{"DATABASE_URL": "postgres:///db"},
			wantErr: []string{"DATABASE_URL is invalid", "host is required"},
		},
		{
			name:    "invalid http addr",
			vars:    map[string]string{"DATABASE_URL": validURL, "HTTP_ADDR": "not-an-addr"},
			wantErr: []string{"HTTP_ADDR is invalid"},
		},
		{
			name:    "reports every problem",
			vars:    map[string]string{"HTTP_ADDR": "nope"},
			wantErr: []string{"DATABASE_URL is required", "HTTP_ADDR is invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Load(env(tt.vars))

			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.want {
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

func TestLoadDoesNotLeakPassword(t *testing.T) {
	_, err := config.Load(env(map[string]string{"DATABASE_URL": "mysql://u:s3cret@localhost/db"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error leaks the password: %v", err)
	}
}
