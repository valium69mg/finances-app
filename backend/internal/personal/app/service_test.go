package app_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fakeSettings struct {
	cfg settings.Config
	err error
}

func (f fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, f.err }

// fakeRepo is an in-memory Repo that mimics the scoping and ordering rules.
type fakeRepo struct {
	cats    []domain.Category
	owners  map[int]string
	moves   []domain.Movement
	mOwners map[int]string
	items   []*fakeItem
	rows    []fakeRow
	wishes  []*fakeWish
	linkErr error
	settled []app.Settlement
	nextCat int
	nextMov int
	seeds   int
	lastF   app.Filter
	err     error
}

func newRepo() *fakeRepo { return &fakeRepo{owners: map[int]string{}, mOwners: map[int]string{}} }

func (r *fakeRepo) addCat(user string, kind domain.Kind, name string) domain.Category {
	r.nextCat++
	c := domain.Category{ID: r.nextCat, Kind: kind, Name: name}
	r.cats = append(r.cats, c)
	r.owners[c.ID] = user
	return c
}

func (r *fakeRepo) SeedDefaults(_ context.Context, user string) error {
	r.seeds++
	for _, k := range domain.Kinds {
		has := false
		for _, c := range r.cats {
			if r.owners[c.ID] == user && c.Kind == k {
				has = true
			}
		}
		if has {
			continue
		}
		for _, n := range domain.Defaults[k] {
			r.addCat(user, k, n)
		}
	}
	return r.err
}

func (r *fakeRepo) ListCategories(_ context.Context, user string, kind domain.Kind) ([]domain.Category, error) {
	var out []domain.Category
	for _, c := range r.cats {
		if r.owners[c.ID] == user && (kind == "" || c.Kind == kind) {
			out = append(out, c)
		}
	}
	return out, r.err
}

func (r *fakeRepo) GetCategory(_ context.Context, user string, id int) (domain.Category, error) {
	for _, c := range r.cats {
		if c.ID == id && r.owners[id] == user {
			return c, nil
		}
	}
	return domain.Category{}, domain.ErrNotFound
}

func (r *fakeRepo) CreateCategory(_ context.Context, user, name string) (domain.Category, error) {
	for _, c := range r.cats {
		if r.owners[c.ID] == user && c.Kind == domain.KindExpense && c.Name == name {
			return domain.Category{}, domain.ErrDuplicateCategory
		}
	}
	return r.addCat(user, domain.KindExpense, name), nil
}

func (r *fakeRepo) RenameCategory(_ context.Context, user string, id int, name string) (domain.Category, error) {
	for i, c := range r.cats {
		if c.ID == id && r.owners[id] == user {
			r.cats[i].Name = name
			return r.cats[i], nil
		}
	}
	return domain.Category{}, domain.ErrNotFound
}

func (r *fakeRepo) ListMovements(_ context.Context, user string, f app.Filter) ([]domain.Movement, error) {
	r.lastF = f
	var out []domain.Movement
	for _, m := range r.moves {
		if r.mOwners[m.ID] != user || m.Date < f.From || m.Date > f.To {
			continue
		}
		if (f.Kind != "" && m.Kind != f.Kind) || (f.CategoryID != 0 && m.CategoryID != f.CategoryID) {
			continue
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, r.err
}

func (r *fakeRepo) KindTotals(_ context.Context, user string, from, to string) (domain.KindTotals, error) {
	var t domain.KindTotals
	for _, m := range r.moves {
		if r.mOwners[m.ID] != user || m.Date < from || m.Date > to {
			continue
		}
		switch m.Kind {
		case domain.KindIncome:
			t.Income = t.Income.Add(m.Amount)
		case domain.KindExpense:
			t.Expenses = t.Expenses.Add(m.Amount)
		case domain.KindSavings:
			t.Savings = t.Savings.Add(m.Amount)
		}
	}
	return t, r.err
}

func (r *fakeRepo) SpentByCategory(_ context.Context, user string, from, to string) ([]domain.CategorySpend, error) {
	sums := map[int]decimal.Decimal{}
	for _, m := range r.moves {
		if r.mOwners[m.ID] == user && m.Kind == domain.KindExpense && m.Date >= from && m.Date <= to {
			sums[m.CategoryID] = sums[m.CategoryID].Add(m.Amount)
		}
	}
	var out []domain.CategorySpend
	for _, c := range r.cats {
		if v, ok := sums[c.ID]; ok {
			out = append(out, domain.CategorySpend{CategoryID: c.ID, Category: c.Name, Spent: v})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Spent.GreaterThan(out[j].Spent) })
	return out, r.err
}

func (r *fakeRepo) CreateMovement(_ context.Context, user string, v domain.ValidMovement) (domain.Movement, error) {
	r.nextMov++
	m := domain.Movement{ID: r.nextMov, Kind: v.Kind, CategoryID: v.CategoryID, Amount: v.Amount, Description: v.Description, Date: v.Date}
	r.moves = append(r.moves, m)
	r.mOwners[m.ID] = user
	return m, r.err
}

func (r *fakeRepo) UpdateMovement(_ context.Context, user string, id int, v domain.ValidMovement) (domain.Movement, error) {
	for i, m := range r.moves {
		if m.ID == id && r.mOwners[id] == user {
			r.moves[i] = domain.Movement{ID: id, Kind: v.Kind, CategoryID: v.CategoryID, Amount: v.Amount, Description: v.Description, Date: v.Date}
			return r.moves[i], nil
		}
	}
	return domain.Movement{}, domain.ErrNotFound
}

func (r *fakeRepo) DeleteMovement(_ context.Context, user string, id int) error {
	for i, m := range r.moves {
		if m.ID == id && r.mOwners[id] == user {
			r.moves = append(r.moves[:i], r.moves[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

var (
	her    = session.Identity{UserID: "u-her", Role: session.RoleHousehold}
	other  = session.Identity{UserID: "u-other", Role: session.RoleHousehold}
	owner  = session.Identity{UserID: "u-owner", Role: session.RoleOwner}
	ctx    = context.Background()
	oct10  = func() time.Time { return time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC) }
	noCfg  = fakeSettings{}
	day15  = fakeSettings{cfg: settings.Config{CycleStartDay: 15}}
	errSet = errors.New("settings down")
)

func newSvc(r *fakeRepo, s fakeSettings) *app.Service { return app.NewService(r, s, oct10) }

func TestEveryOperationForbidsTheOwnerAndAnonymous(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	for _, actor := range []session.Identity{owner, {}, {UserID: "x", Role: "admin"}, {Role: session.RoleHousehold}} {
		calls := map[string]error{}
		_, calls["categories"] = svc.Categories(ctx, actor, "")
		_, calls["create category"] = svc.CreateCategory(ctx, actor, "x")
		_, calls["rename"] = svc.RenameCategory(ctx, actor, 1, "x")
		_, calls["movements"] = svc.Movements(ctx, actor, app.ListInput{})
		_, calls["create movement"] = svc.CreateMovement(ctx, actor, domain.MovementInput{})
		_, calls["update movement"] = svc.UpdateMovement(ctx, actor, 1, domain.MovementInput{})
		calls["delete movement"] = svc.DeleteMovement(ctx, actor, 1)
		_, calls["period"] = svc.Period(ctx, actor, "")
		_, calls["dashboard"] = svc.Dashboard(ctx, actor, "")
		for name, err := range calls {
			if !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("%s as %+v: error = %v; want ErrForbidden", name, actor, err)
			}
		}
	}
}

func TestCategoriesSeedOnFirstUse(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	all, err := svc.Categories(ctx, her, "")
	if err != nil || len(all) != 14 {
		t.Fatalf("got %d categories, %v; want 14", len(all), err)
	}
	gastos, err := svc.Categories(ctx, her, "Gasto")
	if err != nil || len(gastos) != 8 {
		t.Fatalf("got %d Gasto categories, %v; want 8", len(gastos), err)
	}
	if _, err := svc.Categories(ctx, her, "nope"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad kind error = %v; want ErrInvalidInput", err)
	}
	if got, _ := svc.Categories(ctx, other, "Ingreso"); len(got) != 3 {
		t.Errorf("another user must get her own seed, got %d", len(got))
	}
}

func TestCreateCategorySeedsFirstAndRejectsDuplicates(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	c, err := svc.CreateCategory(ctx, her, "  Café ")
	if err != nil || c.Name != "Café" || c.Kind != domain.KindExpense {
		t.Fatalf("got %+v, %v", c, err)
	}
	gastos, _ := svc.Categories(ctx, her, "Gasto")
	if len(gastos) != 9 {
		t.Errorf("a first custom category must not suppress the defaults: got %d Gasto categories, want 9", len(gastos))
	}
	if _, err := svc.CreateCategory(ctx, her, "Café"); !errors.Is(err, domain.ErrDuplicateCategory) {
		t.Errorf("duplicate error = %v; want ErrDuplicateCategory", err)
	}
	if _, err := svc.CreateCategory(ctx, her, " "); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("empty name error = %v; want ErrInvalidInput", err)
	}
}

func TestRenameCategory(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	cats, _ := svc.Categories(ctx, her, "")
	byName := map[string]domain.Category{}
	for _, c := range cats {
		byName[c.Name] = c
	}
	got, err := svc.RenameCategory(ctx, her, byName["Ropa"].ID, "Vestuario")
	if err != nil || got.Name != "Vestuario" {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, fixed := range []string{"Trabajo", "Inversión"} {
		if _, err := svc.RenameCategory(ctx, her, byName[fixed].ID, "X"); !errors.Is(err, domain.ErrCategoryFixed) {
			t.Errorf("renaming %s error = %v; want ErrCategoryFixed", fixed, err)
		}
	}
	if _, err := svc.RenameCategory(ctx, other, byName["Ropa"].ID, "X"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("another user's category error = %v; want ErrNotFound", err)
	}
	if _, err := svc.RenameCategory(ctx, her, 9999, "X"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown category error = %v; want ErrNotFound", err)
	}
	if _, err := svc.RenameCategory(ctx, her, byName["Ropa"].ID, ""); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("empty name error = %v; want ErrInvalidInput", err)
	}
}

// catID returns the id of a seeded category of her.
func catID(t *testing.T, svc *app.Service, who session.Identity, kind, name string) int {
	t.Helper()
	cats, err := svc.Categories(ctx, who, kind)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("no category %s/%s", kind, name)
	return 0
}

func TestMovementLifecycleAndScoping(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	ropa := catID(t, svc, her, "Gasto", "Ropa")
	m, err := svc.CreateMovement(ctx, her, domain.MovementInput{Kind: "Gasto", CategoryID: ropa, Amount: d("199.90"), Description: " Blusa ", Date: "2026-10-04"})
	if err != nil || m.Description != "Blusa" {
		t.Fatalf("create: %+v, %v", m, err)
	}
	up, err := svc.UpdateMovement(ctx, her, m.ID, domain.MovementInput{Kind: "Gasto", CategoryID: ropa, Amount: d("150"), Description: "Blusa", Date: "2026-10-05"})
	if err != nil || !up.Amount.Equal(d("150")) {
		t.Fatalf("update: %+v, %v", up, err)
	}
	if _, err := svc.UpdateMovement(ctx, other, m.ID, domain.MovementInput{Kind: "Gasto", CategoryID: catID(t, svc, other, "Gasto", "Ropa"), Amount: d("1"), Description: "x", Date: "2026-10-05"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("another user's update error = %v; want ErrNotFound", err)
	}
	if err := svc.DeleteMovement(ctx, other, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("another user's delete error = %v; want ErrNotFound", err)
	}
	if got, _ := svc.Movements(ctx, other, app.ListInput{Month: "2026-10"}); len(got) != 0 {
		t.Errorf("another user sees %d movements, want 0", len(got))
	}
	if err := svc.DeleteMovement(ctx, her, m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.DeleteMovement(ctx, her, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete error = %v; want ErrNotFound", err)
	}
}

func TestMovementCategoryRules(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	ropa := catID(t, svc, her, "Gasto", "Ropa")
	trabajo := catID(t, svc, her, "Ingreso", "Trabajo")
	foreign := catID(t, svc, other, "Gasto", "Ropa")

	cases := map[string]domain.MovementInput{
		"kind does not match the category": {Kind: "Ingreso", CategoryID: ropa, Amount: d("1"), Description: "x", Date: "2026-10-04"},
		"expense on an income category":    {Kind: "Gasto", CategoryID: trabajo, Amount: d("1"), Description: "x", Date: "2026-10-04"},
		"another user's category":          {Kind: "Gasto", CategoryID: foreign, Amount: d("1"), Description: "x", Date: "2026-10-04"},
		"unknown category":                 {Kind: "Gasto", CategoryID: 9999, Amount: d("1"), Description: "x", Date: "2026-10-04"},
		"zero expense":                     {Kind: "Gasto", CategoryID: ropa, Amount: d("0"), Description: "x", Date: "2026-10-04"},
	}
	for name, in := range cases {
		if _, err := svc.CreateMovement(ctx, her, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: error = %v; want ErrInvalidInput", name, err)
		}
	}
	if len(r.moves) != 0 {
		t.Errorf("%d movements stored by rejected calls", len(r.moves))
	}
}

func TestSavingsAllowWithdrawals(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	ahorro := catID(t, svc, her, "Ahorro", "Ahorro")
	for _, amount := range []string{"500", "-200"} {
		if _, err := svc.CreateMovement(ctx, her, domain.MovementInput{Kind: "Ahorro", CategoryID: ahorro, Amount: d(amount), Description: "x", Date: "2026-10-04"}); err != nil {
			t.Errorf("savings amount %s rejected: %v", amount, err)
		}
	}
	if _, err := svc.CreateMovement(ctx, her, domain.MovementInput{Kind: "Ahorro", CategoryID: ahorro, Amount: d("0"), Description: "x", Date: "2026-10-04"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("zero savings error = %v; want ErrInvalidInput", err)
	}
}

func TestEmptyDateDefaultsToToday(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	trabajo := catID(t, svc, her, "Ingreso", "Trabajo")
	m, err := svc.CreateMovement(ctx, her, domain.MovementInput{Kind: "Ingreso", CategoryID: trabajo, Amount: d("1"), Description: "x"})
	if err != nil || m.Date != "2026-10-10" {
		t.Errorf("got %+v, %v; want today's date", m, err)
	}
}

func TestMovementsUseTheOwnersCycle(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, day15)
	ropa := catID(t, svc, her, "Gasto", "Ropa")
	for _, date := range []string{"2026-09-14", "2026-09-15", "2026-10-14", "2026-10-15"} {
		if _, err := svc.CreateMovement(ctx, her, domain.MovementInput{Kind: "Gasto", CategoryID: ropa, Amount: d("1"), Description: date, Date: date}); err != nil {
			t.Fatal(err)
		}
	}
	// With cut day 15, cycle 2026-10 runs 2026-09-15..2026-10-14.
	got, err := svc.Movements(ctx, her, app.ListInput{Month: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Date != "2026-10-14" || got[1].Date != "2026-09-15" {
		t.Errorf("got %+v; want 2026-10-14 then 2026-09-15", got)
	}
	// Empty month means the current cycle: 2026-10-10 belongs to 2026-10.
	cur, _ := svc.Movements(ctx, her, app.ListInput{})
	if len(cur) != 2 || r.lastF.From != "2026-09-15" || r.lastF.To != "2026-10-14" {
		t.Errorf("current cycle filter = %+v, %d rows", r.lastF, len(cur))
	}
	// Filters reach the repository.
	if _, err := svc.Movements(ctx, her, app.ListInput{Month: "2026-10", Kind: "Gasto", CategoryID: ropa}); err != nil || r.lastF.Kind != domain.KindExpense || r.lastF.CategoryID != ropa {
		t.Errorf("filter = %+v, %v", r.lastF, err)
	}
}

func TestMovementsRejectBadInputAndSurfaceSettingsErrors(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	if _, err := svc.Movements(ctx, her, app.ListInput{Month: "2026-13"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad month error = %v; want ErrInvalidInput", err)
	}
	if _, err := svc.Movements(ctx, her, app.ListInput{Kind: "x"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad kind error = %v; want ErrInvalidInput", err)
	}
	failing := newSvc(newRepo(), fakeSettings{err: errSet})
	if _, err := failing.Movements(ctx, her, app.ListInput{}); !errors.Is(err, errSet) {
		t.Errorf("settings error = %v; want it surfaced", err)
	}
}

func addMove(r *fakeRepo, user string, kind domain.Kind, cat domain.Category, amount, date string) {
	r.nextMov++
	m := domain.Movement{ID: r.nextMov, Kind: kind, CategoryID: cat.ID, Amount: d(amount), Date: date}
	r.moves = append(r.moves, m)
	r.mOwners[m.ID] = user
}

func TestDashboardTotalsCategoriesAndScoping(t *testing.T) {
	r := newRepo()
	ropa := r.addCat("u-her", domain.KindExpense, "Ropa")
	salud := r.addCat("u-her", domain.KindExpense, "Salud")
	vacia := r.addCat("u-her", domain.KindExpense, "Pintura")
	trabajo := r.addCat("u-her", domain.KindIncome, "Trabajo")
	ahorro := r.addCat("u-her", domain.KindSavings, "Ahorro")
	_ = vacia
	addMove(r, "u-her", domain.KindIncome, trabajo, "10000", "2026-10-02")
	addMove(r, "u-her", domain.KindExpense, ropa, "300.50", "2026-10-03")
	addMove(r, "u-her", domain.KindExpense, salud, "900", "2026-10-04")
	addMove(r, "u-her", domain.KindExpense, ropa, "100", "2026-10-05")
	addMove(r, "u-her", domain.KindSavings, ahorro, "2000", "2026-10-06")
	addMove(r, "u-her", domain.KindSavings, ahorro, "-500", "2026-10-07") // withdrawal
	addMove(r, "u-her", domain.KindExpense, ropa, "9999", "2026-09-30")   // before the cycle
	addMove(r, "u-her", domain.KindExpense, ropa, "9999", "2026-11-01")   // after the cycle
	addMove(r, "u-other", domain.KindIncome, trabajo, "777", "2026-10-02")
	svc := newSvc(r, noCfg)

	got, err := svc.Dashboard(ctx, her, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if got.Period != (domain.Period{Month: "2026-10", Start: "2026-10-01", End: "2026-10-31"}) {
		t.Errorf("period = %+v", got.Period)
	}
	if !got.Totals.Income.Equal(d("10000")) || !got.Totals.Expenses.Equal(d("1300.50")) || !got.Totals.Savings.Equal(d("1500")) {
		t.Errorf("totals = %+v", got.Totals)
	}
	if !got.Totals.Available().Equal(d("7199.50")) {
		t.Errorf("available = %s, want 7199.50", got.Totals.Available())
	}
	if len(got.Categories) != 2 || got.Categories[0].Category != "Salud" || got.Categories[1].Category != "Ropa" ||
		!got.Categories[1].Spent.Equal(d("400.50")) {
		t.Errorf("categories = %+v; want Salud then Ropa, none without spending", got.Categories)
	}
}

func TestDashboardUsesTheCurrentCycleAndTheOwnersCutDay(t *testing.T) {
	r := newRepo()
	trabajo := r.addCat("u-her", domain.KindIncome, "Trabajo")
	addMove(r, "u-her", domain.KindIncome, trabajo, "100", "2026-10-14") // cycle 2026-10 with cut day 15
	addMove(r, "u-her", domain.KindIncome, trabajo, "50", "2026-10-15")  // cycle 2026-11
	svc := newSvc(r, day15)
	got, err := svc.Dashboard(ctx, her, "")
	if err != nil {
		t.Fatal(err)
	}
	// today is 2026-10-10: still the cycle labelled 2026-10 (09-15 .. 10-14).
	if got.Period != (domain.Period{Month: "2026-10", Start: "2026-09-15", End: "2026-10-14"}) {
		t.Errorf("period = %+v", got.Period)
	}
	if !got.Totals.Income.Equal(d("100")) {
		t.Errorf("income = %s, want 100", got.Totals.Income)
	}
	next, _ := svc.Dashboard(ctx, her, "2026-11")
	if !next.Totals.Income.Equal(d("50")) {
		t.Errorf("next cycle income = %s, want 50", next.Totals.Income)
	}
}

func TestDashboardEmptyCycleAndErrors(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	got, err := svc.Dashboard(ctx, her, "2026-10")
	if err != nil || !got.Totals.Available().IsZero() || len(got.Categories) != 0 {
		t.Errorf("empty cycle = %+v, %v", got, err)
	}
	if _, err := svc.Dashboard(ctx, her, "2026-13"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad month error = %v; want ErrInvalidInput", err)
	}
	if _, err := newSvc(newRepo(), fakeSettings{err: errSet}).Dashboard(ctx, her, ""); !errors.Is(err, errSet) {
		t.Errorf("settings error = %v; want it propagated", err)
	}
	r := newRepo()
	r.err = errors.New("db down")
	if _, err := newSvc(r, noCfg).Dashboard(ctx, her, "2026-10"); err == nil {
		t.Error("repository error must propagate")
	}
}

func TestPeriod(t *testing.T) {
	svc := newSvc(newRepo(), day15)
	p, err := svc.Period(ctx, her, "2026-10")
	if err != nil || p != (domain.Period{Month: "2026-10", Start: "2026-09-15", End: "2026-10-14"}) {
		t.Errorf("got %+v, %v", p, err)
	}
	if cur, _ := svc.Period(ctx, her, ""); cur.Month != "2026-10" {
		t.Errorf("empty month = %+v; want the current cycle", cur)
	}
	if _, err := svc.Period(ctx, her, "oct"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad month error = %v; want ErrInvalidInput", err)
	}
}

func TestKindTotalsAvailable(t *testing.T) {
	got := domain.KindTotals{Income: d("100"), Expenses: d("30"), Savings: d("-20")}.Available()
	if !got.Equal(d("90")) {
		t.Errorf("available = %s, want 90 (a withdrawal adds money back)", got)
	}
}
