package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
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

// failingBudgets resolves the config but cannot resolve the month budgets, so
// the budget feedback of a saved expense fails.
type failingBudgets struct{ cfg settings.Config }

func (f failingBudgets) Get(context.Context) (settings.Config, error) { return f.cfg, nil }
func (f failingBudgets) MonthBudgets(context.Context, string) ([]settingsapp.CategoryBudget, error) {
	return nil, errors.New("budgets unavailable")
}

// A budget feedback failure must not turn a saved payment expense into an
// error: the filing still links it and a retry cannot duplicate it.
func TestPaymentExpenseIsLinkedWhenBudgetFeedbackFails(t *testing.T) {
	cfg := settingstest.RealConfig()
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	store := &movementStore{rows: map[int]ledger.Movement{}}
	expenses := expensesapp.NewService(store, failingBudgets{cfg: cfg}, func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) })

	fx := newFixture()
	svc := app.NewService(fx.repo, fx.invoices, expenses, fx.settings, func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }, nil)

	res, err := svc.Register(context.Background(), app.RegisterInput{
		Period: "2026-10", Date: "2026-11-05",
		Payment: &app.PaymentInput{Date: "2026-11-07", ISRPaid: d("100"), IVAPaid: d("50"), RecordExpense: true},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(store.rows) != 1 || res.Expense == nil || res.Filing.ExpenseMovementID == nil || *res.Filing.ExpenseMovementID != res.Expense.ID {
		t.Errorf("rows=%d expense=%+v link=%v", len(store.rows), res.Expense, res.Filing.ExpenseMovementID)
	}

	// Paying later goes through the same path.
	if _, err := svc.Register(context.Background(), app.RegisterInput{Period: "2026-09", Date: "2026-10-05"}); err != nil {
		t.Fatal(err)
	}
	paid, err := svc.Pay(context.Background(), "2026-09", app.PaymentInput{Date: "2026-11-07", ISRPaid: d("10"), RecordExpense: true})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if len(store.rows) != 2 || paid.Filing.ExpenseMovementID == nil || *paid.Filing.ExpenseMovementID != paid.Expense.ID {
		t.Errorf("rows=%d expense=%+v link=%v", len(store.rows), paid.Expense, paid.Filing.ExpenseMovementID)
	}
}
