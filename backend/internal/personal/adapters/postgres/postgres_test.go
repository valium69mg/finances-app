package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var ctx = context.Background()

// newRepo returns the repository on a throwaway schema holding a fresh copy of
// the migrations it depends on. It skips the test when TEST_DATABASE_URL is
// not set.
func newRepo(t *testing.T) (*postgres.Repo, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "personal_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, name := range []string{"000002_users.up.sql", "000006_movements.up.sql", "000009_movements_transfer_id.up.sql", "000016_future_expenses.up.sql", "000017_users_roles.up.sql", "000018_expense_requests.up.sql", "000022_personal_finances.up.sql", "000023_personal_future_expenses.up.sql", "000024_personal_wishlist.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return postgres.NewRepo(pool), pool
}

func addUser(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, verified, role, active) VALUES ($1, 'x', true, 'household', true) RETURNING id::text`,
		email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func names(cats []domain.Category) []string {
	out := make([]string, len(cats))
	for i, c := range cats {
		out[i] = c.Name
	}
	return out
}

func find(t *testing.T, cats []domain.Category, name string) domain.Category {
	t.Helper()
	for _, c := range cats {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("category %q not found in %v", name, names(cats))
	return domain.Category{}
}

func TestSeedDefaultsOrderAndIdempotency(t *testing.T) {
	repo, pool := newRepo(t)
	her := addUser(t, pool, "her@example.com")
	for i := 0; i < 3; i++ {
		if err := repo.SeedDefaults(ctx, her); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	all, err := repo.ListCategories(ctx, her, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 14 {
		t.Fatalf("got %d categories (%v), want 14", len(all), names(all))
	}
	want := []string{"Trabajo", "Transferencia", "Otros",
		"Belleza", "Salud", "Tratamientos", "Tienda de conveniencia", "Ropa", "Pintura", "Maquillaje", "Compras en línea",
		"Ahorro", "Inversión", "Gastos futuros"}
	for i, n := range names(all) {
		if n != want[i] {
			t.Fatalf("order = %v, want %v", names(all), want)
		}
	}
	gastos, _ := repo.ListCategories(ctx, her, domain.KindExpense)
	if len(gastos) != 8 {
		t.Errorf("Gasto filter returned %d, want 8", len(gastos))
	}
}

func TestSeedDefaultsUnderConcurrency(t *testing.T) {
	repo, pool := newRepo(t)
	her := addUser(t, pool, "her@example.com")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.SeedDefaults(ctx, her)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent seed failed: %v", err)
		}
	}
	if all, _ := repo.ListCategories(ctx, her, ""); len(all) != 14 {
		t.Errorf("got %d categories after concurrent seeds, want 14", len(all))
	}
}

func TestSeedNeverReaddsARenamedDefault(t *testing.T) {
	repo, pool := newRepo(t)
	her := addUser(t, pool, "her@example.com")
	_ = repo.SeedDefaults(ctx, her)
	gastos, _ := repo.ListCategories(ctx, her, domain.KindExpense)
	ropa := find(t, gastos, "Ropa")
	if _, err := repo.RenameCategory(ctx, her, ropa.ID, "Vestuario"); err != nil {
		t.Fatal(err)
	}
	_ = repo.SeedDefaults(ctx, her)
	after, _ := repo.ListCategories(ctx, her, domain.KindExpense)
	if len(after) != 8 {
		t.Errorf("got %d Gasto categories after reseeding, want 8", len(after))
	}
	for _, c := range after {
		if c.Name == "Ropa" {
			t.Error("the renamed default was added back")
		}
	}
}

func TestSeedIsPerUser(t *testing.T) {
	repo, pool := newRepo(t)
	her, other := addUser(t, pool, "her@example.com"), addUser(t, pool, "other@example.com")
	_ = repo.SeedDefaults(ctx, her)
	if got, _ := repo.ListCategories(ctx, other, ""); len(got) != 0 {
		t.Errorf("another user sees %d categories before her own seed", len(got))
	}
	_ = repo.SeedDefaults(ctx, other)
	if got, _ := repo.ListCategories(ctx, other, ""); len(got) != 14 {
		t.Errorf("second user got %d categories, want 14", len(got))
	}
}

func TestCategoryCreateRenameAndDuplicates(t *testing.T) {
	repo, pool := newRepo(t)
	her, other := addUser(t, pool, "her@example.com"), addUser(t, pool, "other@example.com")
	_ = repo.SeedDefaults(ctx, her)
	_ = repo.SeedDefaults(ctx, other)

	c, err := repo.CreateCategory(ctx, her, "Café")
	if err != nil || c.Kind != domain.KindExpense {
		t.Fatalf("create: %+v, %v", c, err)
	}
	if _, err := repo.CreateCategory(ctx, her, "Café"); !errors.Is(err, domain.ErrDuplicateCategory) {
		t.Errorf("duplicate create error = %v; want ErrDuplicateCategory", err)
	}
	// The same name is free for another user.
	if _, err := repo.CreateCategory(ctx, other, "Café"); err != nil {
		t.Errorf("another user could not use the same name: %v", err)
	}
	gastos, _ := repo.ListCategories(ctx, her, domain.KindExpense)
	if _, err := repo.RenameCategory(ctx, her, c.ID, "Salud"); !errors.Is(err, domain.ErrDuplicateCategory) {
		t.Errorf("rename onto an existing name error = %v; want ErrDuplicateCategory", err)
	}
	got, err := repo.RenameCategory(ctx, her, find(t, gastos, "Belleza").ID, "Cuidado personal")
	if err != nil || got.Name != "Cuidado personal" {
		t.Errorf("rename: %+v, %v", got, err)
	}
	// Scoping: another user's id is not found, and a fixed kind is not renamed by the repo.
	if _, err := repo.RenameCategory(ctx, other, c.ID, "X"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("rename of another user's category error = %v; want ErrNotFound", err)
	}
	if _, err := repo.GetCategory(ctx, other, c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("get of another user's category error = %v; want ErrNotFound", err)
	}
	all, _ := repo.ListCategories(ctx, her, domain.KindIncome)
	if _, err := repo.RenameCategory(ctx, her, find(t, all, "Trabajo").ID, "X"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("rename of a fixed category error = %v; want ErrNotFound", err)
	}
}

type fixture struct {
	repo       *postgres.Repo
	pool       *pgxpool.Pool
	her, other string
	ropa       domain.Category
	trabajo    domain.Category
	ahorro     domain.Category
	otherRopa  domain.Category
}

func setup(t *testing.T) fixture {
	repo, pool := newRepo(t)
	f := fixture{repo: repo, pool: pool, her: addUser(t, pool, "her@example.com"), other: addUser(t, pool, "other@example.com")}
	for _, u := range []string{f.her, f.other} {
		if err := repo.SeedDefaults(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	mine, _ := repo.ListCategories(ctx, f.her, "")
	f.ropa, f.trabajo, f.ahorro = find(t, mine, "Ropa"), find(t, mine, "Trabajo"), find(t, mine, "Ahorro")
	theirs, _ := repo.ListCategories(ctx, f.other, "")
	f.otherRopa = find(t, theirs, "Ropa")
	return f
}

func mv(kind domain.Kind, cat domain.Category, amount, date string) domain.ValidMovement {
	return domain.ValidMovement{Kind: kind, CategoryID: cat.ID, Amount: d(amount), Description: "row " + date, Date: date}
}

func TestMovementCRUDAndScoping(t *testing.T) {
	f := setup(t)
	m, err := f.repo.CreateMovement(ctx, f.her, mv(domain.KindExpense, f.ropa, "199.90", "2026-10-04"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.CategoryName != "Ropa" || !m.Amount.Equal(d("199.90")) || m.Date != "2026-10-04" || m.Kind != domain.KindExpense {
		t.Errorf("created = %+v", m)
	}
	up, err := f.repo.UpdateMovement(ctx, f.her, m.ID, mv(domain.KindIncome, f.trabajo, "2500", "2026-10-06"))
	if err != nil || up.Kind != domain.KindIncome || up.CategoryName != "Trabajo" || !up.UpdatedAt.After(m.CreatedAt.Add(-1)) {
		t.Errorf("update = %+v, %v", up, err)
	}

	// Another user cannot see, change or delete her row.
	if _, err := f.repo.UpdateMovement(ctx, f.other, m.ID, mv(domain.KindExpense, f.otherRopa, "1", "2026-10-06")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign update error = %v; want ErrNotFound", err)
	}
	if err := f.repo.DeleteMovement(ctx, f.other, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign delete error = %v; want ErrNotFound", err)
	}
	wide := app.Filter{From: "2000-01-01", To: "2100-01-01"}
	if got, _ := f.repo.ListMovements(ctx, f.other, wide); len(got) != 0 {
		t.Errorf("another user lists %d rows, want 0", len(got))
	}
	if err := f.repo.DeleteMovement(ctx, f.her, m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := f.repo.DeleteMovement(ctx, f.her, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete error = %v; want ErrNotFound", err)
	}
}

func TestMovementCategoryMustBelongToTheUserAndKind(t *testing.T) {
	f := setup(t)
	cases := map[string]domain.ValidMovement{
		"another user's category":   mv(domain.KindExpense, f.otherRopa, "10", "2026-10-04"),
		"kind differs from the row": mv(domain.KindIncome, f.ropa, "10", "2026-10-04"),
		"unknown category":          {Kind: domain.KindExpense, CategoryID: 999999, Amount: d("10"), Description: "x", Date: "2026-10-04"},
	}
	for name, v := range cases {
		if _, err := f.repo.CreateMovement(ctx, f.her, v); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("create with %s: error = %v; want ErrInvalidInput", name, err)
		}
	}
	m, _ := f.repo.CreateMovement(ctx, f.her, mv(domain.KindExpense, f.ropa, "10", "2026-10-04"))
	if _, err := f.repo.UpdateMovement(ctx, f.her, m.ID, mv(domain.KindExpense, f.otherRopa, "10", "2026-10-04")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("update to another user's category error = %v; want ErrInvalidInput", err)
	}
	if _, err := f.repo.UpdateMovement(ctx, f.her, m.ID, mv(domain.KindSavings, f.ropa, "10", "2026-10-04")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("update to a mismatched kind error = %v; want ErrInvalidInput", err)
	}
}

func TestAhorroSignRule(t *testing.T) {
	f := setup(t)
	for _, amount := range []string{"500", "-200.50"} {
		if _, err := f.repo.CreateMovement(ctx, f.her, mv(domain.KindSavings, f.ahorro, amount, "2026-10-04")); err != nil {
			t.Errorf("Ahorro %s rejected: %v", amount, err)
		}
	}
	for _, c := range []struct {
		kind   domain.Kind
		cat    domain.Category
		amount string
	}{
		{domain.KindSavings, f.ahorro, "0"},
		{domain.KindExpense, f.ropa, "-5"},
		{domain.KindExpense, f.ropa, "0"},
		{domain.KindIncome, f.trabajo, "-5"},
	} {
		if _, err := f.repo.CreateMovement(ctx, f.her, mv(c.kind, c.cat, c.amount, "2026-10-04")); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s %s: error = %v; want ErrInvalidInput from the CHECK", c.kind, c.amount, err)
		}
	}
	got, _ := f.repo.ListMovements(ctx, f.her, app.Filter{From: "2026-10-01", To: "2026-10-31", Kind: domain.KindSavings})
	if len(got) != 2 {
		t.Fatalf("got %d savings rows, want 2", len(got))
	}
	if !got[0].Amount.IsNegative() && !got[1].Amount.IsNegative() {
		t.Error("the withdrawal was not stored as a negative amount")
	}
}

func TestListMovementsRangeOrderAndFilters(t *testing.T) {
	f := setup(t)
	for _, v := range []domain.ValidMovement{
		mv(domain.KindExpense, f.ropa, "1", "2026-09-30"), // before the range
		mv(domain.KindExpense, f.ropa, "2", "2026-10-01"), // first day
		mv(domain.KindIncome, f.trabajo, "3", "2026-10-15"),
		mv(domain.KindExpense, f.ropa, "4", "2026-10-15"), // same day, created later
		mv(domain.KindExpense, f.ropa, "5", "2026-10-31"), // last day
		mv(domain.KindExpense, f.ropa, "6", "2026-11-01"), // after the range
	} {
		if _, err := f.repo.CreateMovement(ctx, f.her, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.repo.CreateMovement(ctx, f.other, mv(domain.KindExpense, f.otherRopa, "7", "2026-10-15")); err != nil {
		t.Fatal(err)
	}
	rng := app.Filter{From: "2026-10-01", To: "2026-10-31"}
	got, err := f.repo.ListMovements(ctx, f.her, rng)
	if err != nil {
		t.Fatal(err)
	}
	var amounts []string
	for _, m := range got {
		amounts = append(amounts, m.Amount.String())
	}
	want := []string{"5", "4", "3", "2"}
	if len(amounts) != len(want) {
		t.Fatalf("amounts = %v, want %v", amounts, want)
	}
	for i := range want {
		if amounts[i] != want[i] {
			t.Fatalf("amounts = %v, want %v (newest first, ties by id)", amounts, want)
		}
	}
	byKind, _ := f.repo.ListMovements(ctx, f.her, app.Filter{From: rng.From, To: rng.To, Kind: domain.KindIncome})
	if len(byKind) != 1 || byKind[0].CategoryName != "Trabajo" {
		t.Errorf("kind filter = %+v", byKind)
	}
	byCat, _ := f.repo.ListMovements(ctx, f.her, app.Filter{From: rng.From, To: rng.To, CategoryID: f.ropa.ID})
	if len(byCat) != 3 {
		t.Errorf("category filter returned %d rows, want 3", len(byCat))
	}
	empty, err := f.repo.ListMovements(ctx, f.her, app.Filter{From: "2030-01-01", To: "2030-01-31"})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty range = %v, %v; want an empty non-nil slice", empty, err)
	}
}

func TestRenameFollowsMovements(t *testing.T) {
	f := setup(t)
	_, _ = f.repo.CreateMovement(ctx, f.her, mv(domain.KindExpense, f.ropa, "10", "2026-10-04"))
	if _, err := f.repo.RenameCategory(ctx, f.her, f.ropa.ID, "Vestuario"); err != nil {
		t.Fatal(err)
	}
	got, _ := f.repo.ListMovements(ctx, f.her, app.Filter{From: "2026-10-01", To: "2026-10-31"})
	if len(got) != 1 || got[0].CategoryName != "Vestuario" {
		t.Errorf("movement category after rename = %+v", got)
	}
}

func TestKindTotalsAndSpentByCategory(t *testing.T) {
	f := setup(t)
	mine, _ := f.repo.ListCategories(ctx, f.her, "")
	salud := find(t, mine, "Salud")
	for _, v := range []domain.ValidMovement{
		mv(domain.KindIncome, f.trabajo, "10000.10", "2026-10-02"),
		mv(domain.KindExpense, f.ropa, "300.25", "2026-10-03"),
		mv(domain.KindExpense, f.ropa, "100", "2026-10-31"), // last day
		mv(domain.KindExpense, salud, "900", "2026-10-04"),
		mv(domain.KindSavings, f.ahorro, "2000", "2026-10-06"),
		mv(domain.KindSavings, f.ahorro, "-500.50", "2026-10-07"), // withdrawal
		mv(domain.KindExpense, f.ropa, "9999", "2026-09-30"),      // outside the cycle
		mv(domain.KindExpense, f.ropa, "9999", "2026-11-01"),      // outside the cycle
		mv(domain.KindIncome, f.trabajo, "9999", "2026-09-30"),    // outside the cycle
	} {
		if _, err := f.repo.CreateMovement(ctx, f.her, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.repo.CreateMovement(ctx, f.other, mv(domain.KindExpense, f.otherRopa, "5555", "2026-10-15")); err != nil {
		t.Fatal(err)
	}

	got, err := f.repo.KindTotals(ctx, f.her, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Income.Equal(d("10000.10")) || !got.Expenses.Equal(d("1300.25")) || !got.Savings.Equal(d("1499.50")) {
		t.Errorf("totals = %+v; want 10000.10 / 1300.25 / 1499.50", got)
	}
	if !got.Available().Equal(d("7200.35")) {
		t.Errorf("available = %s, want 7200.35", got.Available())
	}

	cats, err := f.repo.SpentByCategory(ctx, f.her, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 2 || cats[0].Category != "Salud" || !cats[0].Spent.Equal(d("900")) ||
		cats[1].CategoryID != f.ropa.ID || !cats[1].Spent.Equal(d("400.25")) {
		t.Errorf("categories = %+v; want Salud 900 then Ropa 400.25; categories without spending are left out", cats)
	}

	empty, err := f.repo.KindTotals(ctx, f.her, "2027-01-01", "2027-01-31")
	if err != nil || !empty.Available().IsZero() {
		t.Errorf("empty range = %+v, %v", empty, err)
	}
	none, err := f.repo.SpentByCategory(ctx, f.her, "2027-01-01", "2027-01-31")
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("empty range categories = %#v, %v; want an empty non-nil slice", none, err)
	}
	theirs, _ := f.repo.KindTotals(ctx, f.other, "2026-10-01", "2026-10-31")
	if !theirs.Expenses.Equal(d("5555")) || !theirs.Income.IsZero() {
		t.Errorf("another user's totals = %+v; each user must only see her own rows", theirs)
	}
}
