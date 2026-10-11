package app_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

// fakeItem and fakeRow back the future expenses half of fakeRepo. Rows mimic
// the Ahorro movements of the Gastos futuros category: linked ones carry the
// item id.
type fakeItem struct {
	user string
	domain.FutureExpense
}

type fakeRow struct {
	user   string
	itemID int
	app.SavingRow
}

func (r *fakeRepo) item(user string, id int) (*fakeItem, error) {
	for _, it := range r.items {
		if it.ID == id && it.user == user {
			return it, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) saved(user string, id int) decimal.Decimal {
	sum := decimal.Zero
	for _, row := range r.rows {
		if row.user == user && row.itemID == id && id != 0 {
			sum = sum.Add(row.Amount)
		}
	}
	return sum
}

func (r *fakeRepo) CreateFutureExpense(_ context.Context, user string, v domain.FutureValidated) (domain.FutureExpense, error) {
	it := &fakeItem{user: user, FutureExpense: domain.FutureExpense{ID: len(r.items) + 1, Name: v.Name, Target: v.Target, DueDate: v.DueDate, Status: domain.FutureActive}}
	r.items = append(r.items, it)
	return it.FutureExpense, nil
}

func (r *fakeRepo) GetFutureExpense(_ context.Context, user string, id int) (domain.FutureExpense, error) {
	it, err := r.item(user, id)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	out := it.FutureExpense
	out.Saved = r.saved(user, id)
	return out, nil
}

func (r *fakeRepo) ListFutureExpenses(_ context.Context, user string) ([]domain.FutureExpense, error) {
	out := []domain.FutureExpense{}
	for _, it := range r.items {
		if it.user == user {
			e := it.FutureExpense
			e.Saved = r.saved(user, it.ID)
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DueDate < out[j].DueDate })
	return out, nil
}

func (r *fakeRepo) UpdateFutureExpense(_ context.Context, user string, id int, v domain.FutureValidated) (domain.FutureExpense, error) {
	it, err := r.item(user, id)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	if it.Status == domain.FuturePaid {
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	it.Name, it.Target, it.DueDate = v.Name, v.Target, v.DueDate
	return it.FutureExpense, nil
}

func (r *fakeRepo) DeleteFutureExpense(_ context.Context, user string, id int) error {
	for i, it := range r.items {
		if it.ID == id && it.user == user {
			r.items = append(r.items[:i], r.items[i+1:]...)
			for j := range r.rows {
				if r.rows[j].user == user && r.rows[j].itemID == id {
					r.rows[j].itemID = 0
				}
			}
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeRepo) free(user string) decimal.Decimal {
	sum := decimal.Zero
	for _, row := range r.rows {
		if row.user == user && row.itemID == 0 {
			sum = sum.Add(row.Amount)
		}
	}
	return sum
}

func (r *fakeRepo) FutureFreeBalance(_ context.Context, user string) (decimal.Decimal, error) {
	return r.free(user), nil
}

func (r *fakeRepo) ContributeFuture(_ context.Context, user string, id int, row app.SavingRow) (domain.Movement, error) {
	it, err := r.item(user, id)
	if err != nil {
		return domain.Movement{}, err
	}
	if it.Status == domain.FuturePaid {
		return domain.Movement{}, domain.ErrAlreadyPaid
	}
	r.rows = append(r.rows, fakeRow{user: user, itemID: id, SavingRow: row})
	return domain.Movement{ID: len(r.rows), Kind: domain.KindSavings, Amount: row.Amount, Description: row.Description, Date: row.Date, FutureExpenseID: &id}, nil
}

func (r *fakeRepo) AssignFuture(_ context.Context, user string, id int, amount decimal.Decimal, date, description string) error {
	it, err := r.item(user, id)
	if err != nil {
		return err
	}
	if it.Status == domain.FuturePaid {
		return domain.ErrAlreadyPaid
	}
	if amount.GreaterThan(r.free(user)) {
		return domain.ErrInsufficientFreeBalance
	}
	r.rows = append(r.rows,
		fakeRow{user: user, SavingRow: app.SavingRow{Amount: amount.Neg(), Date: date, Description: description}},
		fakeRow{user: user, itemID: id, SavingRow: app.SavingRow{Amount: amount, Date: date, Description: description, Linked: true}})
	return nil
}

func (r *fakeRepo) PayFuture(_ context.Context, user string, id int, plan func(domain.FutureExpense) (app.Settlement, error)) (domain.FutureExpense, domain.Movement, error) {
	it, err := r.item(user, id)
	if err != nil {
		return domain.FutureExpense{}, domain.Movement{}, err
	}
	if it.Status == domain.FuturePaid {
		return domain.FutureExpense{}, domain.Movement{}, domain.ErrAlreadyPaid
	}
	cur := it.FutureExpense
	cur.Saved = r.saved(user, id)
	st, err := plan(cur)
	if err != nil {
		return domain.FutureExpense{}, domain.Movement{}, err
	}
	r.settled = append(r.settled, st)
	for _, row := range st.Savings {
		link := 0
		if row.Linked {
			link = id
		}
		r.rows = append(r.rows, fakeRow{user: user, itemID: link, SavingRow: row})
	}
	paidAmount := st.Expense.Amount
	it.Status, it.PaidAt, it.AmountPaid = domain.FuturePaid, st.Expense.Date, &paidAmount
	return it.FutureExpense, domain.Movement{ID: 900, Kind: domain.KindExpense, CategoryID: st.Expense.CategoryID, Amount: st.Expense.Amount, Description: st.Expense.Description, Date: st.Expense.Date}, nil
}

func newItem(t *testing.T, svc *app.Service, actor app.Identity, target string) int {
	t.Helper()
	p, err := svc.CreateFutureExpense(ctx, actor, domain.FutureInput{Name: "Viaje", Target: d(target), DueDate: "2026-12-20"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return p.ID
}

func categoryID(t *testing.T, svc *app.Service, actor app.Identity, name string) int {
	t.Helper()
	cats, err := svc.Categories(ctx, actor, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("no category %q", name)
	return 0
}

func TestFutureOperationsForbidTheOwnerAndAnonymous(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	for _, actor := range []app.Identity{owner, {}, {Role: "household"}} {
		calls := map[string]error{}
		_, calls["create"] = svc.CreateFutureExpense(ctx, actor, domain.FutureInput{})
		_, calls["get"] = svc.FutureExpense(ctx, actor, "", 1)
		_, calls["update"] = svc.UpdateFutureExpense(ctx, actor, 1, domain.FutureInput{})
		calls["delete"] = svc.DeleteFutureExpense(ctx, actor, 1)
		_, calls["list"] = svc.FutureExpenses(ctx, actor, "")
		_, _, calls["contribute"] = svc.ContributeFuture(ctx, actor, 1, app.FutureSavingInput{})
		_, calls["assign"] = svc.AssignFuture(ctx, actor, 1, app.FutureAssignInput{})
		_, _, calls["pay"] = svc.PayFuture(ctx, actor, 1, app.FuturePayInput{})
		for name, err := range calls {
			if !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("%s as %+v: error = %v; want ErrForbidden", name, actor, err)
			}
		}
	}
}

func TestCreateFutureExpenseValidatesAndPlans(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	for name, in := range map[string]domain.FutureInput{
		"empty name":    {Name: " ", Target: d("100"), DueDate: "2026-12-20"},
		"zero target":   {Name: "x", Target: d("0"), DueDate: "2026-12-20"},
		"3 decimals":    {Name: "x", Target: d("1.005"), DueDate: "2026-12-20"},
		"out of range":  {Name: "x", Target: d("1e30"), DueDate: "2026-12-20"},
		"bad date":      {Name: "x", Target: d("100"), DueDate: "20/12/2026"},
		"absurd target": {Name: "x", Target: d("1e400"), DueDate: "2026-12-20"},
	} {
		if _, err := svc.CreateFutureExpense(ctx, her, in); !errors.Is(err, domain.ErrFutureInvalid) {
			t.Errorf("%s: error = %v; want ErrFutureInvalid", name, err)
		}
	}
	// today is 2026-10-10 and the cycle starts on the 1st: Oct, Nov and Dec are
	// 2 cycles away, so 1000 needs 500 per cycle.
	p, err := svc.CreateFutureExpense(ctx, her, domain.FutureInput{Name: " Viaje ", Target: d("1000"), DueDate: "2026-12-20"})
	if err != nil || p.Name != "Viaje" || p.CyclesLeft != 2 || !p.Suggested.Equal(d("500")) {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestFutureExpensesAreScopedToTheUser(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	id := newItem(t, svc, her, "1000")
	if _, err := svc.FutureExpense(ctx, other, "", id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("get: %v; want ErrNotFound", err)
	}
	if _, err := svc.UpdateFutureExpense(ctx, other, id, domain.FutureInput{Name: "x", Target: d("1"), DueDate: "2026-12-20"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("update: %v; want ErrNotFound", err)
	}
	if err := svc.DeleteFutureExpense(ctx, other, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("delete: %v; want ErrNotFound", err)
	}
	if _, _, err := svc.ContributeFuture(ctx, other, id, app.FutureSavingInput{Amount: d("10")}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("contribute: %v; want ErrNotFound", err)
	}
	if _, err := svc.AssignFuture(ctx, other, id, app.FutureAssignInput{Amount: d("10")}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("assign: %v; want ErrNotFound", err)
	}
	if _, _, err := svc.PayFuture(ctx, other, id, app.FuturePayInput{CategoryID: 1}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("pay: %v; want ErrNotFound", err)
	}
	if l, _ := svc.FutureExpenses(ctx, other, ""); len(l.Plan.Items) != 0 {
		t.Errorf("another user lists %d items", len(l.Plan.Items))
	}
}

func TestContributeDefaultsAndRules(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	id := newItem(t, svc, her, "1000")
	p, m, err := svc.ContributeFuture(ctx, her, id, app.FutureSavingInput{Amount: d("250")})
	if err != nil {
		t.Fatal(err)
	}
	if m.Description != "Ahorro para Viaje" || m.Date != "2026-10-10" || m.FutureExpenseID == nil || *m.FutureExpenseID != id {
		t.Errorf("saving = %+v", m)
	}
	if !p.Saved.Equal(d("250")) || !p.Remaining.Equal(d("750")) {
		t.Errorf("item = saved %s remaining %s", p.Saved, p.Remaining)
	}
	for name, in := range map[string]app.FutureSavingInput{
		"zero":     {Amount: d("0")},
		"negative": {Amount: d("-5")},
		"decimals": {Amount: d("1.001")},
		"range":    {Amount: d("1e30")},
		"long":     {Amount: d("5"), Description: strings.Repeat("a", 121)},
	} {
		if _, _, err := svc.ContributeFuture(ctx, her, id, in); !errors.Is(err, domain.ErrFutureInvalid) {
			t.Errorf("%s: error = %v; want ErrFutureInvalid", name, err)
		}
	}
	if _, _, err := svc.ContributeFuture(ctx, her, id, app.FutureSavingInput{Amount: d("5"), Date: "nope"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad date: %v; want ErrInvalidInput", err)
	}
}

func TestAssignMovesTheFreeBalance(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	id := newItem(t, svc, her, "1000")
	// 300 of free balance saved before the module existed.
	r.rows = append(r.rows, fakeRow{user: her.UserID, SavingRow: app.SavingRow{Amount: d("300")}})
	if _, err := svc.AssignFuture(ctx, her, id, app.FutureAssignInput{Amount: d("301")}); !errors.Is(err, domain.ErrInsufficientFreeBalance) {
		t.Fatalf("over the balance: %v; want ErrInsufficientFreeBalance", err)
	}
	p, err := svc.AssignFuture(ctx, her, id, app.FutureAssignInput{Amount: d("120")})
	if err != nil || !p.Saved.Equal(d("120")) {
		t.Fatalf("assign: %+v, %v", p, err)
	}
	if free := r.free(her.UserID); !free.Equal(d("180")) {
		t.Errorf("free balance = %s, want 180", free)
	}
	if _, err := svc.AssignFuture(ctx, her, id, app.FutureAssignInput{Amount: d("0")}); !errors.Is(err, domain.ErrFutureInvalid) {
		t.Errorf("zero: %v; want ErrFutureInvalid", err)
	}
}

func TestPayRequiresHerGastoCategory(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	id := newItem(t, svc, her, "1000")
	ropa := categoryID(t, svc, her, "Ropa")
	trabajo := categoryID(t, svc, her, "Trabajo")
	otherRopa := categoryID(t, svc, other, "Ropa")
	for name, cat := range map[string]int{"missing": 0, "unknown": 9999, "an Ingreso category": trabajo, "another user's category": otherRopa} {
		if _, _, err := svc.PayFuture(ctx, her, id, app.FuturePayInput{CategoryID: cat}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: error = %v; want ErrInvalidInput", name, err)
		}
	}
	if len(r.settled) != 0 || r.items[0].Status != domain.FutureActive {
		t.Fatal("a rejected payment must change nothing")
	}
	if _, _, err := svc.PayFuture(ctx, her, id, app.FuturePayInput{CategoryID: ropa, Amount: ptr("1.005")}); !errors.Is(err, domain.ErrFutureInvalid) {
		t.Errorf("3 decimals: %v; want ErrFutureInvalid", err)
	}
}

func ptr(s string) *decimal.Decimal { v := d(s); return &v }

func TestPayReleasesWhatWasSaved(t *testing.T) {
	cases := []struct {
		name         string
		saved        string
		paid         *decimal.Decimal
		wantRows     int
		wantRelease  string // negative linked row
		wantRemain   string // positive unlinked row
		wantAmount   string
		wantFreeDiff string
	}{
		{"saved more than paid", "500", ptr("300"), 2, "-500", "200", "300", "200"},
		{"saved less than paid", "100", ptr("300"), 1, "-100", "", "300", "0"},
		{"saved exactly the target, amount defaults", "1000", nil, 1, "-1000", "", "1000", "0"},
		{"nothing saved", "0", ptr("300"), 0, "", "", "300", "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRepo()
			svc := newSvc(r, noCfg)
			id := newItem(t, svc, her, "1000")
			if c.saved != "0" {
				if _, _, err := svc.ContributeFuture(ctx, her, id, app.FutureSavingInput{Amount: d(c.saved)}); err != nil {
					t.Fatal(err)
				}
			}
			ropa := categoryID(t, svc, her, "Ropa")
			paid, expense, err := svc.PayFuture(ctx, her, id, app.FuturePayInput{Date: "2026-10-05", Amount: c.paid, CategoryID: ropa})
			if err != nil {
				t.Fatal(err)
			}
			st := r.settled[0]
			if len(st.Savings) != c.wantRows {
				t.Fatalf("savings rows = %d, want %d: %+v", len(st.Savings), c.wantRows, st.Savings)
			}
			if c.wantRelease != "" && (!st.Savings[0].Amount.Equal(d(c.wantRelease)) || !st.Savings[0].Linked) {
				t.Errorf("release row = %+v, want linked %s", st.Savings[0], c.wantRelease)
			}
			if c.wantRemain != "" && (!st.Savings[1].Amount.Equal(d(c.wantRemain)) || st.Savings[1].Linked) {
				t.Errorf("remainder row = %+v, want unlinked %s", st.Savings[1], c.wantRemain)
			}
			if !st.Expense.Amount.Equal(d(c.wantAmount)) || st.Expense.CategoryID != ropa || st.Expense.Kind != domain.KindExpense ||
				st.Expense.Description != "Viaje" || st.Expense.Date != "2026-10-05" {
				t.Errorf("expense = %+v", st.Expense)
			}
			if paid.Status != domain.FuturePaid || paid.PaidAt != "2026-10-05" || paid.AmountPaid == nil || !paid.AmountPaid.Equal(d(c.wantAmount)) || expense.ID != 900 {
				t.Errorf("paid item = %+v, expense %+v", paid, expense)
			}
			if saved := r.saved(her.UserID, id); !saved.IsZero() {
				t.Errorf("a paid item must be left at zero saved, got %s", saved)
			}
			if free := r.free(her.UserID); !free.Equal(d(c.wantFreeDiff)) {
				t.Errorf("free balance = %s, want %s", free, c.wantFreeDiff)
			}
		})
	}
}

func TestPaidItemsAreLocked(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	id := newItem(t, svc, her, "1000")
	ropa := categoryID(t, svc, her, "Ropa")
	if _, _, err := svc.PayFuture(ctx, her, id, app.FuturePayInput{CategoryID: ropa}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PayFuture(ctx, her, id, app.FuturePayInput{CategoryID: ropa}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("second payment: %v; want ErrAlreadyPaid", err)
	}
	if _, err := svc.UpdateFutureExpense(ctx, her, id, domain.FutureInput{Name: "x", Target: d("5"), DueDate: "2026-12-20"}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("update: %v; want ErrAlreadyPaid", err)
	}
	if _, _, err := svc.ContributeFuture(ctx, her, id, app.FutureSavingInput{Amount: d("5")}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("contribute: %v; want ErrAlreadyPaid", err)
	}
	if _, err := svc.AssignFuture(ctx, her, id, app.FutureAssignInput{Amount: d("5")}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("assign: %v; want ErrAlreadyPaid", err)
	}
	if err := svc.DeleteFutureExpense(ctx, her, id); err != nil {
		t.Errorf("a paid item can be deleted: %v", err)
	}
}

func TestDeleteReturnsSavingsToTheFreeBalance(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	id := newItem(t, svc, her, "1000")
	if _, _, err := svc.ContributeFuture(ctx, her, id, app.FutureSavingInput{Amount: d("400")}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteFutureExpense(ctx, her, id); err != nil {
		t.Fatal(err)
	}
	if free := r.free(her.UserID); !free.Equal(d("400")) {
		t.Errorf("free balance = %s, want 400", free)
	}
	if err := svc.DeleteFutureExpense(ctx, her, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete: %v; want ErrNotFound", err)
	}
}

func TestListSplitsActiveAndPaid(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, day15)
	a := newItem(t, svc, her, "1000")
	b := newItem(t, svc, her, "500")
	ropa := categoryID(t, svc, her, "Ropa")
	if _, _, err := svc.PayFuture(ctx, her, a, app.FuturePayInput{CategoryID: ropa}); err != nil {
		t.Fatal(err)
	}
	r.rows = append(r.rows, fakeRow{user: her.UserID, SavingRow: app.SavingRow{Amount: d("75")}})
	l, err := svc.FutureExpenses(ctx, her, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Plan.Items) != 1 || l.Plan.Items[0].ID != b || len(l.Paid) != 1 || l.Paid[0].ID != a {
		t.Fatalf("active %v paid %v", l.Plan.Items, l.Paid)
	}
	if !l.Plan.FreeBalance.Equal(d("75")) || !l.Plan.Target.Equal(d("500")) {
		t.Errorf("free %s target %s", l.Plan.FreeBalance, l.Plan.Target)
	}
	failing := newSvc(&fakeRepo{owners: map[int]string{}, mOwners: map[int]string{}}, fakeSettings{err: errSet})
	if _, err := failing.FutureExpenses(ctx, her, ""); !errors.Is(err, errSet) {
		t.Errorf("settings failure: %v", err)
	}
}
