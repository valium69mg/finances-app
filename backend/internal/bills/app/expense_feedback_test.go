package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

// movementStore is a minimal in-memory ledger for the real expenses service.
type movementStore struct {
	rows   map[int]ledger.Movement
	nextID int
}

func (s *movementStore) Create(_ context.Context, m ledger.Movement) (ledger.Movement, error) {
	s.nextID++
	m.ID = s.nextID
	s.rows[m.ID] = m
	return m, nil
}
func (s *movementStore) Update(context.Context, ledger.Movement) error { return nil }
func (s *movementStore) Delete(_ context.Context, id int) error {
	delete(s.rows, id)
	return nil
}
func (s *movementStore) GetByID(_ context.Context, id int) (ledger.Movement, error) {
	m, ok := s.rows[id]
	if !ok {
		return ledger.Movement{}, ledger.ErrNotFound
	}
	return m, nil
}
func (s *movementStore) ListByRange(context.Context, string, string, ledger.Kind, int) ([]ledger.Movement, error) {
	return nil, nil
}
func (s *movementStore) ListAllByKind(context.Context, ledger.Kind) ([]ledger.Movement, error) {
	return nil, nil
}

// failingBudgets resolves the config but not the month budgets, so the budget
// feedback of a saved expense fails.
type failingBudgets struct{ cfg settings.Config }

func (f failingBudgets) Get(context.Context) (settings.Config, error) { return f.cfg, nil }
func (f failingBudgets) MonthBudgets(context.Context, string) ([]settingsapp.CategoryBudget, error) {
	return nil, errors.New("budgets unavailable")
}

// A budget feedback failure must not fail Pay after the expense was saved:
// the occurrence still links the expense and a retry cannot duplicate it.
func TestPayLinksTheExpenseWhenBudgetFeedbackFails(t *testing.T) {
	cfg := settingstest.RealConfig()
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	store := &movementStore{rows: map[int]ledger.Movement{}}
	now := func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }
	expenses := expensesapp.NewService(store, failingBudgets{cfg: cfg}, now)

	fx := newFixture()
	svc := app.NewService(fx.repo, expenses, fx.settings, now, nil)
	st, err := svc.Create(context.Background(), megacable())
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Pay(context.Background(), st.Bill.ID, bills.PaymentInput{})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if len(store.rows) != 1 || len(fx.repo.resolved) != 1 || fx.repo.resolved[0].ExpenseID == nil || *fx.repo.resolved[0].ExpenseID != res.Expense.ID {
		t.Errorf("rows=%d resolved=%+v expense=%+v", len(store.rows), fx.repo.resolved, res.Expense)
	}
}
