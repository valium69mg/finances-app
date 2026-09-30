package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	"github.com/valium69mg/finances-app/backend/internal/futureexpenses/app"
	domain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savingsapp "github.com/valium69mg/finances-app/backend/internal/savings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// memory is an in-memory Repo, Savings and Expenses in one: the movements it
// stores are the shared table, so the tests read the same numbers the real
// repository would sum.
type memory struct {
	items     []domain.FutureExpense
	movements []ledger.Movement
	nextID    int
}

type config struct{ cfg settings.Config }

func (c config) Get(context.Context) (settings.Config, error) { return c.cfg, nil }

func (m *memory) saved(id int) decimal.Decimal {
	sum := decimal.Zero
	for _, mv := range m.movements {
		if mv.FutureExpenseID == id {
			sum = sum.Add(mv.AmountMXN)
		}
	}
	return sum
}

func (m *memory) Create(_ context.Context, v domain.Validated) (domain.FutureExpense, error) {
	m.nextID++
	it := domain.FutureExpense{ID: m.nextID, Name: v.Name, Target: v.Target, DueDate: v.DueDate, Status: domain.StatusActive}
	m.items = append(m.items, it)
	return it, nil
}

func (m *memory) index(id int) int {
	for i, it := range m.items {
		if it.ID == id {
			return i
		}
	}
	return -1
}

func (m *memory) Get(_ context.Context, id int) (domain.FutureExpense, error) { return m.get(id) }

func (m *memory) get(id int) (domain.FutureExpense, error) {
	i := m.index(id)
	if i < 0 {
		return domain.FutureExpense{}, domain.ErrNotFound
	}
	it := m.items[i]
	it.Saved = m.saved(id)
	return it, nil
}

func (m *memory) List(_ context.Context) ([]domain.FutureExpense, error) {
	out := []domain.FutureExpense{}
	for _, it := range m.items {
		it.Saved = m.saved(it.ID)
		out = append(out, it)
	}
	return out, nil
}

func (m *memory) Update(_ context.Context, id int, v domain.Validated) (domain.FutureExpense, error) {
	i := m.index(id)
	switch {
	case i < 0:
		return domain.FutureExpense{}, domain.ErrNotFound
	case m.items[i].Status == domain.StatusPaid:
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	m.items[i].Name, m.items[i].Target, m.items[i].DueDate = v.Name, v.Target, v.DueDate
	return m.get(id)
}

func (m *memory) Delete(_ context.Context, id int) error {
	i := m.index(id)
	if i < 0 {
		return domain.ErrNotFound
	}
	m.items = append(m.items[:i], m.items[i+1:]...)
	for j := range m.movements {
		if m.movements[j].FutureExpenseID == id {
			m.movements[j].FutureExpenseID = 0
		}
	}
	return nil
}

func (m *memory) FreeBalance(context.Context) (decimal.Decimal, error) {
	sum := decimal.Zero
	for _, mv := range m.movements {
		if mv.Kind == ledger.KindSavings && mv.Category == ledger.CategoryFutureExpenses && mv.FutureExpenseID == 0 {
			sum = sum.Add(mv.AmountMXN)
		}
	}
	return sum, nil
}

func (m *memory) MarkPaid(_ context.Context, id int, plan func(decimal.Decimal) (app.Settlement, error)) (domain.FutureExpense, error) {
	i := m.index(id)
	switch {
	case i < 0:
		return domain.FutureExpense{}, domain.ErrNotFound
	case m.items[i].Status == domain.StatusPaid:
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	st, err := plan(m.saved(id))
	if err != nil {
		return domain.FutureExpense{}, err
	}
	m.add(st.Expense)
	for _, s := range st.Savings {
		m.add(s)
	}
	paid := st.Expense.AmountMXN
	expenseID := m.movements[len(m.movements)-len(st.Savings)-1].ID
	m.items[i].Status, m.items[i].PaidAt, m.items[i].AmountPaid, m.items[i].ExpenseMovementID = domain.StatusPaid, st.Expense.Date, &paid, &expenseID
	return m.get(id)
}

func (m *memory) add(mv ledger.Movement) ledger.Movement {
	mv.ID = len(m.movements) + 1
	m.movements = append(m.movements, mv)
	return mv
}

func (m *memory) Build(_ context.Context, in savingsapp.Input) (ledger.Movement, error) {
	if in.Amount.IsZero() {
		return ledger.Movement{}, errors.New("zero")
	}
	return ledger.Movement{
		Date: in.Date, Description: in.Description, Category: in.Category, Kind: ledger.KindSavings,
		Amount: in.Amount, AmountMXN: in.Amount, FutureExpenseID: in.FutureExpenseID,
	}, nil
}

func (m *memory) CreateSaving(ctx context.Context, in savingsapp.Input) (ledger.Movement, error) {
	mv, err := m.Build(ctx, in)
	if err != nil {
		return ledger.Movement{}, err
	}
	return m.add(mv), nil
}

func (m *memory) CreateMany(ctx context.Context, ins []savingsapp.Input) ([]ledger.Movement, error) {
	built := make([]ledger.Movement, len(ins))
	for i, in := range ins {
		mv, err := m.Build(ctx, in)
		if err != nil {
			return nil, err
		}
		built[i] = mv
	}
	out := make([]ledger.Movement, len(built))
	for i, mv := range built {
		out[i] = m.add(mv)
	}
	return out, nil
}

type expenses struct{ memory *memory }

func (e expenses) Build(_ context.Context, in expensesapp.Input) (ledger.Movement, error) {
	if !in.Amount.IsPositive() {
		return ledger.Movement{}, errors.New("not positive")
	}
	return ledger.Movement{
		Date: in.Date, Description: in.Description, Category: in.Category, Kind: ledger.KindExpense,
		Amount: in.Amount, AmountMXN: in.Amount,
	}, nil
}

func (e expenses) InferCategory(_ context.Context, description string) (string, bool, error) {
	if description == "Laptop" {
		return "Ocio", true, nil
	}
	return "", false, nil
}

func newService(t *testing.T) (*app.Service, *memory) {
	t.Helper()
	mem := &memory{}
	now := func() time.Time { return time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC) }
	return app.NewService(mem, mem, expenses{mem}, config{}, now), mem
}

func mustCreate(t *testing.T, s *app.Service, name, due, target string) domain.Planned {
	t.Helper()
	p, err := s.Create(context.Background(), domain.Input{Name: name, Target: d(target), DueDate: due})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return p
}

func TestCreateValidatesAndPlans(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.Create(context.Background(), domain.Input{Name: "", Target: d("1"), DueDate: "2027-01-01"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
	p := mustCreate(t, s, "Laptop", "2027-01-20", "8000")
	if p.Status != domain.StatusActive || p.CyclesLeft != 4 || !p.Suggested.Equal(d("2000")) {
		t.Errorf("planned = %+v", p)
	}
}

func TestContributeLinksAndCountsPerItem(t *testing.T) {
	s, mem := newService(t)
	ctx := context.Background()
	a := mustCreate(t, s, "Viaje", "2026-12-15", "24000")
	b := mustCreate(t, s, "Laptop", "2027-01-20", "8000")
	if _, m, err := s.Contribute(ctx, a.ID, app.SavingInput{Amount: d("1000")}); err != nil || m.FutureExpenseID != a.ID || m.Category != "Gastos futuros" || m.Description != "Ahorro para Viaje" {
		t.Fatalf("Contribute = %+v, %v", m, err)
	}
	got, _, err := s.Contribute(ctx, b.ID, app.SavingInput{Amount: d("250.50")})
	if err != nil || !got.Saved.Equal(d("250.50")) {
		t.Fatalf("saved = %s, %v", got.Saved, err)
	}
	first, _ := s.Get(ctx, a.ID)
	if !first.Saved.Equal(d("1000")) {
		t.Errorf("first saved = %s, want 1000 (savings are per item, not spread by due date)", first.Saved)
	}
	if free, _ := mem.FreeBalance(ctx); !free.IsZero() {
		t.Errorf("free balance = %s, want 0", free)
	}
	for _, bad := range []string{"0", "-5", "1.005"} {
		if _, _, err := s.Contribute(ctx, a.ID, app.SavingInput{Amount: d(bad)}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("amount %s: err = %v, want ErrInvalidInput", bad, err)
		}
	}
	if _, _, err := s.Contribute(ctx, 99, app.SavingInput{Amount: d("1")}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown item: err = %v", err)
	}
}

func TestAssignMovesFreeBalanceOnRequestOnly(t *testing.T) {
	s, mem := newService(t)
	ctx := context.Background()
	item := mustCreate(t, s, "Viaje", "2026-12-15", "24000")
	mem.add(ledger.Movement{Kind: ledger.KindSavings, Category: "Gastos futuros", Amount: d("5000"), AmountMXN: d("5000")})
	mem.add(ledger.Movement{Kind: ledger.KindSavings, Category: "Inversiones", Amount: d("900"), AmountMXN: d("900")})

	if free, _ := mem.FreeBalance(ctx); !free.Equal(d("5000")) {
		t.Fatalf("free = %s, want 5000 (other categories do not count)", free)
	}
	if plan, _ := s.Overview(ctx); !plan.Saved.IsZero() || !plan.FreeBalance.Equal(d("5000")) {
		t.Errorf("before assigning: saved %s free %s; the pool must not be spread automatically", plan.Saved, plan.FreeBalance)
	}
	got, err := s.Assign(ctx, item.ID, app.AssignInput{Amount: d("1200")})
	if err != nil || !got.Saved.Equal(d("1200")) {
		t.Fatalf("Assign = %+v, %v", got, err)
	}
	if free, _ := mem.FreeBalance(ctx); !free.Equal(d("3800")) {
		t.Errorf("free = %s, want 3800", free)
	}
	total := ledger.SumBy(mem.movements, ledger.Filter{Kind: ledger.KindSavings, Category: "Gastos futuros"})
	if !total.Equal(d("5000")) {
		t.Errorf("Gastos futuros total = %s, want 5000: an assignment does not change the savings", total)
	}
	if _, err := s.Assign(ctx, item.ID, app.AssignInput{Amount: d("3800.01")}); !errors.Is(err, domain.ErrInsufficientFreeBalance) {
		t.Errorf("err = %v, want ErrInsufficientFreeBalance", err)
	}
	if _, err := s.Assign(ctx, item.ID, app.AssignInput{Amount: d("0")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("zero: err = %v", err)
	}
}

func payRows(mem *memory) (linked, unlinked []ledger.Movement) {
	for _, m := range mem.movements {
		if m.Kind != ledger.KindSavings {
			continue
		}
		if m.FutureExpenseID != 0 {
			linked = append(linked, m)
		} else {
			unlinked = append(unlinked, m)
		}
	}
	return
}

func TestPayReleasesSavedAndReturnsTheRemainder(t *testing.T) {
	tests := []struct {
		name, saved, paid string
		wantFree          string // free balance after paying (no free money before)
		wantPortfolioDrop string // what the savings lose: min(saved, paid)
	}{
		{"saved exceeds paid", "500", "400", "100", "400"},
		{"saved equals paid", "400", "400", "0", "400"},
		{"saved below paid", "300", "400", "0", "300"},
		{"nothing saved", "0", "400", "0", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mem := newService(t)
			ctx := context.Background()
			item := mustCreate(t, s, "Laptop", "2027-01-20", "8000")
			if d(tt.saved).IsPositive() {
				if _, _, err := s.Contribute(ctx, item.ID, app.SavingInput{Amount: d(tt.saved)}); err != nil {
					t.Fatal(err)
				}
			}
			paid := d(tt.paid)
			got, expense, err := s.Pay(ctx, item.ID, app.PayInput{Amount: &paid, Date: "2026-10-05"})
			if err != nil {
				t.Fatalf("Pay: %v", err)
			}
			if got.Status != domain.StatusPaid || got.PaidAt != "2026-10-05" || got.AmountPaid == nil || !got.AmountPaid.Equal(paid) || !got.Saved.IsZero() {
				t.Errorf("paid item = %+v", got)
			}
			if expense.Kind != ledger.KindExpense || expense.Category != "Ocio" || !expense.Amount.Equal(paid) || expense.Date != "2026-10-05" || expense.ID == 0 {
				t.Errorf("expense = %+v (category inferred from the name)", expense)
			}
			if free, _ := mem.FreeBalance(ctx); !free.Equal(d(tt.wantFree)) {
				t.Errorf("free = %s, want %s", free, tt.wantFree)
			}
			savings := ledger.SumBy(mem.movements, ledger.Filter{Kind: ledger.KindSavings})
			if !savings.Equal(d("0").Sub(d(tt.wantPortfolioDrop)).Add(d(tt.saved))) {
				t.Errorf("net savings = %s, want %s (saved %s minus what was spent %s)", savings, d(tt.saved).Sub(d(tt.wantPortfolioDrop)), tt.saved, tt.wantPortfolioDrop)
			}
			if linked, _ := payRows(mem); !mem.saved(item.ID).IsZero() && len(linked) > 0 {
				t.Errorf("the item keeps %s linked after paying", mem.saved(item.ID))
			}
		})
	}
}

func TestPayDefaultsToTheTarget(t *testing.T) {
	s, _ := newService(t)
	item := mustCreate(t, s, "Domain", "2027-01-20", "300")
	got, expense, err := s.Pay(context.Background(), item.ID, app.PayInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !expense.Amount.Equal(d("300")) || expense.Category != domain.DefaultExpenseCategory || expense.Date != "" {
		// The date is filled by the real expenses service (today in TZ_NAME).
		t.Errorf("expense = %+v, want the target in the default category", expense)
	}
	if got.AmountPaid == nil || !got.AmountPaid.Equal(d("300")) {
		t.Errorf("amount paid = %v", got.AmountPaid)
	}
}

func TestPayTwiceIsRejectedAndPaidItemsAreFrozen(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	item := mustCreate(t, s, "Laptop", "2027-01-20", "100")
	if _, _, err := s.Pay(ctx, item.ID, app.PayInput{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Pay(ctx, item.ID, app.PayInput{}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("second Pay: err = %v, want ErrAlreadyPaid", err)
	}
	if _, _, err := s.Contribute(ctx, item.ID, app.SavingInput{Amount: d("1")}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("Contribute: err = %v, want ErrAlreadyPaid", err)
	}
	if _, err := s.Assign(ctx, item.ID, app.AssignInput{Amount: d("1")}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("Assign: err = %v, want ErrAlreadyPaid", err)
	}
	if _, err := s.Update(ctx, item.ID, domain.Input{Name: "x", Target: d("1"), DueDate: "2027-01-01"}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("Update: err = %v, want ErrAlreadyPaid", err)
	}
}

func TestPayValidatesBeforeWriting(t *testing.T) {
	s, mem := newService(t)
	item := mustCreate(t, s, "Laptop", "2027-01-20", "100")
	for _, amount := range []string{"0", "-1", "1.001"} {
		a := d(amount)
		if _, _, err := s.Pay(context.Background(), item.ID, app.PayInput{Amount: &a}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("amount %s: err = %v, want ErrInvalidInput", amount, err)
		}
	}
	if len(mem.movements) != 0 || mem.items[0].Status != domain.StatusActive {
		t.Errorf("a rejected payment wrote something: %+v", mem.movements)
	}
}

func TestDeleteReturnsLinkedSavingsToTheFreeBalance(t *testing.T) {
	s, mem := newService(t)
	ctx := context.Background()
	item := mustCreate(t, s, "Viaje", "2026-12-15", "24000")
	if _, _, err := s.Contribute(ctx, item.ID, app.SavingInput{Amount: d("700")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if free, _ := mem.FreeBalance(ctx); !free.Equal(d("700")) {
		t.Errorf("free = %s, want 700: deleting an item never orphans money", free)
	}
	if err := s.Delete(ctx, item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second Delete: err = %v", err)
	}
}

func TestListSplitsActiveAndPaid(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	mustCreate(t, s, "Viaje", "2026-12-15", "24000")
	old := mustCreate(t, s, "Old", "2026-10-01", "10")
	recent := mustCreate(t, s, "Recent", "2026-10-02", "10")
	if _, _, err := s.Pay(ctx, old.ID, app.PayInput{Date: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Pay(ctx, recent.ID, app.PayInput{Date: "2026-09-20"}); err != nil {
		t.Fatal(err)
	}
	l, err := s.List(ctx)
	if err != nil || len(l.Plan.Items) != 1 || len(l.Paid) != 2 || l.Paid[0].Name != "Recent" {
		t.Errorf("List = %+v, %v, want 1 active and 2 paid, most recent first", l, err)
	}
}
