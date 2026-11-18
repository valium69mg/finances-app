package postgres_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

func validItem(name, target string) domain.FutureValidated {
	return domain.FutureValidated{Name: name, Target: d(target), DueDate: "2026-12-20"}
}

func (f fixture) item(t *testing.T, user, target string) domain.FutureExpense {
	t.Helper()
	it, err := f.repo.CreateFutureExpense(ctx, user, validItem("Viaje", target))
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	return it
}

func (f fixture) save(t *testing.T, user string, itemID int, amount string) {
	t.Helper()
	if _, err := f.repo.ContributeFuture(ctx, user, itemID, app.SavingRow{Amount: d(amount), Date: "2026-10-01", Description: "ahorro", Linked: true}); err != nil {
		t.Fatalf("contribute: %v", err)
	}
}

// seedFree puts an unlinked Gastos futuros saving (the free balance) in place.
func (f fixture) seedFree(t *testing.T, user, amount string) {
	t.Helper()
	cats, _ := f.repo.ListCategories(ctx, user, domain.KindSavings)
	gf := find(t, cats, domain.SavingsCategory)
	if _, err := f.repo.CreateMovement(ctx, user, mv(domain.KindSavings, gf, amount, "2026-09-01")); err != nil {
		t.Fatalf("seed free balance: %v", err)
	}
}

func (f fixture) free(t *testing.T, user string) decimal.Decimal {
	t.Helper()
	v, err := f.repo.FutureFreeBalance(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func settle(cat domain.Category, paid string, rows ...app.SavingRow) func(domain.FutureExpense) (app.Settlement, error) {
	return func(it domain.FutureExpense) (app.Settlement, error) {
		// What the savings paid is minus the net of the release rows.
		covered := decimal.Zero
		for _, r := range rows {
			covered = covered.Sub(r.Amount)
		}
		return app.Settlement{
			Expense: domain.ValidMovement{Kind: domain.KindExpense, CategoryID: cat.ID, Amount: d(paid), Description: it.Name, Date: "2026-10-05", CoveredBySavings: covered},
			Savings: rows,
		}, nil
	}
}

func count(t *testing.T, f fixture, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeedAddsTheNewFixedCategoryToAnAlreadySeededUser(t *testing.T) {
	repo, pool := newRepo(t)
	her := addUser(t, pool, "her@example.com")
	// The state of a user seeded before migration 000024's data step: 13 categories.
	if err := repo.SeedDefaults(ctx, her); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM personal_categories WHERE user_id = $1::text::uuid AND name = 'Gastos futuros'`, her); err != nil {
		t.Fatal(err)
	}
	gastos, _ := repo.ListCategories(ctx, her, domain.KindExpense)
	ropa := find(t, gastos, "Ropa")
	if _, err := repo.RenameCategory(ctx, her, ropa.ID, "Vestuario"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.SeedDefaults(ctx, her); err != nil {
			t.Fatal(err)
		}
	}
	savings, _ := repo.ListCategories(ctx, her, domain.KindSavings)
	if got := names(savings); len(got) != 3 || got[2] != "Gastos futuros" {
		t.Errorf("savings categories = %v, want Ahorro, Inversión, Gastos futuros", got)
	}
	after, _ := repo.ListCategories(ctx, her, domain.KindExpense)
	if len(after) != 8 {
		t.Errorf("Gasto categories = %d, want 8 (the renamed default is not re-added)", len(after))
	}
	for _, c := range after {
		if c.Name == "Ropa" {
			t.Error("the renamed Gasto default came back")
		}
	}
}

func TestMigrationAddsGastosFuturosToSeededUsers(t *testing.T) {
	repo, pool := newRepo(t)
	her := addUser(t, pool, "her@example.com")
	_ = repo.SeedDefaults(ctx, her)
	if _, err := pool.Exec(ctx, `DELETE FROM personal_categories WHERE user_id = $1::text::uuid AND name = 'Gastos futuros'`, her); err != nil {
		t.Fatal(err)
	}
	// The data step of the migration is idempotent and only touches seeded users.
	step := `INSERT INTO personal_categories (user_id, kind, name)
		SELECT DISTINCT user_id, 'Ahorro', 'Gastos futuros' FROM personal_categories ON CONFLICT DO NOTHING`
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, step); err != nil {
			t.Fatal(err)
		}
	}
	savings, _ := repo.ListCategories(ctx, her, domain.KindSavings)
	if len(savings) != 3 {
		t.Errorf("savings categories = %v", names(savings))
	}
}

func TestFutureExpenseCRUDAndScoping(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	if it.Status != domain.FutureActive || !it.Saved.IsZero() || it.Name != "Viaje" || !it.Target.Equal(d("1000")) {
		t.Fatalf("created = %+v", it)
	}
	up, err := f.repo.UpdateFutureExpense(ctx, f.her, it.ID, domain.FutureValidated{Name: "Vacaciones", Target: d("1500.50"), DueDate: "2027-01-10"})
	if err != nil || up.Name != "Vacaciones" || !up.Target.Equal(d("1500.50")) || up.DueDate != "2027-01-10" {
		t.Errorf("update = %+v, %v", up, err)
	}
	if _, err := f.repo.GetFutureExpense(ctx, f.other, it.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign get: %v; want ErrNotFound", err)
	}
	if _, err := f.repo.UpdateFutureExpense(ctx, f.other, it.ID, validItem("x", "1")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign update: %v; want ErrNotFound", err)
	}
	if err := f.repo.DeleteFutureExpense(ctx, f.other, it.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign delete: %v; want ErrNotFound", err)
	}
	if got, _ := f.repo.ListFutureExpenses(ctx, f.other); len(got) != 0 {
		t.Errorf("another user lists %d items", len(got))
	}
	if _, err := f.repo.ContributeFuture(ctx, f.other, it.ID, app.SavingRow{Amount: d("5"), Date: "2026-10-01", Description: "x", Linked: true}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign contribute: %v; want ErrNotFound", err)
	}
	if err := f.repo.AssignFuture(ctx, f.other, it.ID, d("5"), "2026-10-01", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign assign: %v; want ErrNotFound", err)
	}
	cats, _ := f.repo.ListCategories(ctx, f.other, domain.KindExpense)
	if _, _, err := f.repo.PayFuture(ctx, f.other, it.ID, settle(cats[0], "5")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign pay: %v; want ErrNotFound", err)
	}
	if err := f.repo.DeleteFutureExpense(ctx, f.her, it.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DeleteFutureExpense(ctx, f.her, it.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete: %v; want ErrNotFound", err)
	}
}

func TestSavedIsTheNetOfLinkedAhorroAndFreeBalanceOfTheUnlinked(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	f.save(t, f.her, it.ID, "400")
	f.save(t, f.her, it.ID, "-100") // a linked withdrawal
	f.seedFree(t, f.her, "250")
	f.seedFree(t, f.other, "999") // never counted for her
	got, _ := f.repo.GetFutureExpense(ctx, f.her, it.ID)
	if !got.Saved.Equal(d("300")) {
		t.Errorf("saved = %s, want 300", got.Saved)
	}
	if free := f.free(t, f.her); !free.Equal(d("250")) {
		t.Errorf("free balance = %s, want 250", free)
	}
	// Ahorro of another category never counts as free balance.
	if _, err := f.repo.CreateMovement(ctx, f.her, mv(domain.KindSavings, f.ahorro, "5000", "2026-10-01")); err != nil {
		t.Fatal(err)
	}
	if free := f.free(t, f.her); !free.Equal(d("250")) {
		t.Errorf("free balance = %s after an ordinary saving, want 250", free)
	}
}

func TestAssignIsAtomicAndChecksTheFreeBalance(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	f.seedFree(t, f.her, "300")
	if err := f.repo.AssignFuture(ctx, f.her, it.ID, d("300.01"), "2026-10-01", "Asignado"); !errors.Is(err, domain.ErrInsufficientFreeBalance) {
		t.Fatalf("over the balance: %v; want ErrInsufficientFreeBalance", err)
	}
	if got, _ := f.repo.GetFutureExpense(ctx, f.her, it.ID); !got.Saved.IsZero() || !f.free(t, f.her).Equal(d("300")) {
		t.Fatal("a rejected assignment must write nothing")
	}
	if err := f.repo.AssignFuture(ctx, f.her, it.ID, d("120"), "2026-10-01", "Asignado"); err != nil {
		t.Fatal(err)
	}
	got, _ := f.repo.GetFutureExpense(ctx, f.her, it.ID)
	if !got.Saved.Equal(d("120")) || !f.free(t, f.her).Equal(d("180")) {
		t.Errorf("saved %s free %s; want 120 and 180", got.Saved, f.free(t, f.her))
	}
	// The portfolio (sum of every Ahorro row) did not move: it is a transfer.
	var total string
	if err := f.pool.QueryRow(ctx, `SELECT SUM(amount)::text FROM personal_movements WHERE user_id = $1::text::uuid AND kind = 'Ahorro'`, f.her).Scan(&total); err != nil || !d(total).Equal(d("300")) {
		t.Errorf("savings total = %s, %v; want 300", total, err)
	}
}

func TestConcurrentAssignmentsNeverOverdrawTheFreeBalance(t *testing.T) {
	f := setup(t)
	a, b := f.item(t, f.her, "1000"), f.item(t, f.her, "1000")
	f.seedFree(t, f.her, "100")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []int{a.ID, b.ID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- f.repo.AssignFuture(ctx, f.her, id, d("80"), "2026-10-01", "Asignado")
		}()
	}
	wg.Wait()
	close(results)
	ok, insufficient := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrInsufficientFreeBalance):
			insufficient++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || insufficient != 1 {
		t.Errorf("ok %d insufficient %d; want exactly one of each", ok, insufficient)
	}
	if free := f.free(t, f.her); free.IsNegative() {
		t.Errorf("free balance = %s, must never go negative", free)
	}
}

func TestPayReleasesSavedAndReturnsTheRemainder(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	f.save(t, f.her, it.ID, "500")
	f.seedFree(t, f.her, "50")
	rows := []app.SavingRow{
		{Amount: d("-500"), Date: "2026-10-05", Description: "Pago de Viaje", Linked: true, Settlement: true},
		{Amount: d("200"), Date: "2026-10-05", Description: "Sobrante de Viaje", Settlement: true},
	}
	paid, expense, err := f.repo.PayFuture(ctx, f.her, it.ID, settle(f.ropa, "300", rows...))
	if err != nil {
		t.Fatal(err)
	}
	if paid.Status != domain.FuturePaid || paid.PaidAt != "2026-10-05" || paid.AmountPaid == nil || !paid.AmountPaid.Equal(d("300")) ||
		paid.ExpenseMovementID == nil || *paid.ExpenseMovementID != expense.ID || !paid.Saved.IsZero() {
		t.Errorf("paid = %+v", paid)
	}
	if expense.Kind != domain.KindExpense || expense.CategoryName != "Ropa" || !expense.Amount.Equal(d("300")) || expense.FutureExpenseID != nil {
		t.Errorf("expense = %+v", expense)
	}
	if free := f.free(t, f.her); !free.Equal(d("250")) {
		t.Errorf("free balance = %s, want 50 + 200 remainder", free)
	}
	// The markers round trip and her KPIs leave the covered part out while the
	// raw Ahorro rows (free balance, item totals) stay as they were.
	if !expense.CoveredBySavings.Equal(d("300")) {
		t.Errorf("covered by savings = %s, want 300", expense.CoveredBySavings)
	}
	month, err := f.repo.KPIMovements(ctx, f.her, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	settlements := 0
	for _, m := range month {
		if m.SavingsSettlement {
			settlements++
		}
	}
	// The 500 contribution of the cycle stays a saving; the payment adds nothing.
	if kpi := domain.KPITotals(month); settlements != 2 || !kpi.Expenses.IsZero() || !kpi.Savings.Equal(d("500")) {
		t.Errorf("KPIs = %+v with %d settlement rows, want nothing spent, 500 saved and 2 rows", kpi, settlements)
	}
	if _, _, err := f.repo.PayFuture(ctx, f.her, it.ID, settle(f.ropa, "300")); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("second payment: %v; want ErrAlreadyPaid", err)
	}
	if _, err := f.repo.UpdateFutureExpense(ctx, f.her, it.ID, validItem("x", "1")); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("update of a paid item: %v; want ErrAlreadyPaid", err)
	}
	if _, err := f.repo.ContributeFuture(ctx, f.her, it.ID, app.SavingRow{Amount: d("5"), Date: "2026-10-06", Description: "x", Linked: true}); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("contribute to a paid item: %v; want ErrAlreadyPaid", err)
	}
	if err := f.repo.AssignFuture(ctx, f.her, it.ID, d("5"), "2026-10-06", "x"); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("assign to a paid item: %v; want ErrAlreadyPaid", err)
	}
}

func TestPayRollsBackEverythingOnFailure(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	f.save(t, f.her, it.ID, "500")
	before := count(t, f, `SELECT count(*) FROM personal_movements`)

	cases := map[string]func(domain.FutureExpense) (app.Settlement, error){
		// The Gasto is fine, the second release row breaks the sign CHECK (Ahorro 0).
		"a release row fails": settle(f.ropa, "300",
			app.SavingRow{Amount: d("-500"), Date: "2026-10-05", Description: "Pago", Linked: true},
			app.SavingRow{Amount: d("0"), Date: "2026-10-05", Description: "Sobrante"}),
		// The category belongs to another user: the composite FK rejects the Gasto.
		"a foreign category": settle(f.otherRopa, "300"),
		"the plan fails": func(domain.FutureExpense) (app.Settlement, error) {
			return app.Settlement{}, errors.New("plan failed")
		},
	}
	for name, plan := range cases {
		if _, _, err := f.repo.PayFuture(ctx, f.her, it.ID, plan); err == nil {
			t.Errorf("%s: payment succeeded", name)
		}
		if got := count(t, f, `SELECT count(*) FROM personal_movements`); got != before {
			t.Errorf("%s: %d movements, want %d (rolled back)", name, got, before)
		}
		got, _ := f.repo.GetFutureExpense(ctx, f.her, it.ID)
		if got.Status != domain.FutureActive || got.ExpenseMovementID != nil || !got.Saved.Equal(d("500")) {
			t.Errorf("%s: item changed: %+v", name, got)
		}
	}
}

func TestDeleteUnlinksSavingsKeepingTheUserAndTheExpense(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	f.save(t, f.her, it.ID, "400")
	if err := f.repo.DeleteFutureExpense(ctx, f.her, it.ID); err != nil {
		t.Fatal(err)
	}
	if free := f.free(t, f.her); !free.Equal(d("400")) {
		t.Errorf("free balance = %s, want the 400 back", free)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE user_id = $1::text::uuid AND future_expense_id IS NULL AND kind = 'Ahorro'`, f.her); n != 1 {
		t.Errorf("unlinked rows of hers = %d, want 1 (user_id must not be nulled)", n)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE user_id IS NULL`); n != 0 {
		t.Errorf("%d rows lost their user_id", n)
	}

	// A paid item: deleting it never touches the registered Gasto, and the free
	// balance is unchanged (its linked rows net to zero).
	paidItem := f.item(t, f.her, "1000")
	f.save(t, f.her, paidItem.ID, "500")
	paid, expense, err := f.repo.PayFuture(ctx, f.her, paidItem.ID, settle(f.ropa, "300",
		app.SavingRow{Amount: d("-500"), Date: "2026-10-05", Description: "Pago", Linked: true},
		app.SavingRow{Amount: d("200"), Date: "2026-10-05", Description: "Sobrante"}))
	if err != nil || paid.ExpenseMovementID == nil {
		t.Fatal(err)
	}
	freeBefore := f.free(t, f.her)
	if err := f.repo.DeleteFutureExpense(ctx, f.her, paidItem.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE id = $1`, int64(expense.ID)); n != 1 {
		t.Error("the Gasto of a paid item must survive its deletion")
	}
	if free := f.free(t, f.her); !free.Equal(freeBefore) {
		t.Errorf("free balance moved from %s to %s on deleting a paid item", freeBefore, free)
	}
}

func TestDeletingTheExpenseClearsOnlyTheLink(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	_, expense, err := f.repo.PayFuture(ctx, f.her, it.ID, settle(f.ropa, "300"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DeleteMovement(ctx, f.her, expense.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.GetFutureExpense(ctx, f.her, it.ID)
	if err != nil || got.Status != domain.FuturePaid || got.ExpenseMovementID != nil {
		t.Errorf("item = %+v, %v; want paid with no expense link", got, err)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_future_expenses WHERE user_id IS NULL`); n != 0 {
		t.Error("user_id was nulled")
	}
}

func TestSchemaRejectsCrossUserLinksAndNonAhorroLinks(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	var otherSavings int
	cats, _ := f.repo.ListCategories(ctx, f.other, domain.KindSavings)
	otherSavings = find(t, cats, "Ahorro").ID
	// The other user cannot link a row to her item (composite FK on user_id).
	_, err := f.pool.Exec(ctx, `
		INSERT INTO personal_movements (user_id, kind, category_id, amount, description, date, future_expense_id)
		VALUES ($1::text::uuid, 'Ahorro', $2, 10, 'x', '2026-10-01', $3)`, f.other, int64(otherSavings), int64(it.ID))
	if err == nil {
		t.Error("a row of another user linked to her item was accepted")
	}
	// A Gasto cannot carry the link.
	_, err = f.pool.Exec(ctx, `
		INSERT INTO personal_movements (user_id, kind, category_id, amount, description, date, future_expense_id)
		VALUES ($1::text::uuid, 'Gasto', $2, 10, 'x', '2026-10-01', $3)`, f.her, int64(f.ropa.ID), int64(it.ID))
	if err == nil {
		t.Error("a Gasto linked to an item was accepted")
	}
	// The paid columns follow the status.
	_, err = f.pool.Exec(ctx, `UPDATE personal_future_expenses SET status = 'paid' WHERE id = $1`, int64(it.ID))
	if err == nil {
		t.Error("a paid item without paid_at was accepted")
	}
	// Her expense link cannot point at another user's movement.
	m, _ := f.repo.CreateMovement(ctx, f.other, mv(domain.KindExpense, f.otherRopa, "10", "2026-10-01"))
	_, err = f.pool.Exec(ctx, `UPDATE personal_future_expenses SET status = 'paid', paid_at = '2026-10-01', amount_paid = 10, expense_movement_id = $2 WHERE id = $1`,
		int64(it.ID), int64(m.ID))
	if err == nil {
		t.Error("an expense link to another user's movement was accepted")
	}
}

func TestManualUpdateKeepsTheLinkAndListExposesIt(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	saving, err := f.repo.ContributeFuture(ctx, f.her, it.ID, app.SavingRow{Amount: d("100"), Date: "2026-10-01", Description: "ahorro", Linked: true})
	if err != nil || saving.FutureExpenseID == nil || *saving.FutureExpenseID != it.ID {
		t.Fatalf("contribute = %+v, %v", saving, err)
	}
	cats, _ := f.repo.ListCategories(ctx, f.her, domain.KindSavings)
	gf := find(t, cats, domain.SavingsCategory)
	up, err := f.repo.UpdateMovement(ctx, f.her, saving.ID, mv(domain.KindSavings, gf, "150", "2026-10-02"))
	if err != nil || up.FutureExpenseID == nil || *up.FutureExpenseID != it.ID || !up.Amount.Equal(d("150")) {
		t.Errorf("update = %+v, %v; the link must stay", up, err)
	}
	// Turning a linked saving into a Gasto is rejected instead of dropping the link.
	if _, err := f.repo.UpdateMovement(ctx, f.her, saving.ID, mv(domain.KindExpense, f.ropa, "150", "2026-10-02")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("kind change of a linked saving: %v; want ErrInvalidInput", err)
	}
	listed, _ := f.repo.ListMovements(ctx, f.her, app.Filter{From: "2026-10-01", To: "2026-10-31", Kind: domain.KindSavings})
	if len(listed) != 1 || listed[0].FutureExpenseID == nil || *listed[0].FutureExpenseID != it.ID {
		t.Errorf("listed = %+v", listed)
	}
	// Deleting a linked Ahorro is allowed, like the owner's savings.
	if err := f.repo.DeleteMovement(ctx, f.her, saving.ID); err != nil {
		t.Errorf("delete of a linked saving: %v", err)
	}
	if got, _ := f.repo.GetFutureExpense(ctx, f.her, it.ID); !got.Saved.IsZero() {
		t.Errorf("saved = %s after deleting the saving", got.Saved)
	}
}

func (f fixture) addPaymentWith(t *testing.T, name string, saved, paid string) (domain.FutureExpense, domain.Movement) {
	t.Helper()
	it, err := f.repo.CreateFutureExpense(ctx, f.her, validItem(name, "1000"))
	if err != nil {
		t.Fatal(err)
	}
	if saved != "0" {
		f.save(t, f.her, it.ID, saved)
	}
	var rows []app.SavingRow
	linked, remainder := domain.Release(d(saved), d(paid))
	if linked.IsPositive() {
		rows = append(rows, app.SavingRow{Amount: linked.Neg(), Date: "2026-10-05", Description: "Pago de " + name, Linked: true})
	}
	if remainder.IsPositive() {
		rows = append(rows, app.SavingRow{Amount: remainder, Date: "2026-10-05", Description: "Sobrante de " + name})
	}
	paidItem, expense, err := f.repo.PayFuture(ctx, f.her, it.ID, settle(f.ropa, paid, rows...))
	if err != nil {
		t.Fatal(err)
	}
	return paidItem, expense
}

// The data step of migration 000030 marks the payments made before the
// markers existed: release and remainder rows as settlements and the Gasto with
// min(saved, paid). It is idempotent and leaves manual rows alone.
func TestMigrationBackfillsAlreadyPaidItems(t *testing.T) {
	f := setup(t)
	_, over := f.addPaymentWith(t, "Viaje", "500", "300")
	_, under := f.addPaymentWith(t, "Laptop", "100", "300")
	_, none := f.addPaymentWith(t, "Cena", "0", "80")
	// Manual rows that look similar but are not payments.
	cats, _ := f.repo.ListCategories(ctx, f.her, domain.KindSavings)
	gf := find(t, cats, domain.SavingsCategory)
	manual := mv(domain.KindSavings, gf, "70", "2026-10-06") // another day: not part of the payment
	manual.Description = "Sobrante de Viaje"
	if _, err := f.repo.CreateMovement(ctx, f.her, manual); err != nil {
		t.Fatal(err)
	}
	manual.Date = "2026-10-05"
	manual.Description = "ordinario"
	manual.Amount = d("-20")
	if _, err := f.repo.CreateMovement(ctx, f.her, manual); err != nil {
		t.Fatal(err)
	}

	// Go back to the legacy shape and run the up migration data steps again.
	root := filepath.Join("..", "..", "..", "..", "..", "migrations")
	down, err := os.ReadFile(filepath.Join(root, "000030_personal_savings_covered_expenses.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(filepath.Join(root, "000030_personal_savings_covered_expenses.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	if _, err := f.pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("up: %v", err)
	}

	covered := func(id int) string {
		var s *string
		if err := f.pool.QueryRow(ctx, `SELECT covered_by_savings::text FROM personal_movements WHERE id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		if s == nil {
			return "none"
		}
		return *s
	}
	if g := covered(over.ID); g != "300.00" {
		t.Errorf("saved more than paid: covered = %s, want 300.00", g)
	}
	if g := covered(under.ID); g != "100.00" {
		t.Errorf("saved less than paid: covered = %s, want 100.00 (what existed)", g)
	}
	if g := covered(none.ID); g != "none" {
		t.Errorf("nothing saved: covered = %s, want none", g)
	}
	// The two releases and the remainder of "Viaje", never the manual rows.
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE savings_settlement`); n != 3 {
		t.Errorf("settlement rows = %d, want 3 (2 releases + 1 remainder)", n)
	}
	// The raw readers are unchanged.
	if free := f.free(t, f.her); !free.Equal(d("250")) {
		t.Errorf("free balance = %s, want 200 remainder + 70 - 20", free)
	}
}

func TestSchemaRejectsMisplacedSavingsMarkers(t *testing.T) {
	f := setup(t)
	bad := mv(domain.KindSavings, f.ahorro, "5", "2026-10-05")
	bad.CoveredBySavings = d("5")
	if _, err := f.repo.CreateMovement(ctx, f.her, bad); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("covered on an Ahorro: %v; want ErrInvalidInput", err)
	}
	bad = mv(domain.KindExpense, f.ropa, "5", "2026-10-05")
	bad.SavingsSettlement = true
	if _, err := f.repo.CreateMovement(ctx, f.her, bad); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("settlement on a Gasto: %v; want ErrInvalidInput", err)
	}
	// An update never writes the markers, and a Gasto paid with savings cannot become another kind.
	ok := mv(domain.KindExpense, f.ropa, "100", "2026-10-05")
	ok.CoveredBySavings = d("60")
	m, err := f.repo.CreateMovement(ctx, f.her, ok)
	if err != nil || !m.CoveredBySavings.Equal(d("60")) {
		t.Fatalf("create covered Gasto = %+v, %v", m, err)
	}
	edited := mv(domain.KindExpense, f.ropa, "90", "2026-10-05")
	up, err := f.repo.UpdateMovement(ctx, f.her, m.ID, edited)
	if err != nil || !up.CoveredBySavings.Equal(d("60")) || !up.Amount.Equal(d("90")) {
		t.Errorf("update = %+v, %v; want the coverage kept", up, err)
	}
	if _, err := f.repo.UpdateMovement(ctx, f.her, m.ID, mv(domain.KindIncome, f.trabajo, "90", "2026-10-05")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("kind change of a covered Gasto: %v; want ErrInvalidInput", err)
	}
}

func TestAssignmentRowsShareATransferIDAndDeleteTogether(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	f.seedFree(t, f.her, "300")
	if err := f.repo.AssignFuture(ctx, f.her, it.ID, d("120"), "2026-10-01", "Asignado a Viaje"); err != nil {
		t.Fatal(err)
	}
	rows, err := f.repo.ListMovements(ctx, f.her, app.Filter{From: "2026-10-01", To: "2026-10-01"})
	if err != nil || len(rows) != 2 || rows[0].TransferID == "" || rows[0].TransferID != rows[1].TransferID {
		t.Fatalf("rows = %+v, %v; want two sharing a transfer id", rows, err)
	}
	if err := f.repo.DeleteMovement(ctx, f.other, rows[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another user deleting: %v, want ErrNotFound", err)
	}
	if err := f.repo.DeleteMovement(ctx, f.her, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ := f.repo.GetFutureExpense(ctx, f.her, it.ID)
	if !got.Saved.IsZero() || !f.free(t, f.her).Equal(d("300")) {
		t.Errorf("saved %s free %s; want 0 and 300 after deleting the pair", got.Saved, f.free(t, f.her))
	}
}

// Migration 000031 links the assignment pairs created before transfer_id
// existed: duplicates (same day, same amount) pair in id order and a row with
// no counterpart stays unlinked. Down unlinks them again.
func TestMigrationLinksExistingAssignmentPairs(t *testing.T) {
	f := setup(t)
	it := f.item(t, f.her, "1000")
	f.seedFree(t, f.her, "500")
	for i := 0; i < 2; i++ {
		if err := f.repo.AssignFuture(ctx, f.her, it.ID, d("100"), "2026-10-02", "Asignado a Viaje"); err != nil {
			t.Fatal(err)
		}
	}
	cats, _ := f.repo.ListCategories(ctx, f.her, domain.KindSavings)
	gf := find(t, cats, domain.SavingsCategory)
	lonely := mv(domain.KindSavings, gf, "-30", "2026-10-03")
	lonely.Description = "Asignado a Viaje"
	if _, err := f.repo.CreateMovement(ctx, f.her, lonely); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join("..", "..", "..", "..", "..", "migrations")
	for _, name := range []string{"000031_future_expense_assignment_transfer_id.down.sql", "000031_future_expense_assignment_transfer_id.up.sql"} {
		sql, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE transfer_id IS NOT NULL`); n != 4 {
		t.Errorf("linked rows = %d, want 4 (two pairs; the lonely row stays unlinked)", n)
	}
	if n := count(t, f, `SELECT count(*) FROM (SELECT transfer_id FROM personal_movements WHERE transfer_id IS NOT NULL GROUP BY transfer_id HAVING count(*) = 2 AND sum(amount) = 0 AND count(future_expense_id) = 1) g`); n != 2 {
		t.Errorf("balanced pairs = %d, want 2", n)
	}
	if got := f.free(t, f.her); !got.Equal(d("270")) {
		t.Errorf("free = %s, want 270: linking changes no balance", got)
	}
}
