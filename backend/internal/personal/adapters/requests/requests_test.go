package requests_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	erapp "github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	erdomain "github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	"github.com/valium69mg/finances-app/backend/internal/personal/adapters/requests"
	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

// The adapter runs the REAL expense requests service; only its storage, mail and limiter are stubbed.

type repo struct {
	erapp.Repo // the methods this test does not use panic on the nil interface
	rows       map[int]erdomain.Request
	owners     []string
}

func (r *repo) Create(_ context.Context, requesterID string, v erdomain.Validated) (erdomain.Request, error) {
	req := erdomain.Request{
		ID: len(r.rows) + 1, RequesterID: requesterID, Amount: v.Amount, Description: v.Description,
		SuggestedCategory: v.SuggestedCategory, ExpenseDate: v.Date, Status: erdomain.StatusPending,
	}
	r.rows[req.ID] = req
	return req, nil
}
func (r *repo) Get(_ context.Context, id int) (erdomain.Request, error) {
	req, ok := r.rows[id]
	if !ok {
		return erdomain.Request{}, erdomain.ErrNotFound
	}
	return req, nil
}
func (r *repo) Cancel(_ context.Context, id int, requesterID string) (erdomain.Request, error) {
	req := r.rows[id]
	if req.Status != erdomain.StatusPending {
		return erdomain.Request{}, erdomain.ErrInvalidState
	}
	req.Status = erdomain.StatusCancelled
	r.rows[id] = req
	return req, nil
}
func (r *repo) OwnerEmails(context.Context) ([]string, error) { return r.owners, nil }

type cfg struct{}

func (cfg) Get(context.Context) (settings.Config, error) { return settingstest.RealConfig(), nil }

type mailer struct{ sent []string }

func (m *mailer) Send(_ context.Context, to, _, _, _ string) error {
	m.sent = append(m.sent, to)
	return nil
}

type limiter struct{ left int }

func (l *limiter) Allow(string, int, time.Duration) bool {
	if l.left == 0 {
		return false
	}
	l.left--
	return true
}

var her = session.Identity{UserID: "u-her", Role: session.RoleHousehold}

func setup(allowed int) (*requests.Adapter, *repo, *mailer) {
	r := &repo{rows: map[int]erdomain.Request{}, owners: []string{"owner@example.com"}}
	m := &mailer{}
	svc := erapp.NewService(erapp.Deps{Repo: r, Settings: cfg{}, Mailer: m, Limiter: &limiter{left: allowed}})
	return requests.New(svc), r, m
}

func TestCreateMakesAPendingRequestAsTheSessionUserAndTellsTheOwner(t *testing.T) {
	a, r, m := setup(5)
	id, err := a.Create(context.Background(), her, app.RequestDraft{Amount: decimal.RequireFromString("499.90"), Description: "Bolsa", Category: "Vivienda", Date: "2026-10-09"})
	if err != nil {
		t.Fatal(err)
	}
	got := r.rows[id]
	if got.RequesterID != her.UserID || got.Status != erdomain.StatusPending || got.SuggestedCategory != "Vivienda" || got.ExpenseDate != "2026-10-09" || !got.Amount.Equal(decimal.RequireFromString("499.90")) {
		t.Errorf("request = %+v", got)
	}
	if len(m.sent) != 1 || m.sent[0] != "owner@example.com" {
		t.Errorf("mails = %v; want the owner notified", m.sent)
	}
}

func TestCreateKeepsTheRulesOfTheRequestsModule(t *testing.T) {
	a, r, _ := setup(1)
	draft := app.RequestDraft{Amount: decimal.RequireFromString("10"), Description: "x", Category: "Vivienda"}

	bad := draft
	bad.Category = "No existe"
	if _, err := a.Create(context.Background(), her, bad); !errors.Is(err, erdomain.ErrInvalidInput) {
		t.Errorf("unknown category: %v; want ErrInvalidInput", err)
	}
	bad = draft
	bad.Amount = decimal.RequireFromString("1.234")
	if _, err := a.Create(context.Background(), her, bad); !errors.Is(err, erdomain.ErrInvalidInput) {
		t.Errorf("3 decimals: %v; want ErrInvalidInput", err)
	}
	if len(r.rows) != 0 {
		t.Fatal("an invalid request was stored")
	}

	// Invalid requests do not consume the hourly budget; the second valid one is limited.
	if _, err := a.Create(context.Background(), her, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Create(context.Background(), her, draft); !errors.Is(err, erdomain.ErrRateLimited) {
		t.Errorf("over the limit: %v; want ErrRateLimited", err)
	}
}

func TestCancelWithdrawsOnlyHerOwnPendingRequest(t *testing.T) {
	a, r, _ := setup(5)
	id, err := a.Create(context.Background(), her, app.RequestDraft{Amount: decimal.RequireFromString("10"), Description: "x", Category: "Vivienda"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Cancel(context.Background(), session.Identity{UserID: "u-other", Role: session.RoleHousehold}, id); !errors.Is(err, erdomain.ErrForbidden) {
		t.Errorf("another user's cancel: %v; want ErrForbidden", err)
	}
	if err := a.Cancel(context.Background(), her, id); err != nil {
		t.Fatal(err)
	}
	if r.rows[id].Status != erdomain.StatusCancelled {
		t.Errorf("status = %s; want cancelada", r.rows[id].Status)
	}
	if err := a.Cancel(context.Background(), her, id); !errors.Is(err, erdomain.ErrInvalidState) {
		t.Errorf("second cancel: %v; want ErrInvalidState", err)
	}
}
