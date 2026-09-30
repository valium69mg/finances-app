package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/settings/app"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

type fakeRepo struct {
	cfg      domain.Config
	empty    bool
	imported *domain.Config
	saved    []string
	err      error
}

func (f *fakeRepo) Load(context.Context) (domain.Config, error) { return f.cfg, f.err }
func (f *fakeRepo) IsEmpty(context.Context) (bool, error)       { return f.empty, f.err }
func (f *fakeRepo) note(s string) error                         { f.saved = append(f.saved, s); return f.err }
func (f *fakeRepo) SaveGeneral(context.Context, domain.General) error {
	return f.note("general")
}
func (f *fakeRepo) SaveCategories(context.Context, []domain.Category) error {
	return f.note("categories")
}
func (f *fakeRepo) SaveClients(context.Context, []domain.Client) error { return f.note("clients") }
func (f *fakeRepo) SaveInstruments(context.Context, []domain.Instrument, map[string]string) error {
	return f.note("instruments")
}
func (f *fakeRepo) SaveBrackets(context.Context, []domain.Bracket) error { return f.note("brackets") }
func (f *fakeRepo) SavePaymentMethods(context.Context, []string) error   { return f.note("payment") }
func (f *fakeRepo) SaveIssuer(context.Context, domain.Issuer) error      { return f.note("issuer") }
func (f *fakeRepo) SavePause(context.Context, *domain.PausePlan) error   { return f.note("pause") }
func (f *fakeRepo) Import(_ context.Context, cfg domain.Config) error {
	f.imported = &cfg
	return f.err
}

func TestUpdateValidatesBeforeSaving(t *testing.T) {
	repo := &fakeRepo{}
	svc := app.NewService(repo)
	err := svc.UpdateCategories(context.Background(), []domain.Category{{Name: "", Kind: "Gasto"}})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if len(repo.saved) != 0 {
		t.Fatalf("repo was called: %v", repo.saved)
	}
}

func TestUpdateValidPersists(t *testing.T) {
	cfg := settingstest.RealConfig()
	repo := &fakeRepo{cfg: cfg}
	svc := app.NewService(repo)
	ctx := context.Background()
	steps := []error{
		svc.UpdateGeneral(ctx, cfg.General()),
		svc.UpdateCategories(ctx, cfg.Categories),
		svc.UpdateClients(ctx, cfg.Clients),
		svc.UpdateInstruments(ctx, cfg.Instruments, cfg.InstrumentByCategory),
		svc.UpdateBrackets(ctx, cfg.Brackets),
		svc.UpdatePaymentMethods(ctx, []string{"Efectivo"}),
		svc.UpdateIssuer(ctx, domain.Issuer{RFC: "X", Name: "Y"}),
		svc.UpdatePause(ctx, nil),
	}
	for i, err := range steps {
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if len(repo.saved) != len(steps) {
		t.Fatalf("saved = %v, want %d sections", repo.saved, len(steps))
	}
}

func TestUpdateGeneralRejectsUnknownAllocationInstrument(t *testing.T) {
	cfg := settingstest.RealConfig()
	repo := &fakeRepo{cfg: cfg}
	g := cfg.General()
	g.InvestmentAllocation = []domain.Weight{{Key: "nope", Value: settingstest.D("1")}}
	if err := app.NewService(repo).UpdateGeneral(context.Background(), g); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestMonthBudgetsAppliesPause(t *testing.T) {
	cfg := settingstest.RealConfig()
	pause := settingstest.RealPause()
	cfg.Pause = &pause
	svc := app.NewService(&fakeRepo{cfg: cfg})

	got, err := svc.MonthBudgets(context.Background(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	budgets := map[string]string{}
	for _, b := range got {
		if b.Budget != nil {
			budgets[b.Name] = b.Budget.String()
		}
	}
	if budgets["Inversiones"] != "0" || budgets["Gastos futuros"] != "12000" {
		t.Fatalf("budgets = %v", budgets)
	}
	if _, ok := budgets["Impuestos"]; !ok {
		t.Fatal("Impuestos budget should be computed")
	}

	got, err = svc.MonthBudgets(context.Background(), "2027-02")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got {
		if b.Name == "Inversiones" && b.Budget.String() != "5000" {
			t.Fatalf("Inversiones in 2027-02 = %s, want 5000", b.Budget)
		}
	}
}

func TestMonthBudgetsErrors(t *testing.T) {
	svc := app.NewService(&fakeRepo{})
	if _, err := svc.MonthBudgets(context.Background(), "2026-1"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("bad month err = %v, want ErrInvalid", err)
	}
	if _, err := svc.MonthBudgets(context.Background(), "2026-10"); !errors.Is(err, domain.ErrMissingConfig) {
		t.Fatalf("empty config err = %v, want ErrMissingConfig", err)
	}
}

func TestImportRefusesNonEmptyWithoutForce(t *testing.T) {
	cfg := settingstest.RealConfig()
	repo := &fakeRepo{empty: false}
	svc := app.NewService(repo)

	if err := svc.Import(context.Background(), cfg, false); !errors.Is(err, app.ErrAlreadyImported) {
		t.Fatalf("err = %v, want ErrAlreadyImported", err)
	}
	if repo.imported != nil {
		t.Fatal("import ran on a non-empty repo")
	}
	if err := svc.Import(context.Background(), cfg, true); err != nil {
		t.Fatalf("forced import: %v", err)
	}
	if repo.imported == nil {
		t.Fatal("forced import did not run")
	}
}

func TestImportEmptyAndInvalid(t *testing.T) {
	repo := &fakeRepo{empty: true}
	svc := app.NewService(repo)
	if err := svc.Import(context.Background(), settingstest.RealConfig(), false); err != nil {
		t.Fatalf("import into empty repo: %v", err)
	}
	if err := svc.Import(context.Background(), domain.Config{}, false); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty config err = %v, want ErrInvalid", err)
	}
}
