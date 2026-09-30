package resend_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/resend"
)

func TestSendVerification(t *testing.T) {
	const link = "https://app.example.com/verify?token=abc&x=1"

	var gotAuth, gotPath, gotMethod string
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotMethod = r.Header.Get("Authorization"), r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()

	m := resend.New("re_test_key", "onboarding@resend.dev", srv.URL, srv.Client())
	if err := m.SendVerification(context.Background(), "me@example.com", link); err != nil {
		t.Fatal(err)
	}

	if gotMethod != http.MethodPost || gotPath != "/emails" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer re_test_key" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if got["from"] != "onboarding@resend.dev" {
		t.Errorf("from = %v", got["from"])
	}
	to, _ := got["to"].([]any)
	if len(to) != 1 || to[0] != "me@example.com" {
		t.Errorf("to = %v", got["to"])
	}
	if s, _ := got["subject"].(string); !strings.Contains(s, "Verifica") {
		t.Errorf("subject = %q, want Spanish copy", s)
	}
	text, _ := got["text"].(string)
	if !strings.Contains(text, link) {
		t.Errorf("text body lacks the link: %q", text)
	}
	htmlBody, _ := got["html"].(string)
	if !strings.Contains(htmlBody, "token=abc&amp;x=1") {
		t.Errorf("html body must contain the escaped link: %q", htmlBody)
	}
}

func TestSend(t *testing.T) {
	var gotAuth, gotPath string
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := resend.New("re_test_key", "avisos@example.com", srv.URL, srv.Client())
	if err := m.Send(context.Background(), "me@example.com", "Asunto", "<p>hola</p>", "hola"); err != nil {
		t.Fatal(err)
	}

	if gotPath != "/emails" || gotAuth != "Bearer re_test_key" {
		t.Errorf("request path %q, authorization %q", gotPath, gotAuth)
	}
	want := map[string]string{"from": "avisos@example.com", "subject": "Asunto", "html": "<p>hola</p>", "text": "hola"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %q", k, got[k], v)
		}
	}
	if to, _ := got["to"].([]any); len(to) != 1 || to[0] != "me@example.com" {
		t.Errorf("to = %v", got["to"])
	}
}

func TestSendRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	m := resend.New("re_secret_key", "a@b.co", srv.URL, srv.Client())
	err := m.Send(context.Background(), "me@example.com", "s", "<p>h</p>", "h")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "re_secret_key") {
		t.Errorf("error leaks the API key: %v", err)
	}
}

func TestSendVerificationErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"unauthorized", http.StatusUnauthorized},
		{"validation error", http.StatusUnprocessableEntity},
		{"server error", http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			m := resend.New("re_secret_key", "a@b.co", srv.URL, srv.Client())
			err := m.SendVerification(context.Background(), "me@example.com", "https://x/verify?token=t")
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), "re_secret_key") {
				t.Errorf("error leaks the API key: %v", err)
			}
		})
	}
}

func TestSendVerificationUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	m := resend.New("k", "a@b.co", url, nil)
	if err := m.SendVerification(context.Background(), "me@example.com", "l"); err == nil {
		t.Fatal("expected error")
	}
}
