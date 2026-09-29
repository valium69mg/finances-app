// Package resend sends the verification email through the Resend HTTP API.
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the production Resend API origin.
const DefaultBaseURL = "https://api.resend.com"

// Mailer implements app.Mailer over the Resend API.
type Mailer struct {
	apiKey  string
	from    string
	baseURL string
	client  *http.Client
}

// New builds a Mailer. An empty baseURL selects DefaultBaseURL; a nil client
// selects one with a 10 second timeout.
func New(apiKey, from, baseURL string, client *http.Client) *Mailer {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Mailer{apiKey: apiKey, from: from, baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

type emailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
}

const subject = "Verifica tu correo y define tu contraseña"

// SendVerification emails the verification link to the recipient.
func (m *Mailer) SendVerification(ctx context.Context, to, link string) error {
	body, err := json.Marshal(emailRequest{
		From:    m.from,
		To:      []string{to},
		Subject: subject,
		HTML:    verificationHTML(link),
		Text:    verificationText(link),
	})
	if err != nil {
		return fmt.Errorf("encode email: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		// Do returns a *url.Error that includes the URL only, never the key.
		return fmt.Errorf("resend request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errors.New("resend rejected the email: status " + resp.Status)
	}
	return nil
}

func verificationText(link string) string {
	return "Hola,\n\n" +
		"Recibimos una solicitud para iniciar sesión en tu cuenta. Para verificar tu correo " +
		"y definir tu contraseña, abre el siguiente enlace:\n\n" + link + "\n\n" +
		"El enlace es válido durante 1 hora y solo puede usarse una vez. " +
		"Si no solicitaste este correo, puedes ignorarlo.\n"
}

func verificationHTML(link string) string {
	safe := html.EscapeString(link)
	return "<p>Hola,</p>" +
		"<p>Recibimos una solicitud para iniciar sesión en tu cuenta. Para verificar tu correo " +
		"y definir tu contraseña, usa el siguiente enlace:</p>" +
		`<p><a href="` + safe + `">Verificar correo y definir contraseña</a></p>` +
		"<p>El enlace es válido durante 1 hora y solo puede usarse una vez. " +
		"Si no solicitaste este correo, puedes ignorarlo.</p>"
}
