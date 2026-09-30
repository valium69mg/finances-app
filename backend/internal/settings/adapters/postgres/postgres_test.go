package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/valium69mg/finances-app/backend/internal/settings/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// settings migration, so the tests never touch real data. It skips the test
// when TEST_DATABASE_URL is not set.
func newRepo(t *testing.T) *postgres.Repo {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "settings_test_" + hex.EncodeToString(suffix)
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

	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", "000005_settings.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return postgres.NewRepo(pool)
}

func fullConfig() domain.Config {
	cfg := settingstest.RealConfig()
	pause := settingstest.RealPause()
	pause.Note = "paused to fund Gastos futuros"
	cfg.Pause = &pause
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	cfg.Issuer = domain.Issuer{RFC: "AAAA010101AAA", Name: "TEST ISSUER", Regimen: "626", PostalCode: "76246", Note: "n"}
	cfg.Clients[0].RFC = "XEXX010101000"
	cfg.Clients[0].Concepto = "Servicios de desarrollo de software"
	cfg.Clients[1].RetISRRate = settingstest.D("0.0125")
	return cfg
}

func TestEmptyLoad(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	empty, err := repo.IsEmpty(ctx)
	if err != nil || !empty {
		t.Fatalf("IsEmpty = %v, %v; want true", empty, err)
	}
	cfg, err := repo.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SalaryUSD != nil || len(cfg.Categories) != 0 || cfg.Pause != nil {
		t.Fatalf("expected an empty config, got %+v", cfg)
	}
}

func TestImportRoundTrip(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	want := fullConfig()

	if err := repo.Import(ctx, want); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if empty, _ := repo.IsEmpty(ctx); empty {
		t.Fatal("IsEmpty = true after import")
	}
	got, err := repo.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Categories) != 22 || len(got.Clients) != 2 || len(got.Instruments) != 5 ||
		len(got.Brackets) != 5 || len(got.PaymentMethods) != 4 {
		t.Fatalf("row counts: %d categories, %d clients, %d instruments, %d brackets, %d payment methods",
			len(got.Categories), len(got.Clients), len(got.Instruments), len(got.Brackets), len(got.PaymentMethods))
	}
	// Decimals compare by value, so normalize both sides through the domain views.
	if !got.General().SalaryUSD.Equal(want.General().SalaryUSD) || !got.MorseFeeRate.Equal(*want.MorseFeeRate) {
		t.Fatalf("general mismatch: %+v", got.General())
	}
	for i, c := range got.Categories {
		w := want.Categories[i]
		if c.Name != w.Name || c.Kind != w.Kind || c.Includes != w.Includes || !reflect.DeepEqual(nilIfEmpty(c.Keywords), nilIfEmpty(w.Keywords)) {
			t.Fatalf("category %d = %+v, want %+v", i, c, w)
		}
		if (c.Budget == nil) != (w.Budget == nil) || (c.Budget != nil && !c.Budget.Equal(*w.Budget)) {
			t.Fatalf("category %q budget = %v, want %v", c.Name, c.Budget, w.Budget)
		}
	}
	if !got.Brackets[2].Upper.Equal(settingstest.D("83333.33")) {
		t.Fatalf("bracket 3 upper = %s", got.Brackets[2].Upper)
	}
	if got.Clients[1].RetISRRate.String() != "0.0125" || got.Clients[0].RFC != "XEXX010101000" {
		t.Fatalf("client invoicing fields lost: %+v", got.Clients)
	}
	if !reflect.DeepEqual(got.InstrumentByCategory, want.InstrumentByCategory) {
		t.Fatalf("instrument_by_category = %v", got.InstrumentByCategory)
	}
	if got.ExtraIncomeSplit["sat_reserve_rate"].String() != "0.165" {
		t.Fatalf("split = %v", got.ExtraIncomeSplit)
	}
	if len(got.InvestmentAllocation) != 1 || got.InvestmentAllocation[0].Key != "voo" {
		t.Fatalf("allocation = %v", got.InvestmentAllocation)
	}
	if got.Issuer != want.Issuer {
		t.Fatalf("issuer = %+v", got.Issuer)
	}
	if got.Pause == nil || !reflect.DeepEqual(got.Pause.Months, want.Pause.Months) ||
		got.Pause.ResumeMonth != "2027-02" || got.Pause.Note != want.Pause.Note ||
		!got.Pause.FutureExpensesPlan["2027-01"].Equal(settingstest.D("10000")) {
		t.Fatalf("pause = %+v", got.Pause)
	}

	// The loaded configuration must reproduce the hand-computed pause budgets.
	overrides, err := got.MonthBudgetOverrides("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if !overrides["Inversiones"].IsZero() || !overrides["Gastos futuros"].Equal(settingstest.D("12000")) {
		t.Fatalf("2026-10 overrides = %v", overrides)
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func TestSectionReplace(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	if err := repo.Import(ctx, fullConfig()); err != nil {
		t.Fatal(err)
	}

	newInstruments := []domain.Instrument{{ID: "sp500", Name: "S&P 500", Type: "renta_variable", Platform: "GBM"}}
	if err := repo.SaveInstruments(ctx, newInstruments, map[string]string{"Inversiones": "sp500"}); err != nil {
		t.Fatalf("SaveInstruments: %v", err)
	}
	budget := settingstest.D("999.5")
	cats := []domain.Category{{Name: "Solo", Kind: "Gasto", Budget: &budget, Keywords: []string{"a", "b"}}}
	if err := repo.SaveCategories(ctx, cats); err != nil {
		t.Fatalf("SaveCategories: %v", err)
	}
	if err := repo.SavePause(ctx, nil); err != nil {
		t.Fatalf("SavePause(nil): %v", err)
	}
	if err := repo.SaveBrackets(ctx, []domain.Bracket{{Upper: settingstest.D("100"), Rate: settingstest.D("0.05")}}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Instruments) != 1 || got.InstrumentByCategory["Inversiones"] != "sp500" || len(got.InstrumentByCategory) != 1 {
		t.Fatalf("instruments = %+v / %v", got.Instruments, got.InstrumentByCategory)
	}
	if len(got.Categories) != 1 || got.Categories[0].Budget.String() != "999.5" || len(got.Categories[0].Keywords) != 2 {
		t.Fatalf("categories = %+v", got.Categories)
	}
	if got.Pause != nil {
		t.Fatalf("pause should be removed, got %+v", got.Pause)
	}
	if len(got.Brackets) != 1 || len(got.Clients) != 2 {
		t.Fatalf("brackets = %d, clients = %d", len(got.Brackets), len(got.Clients))
	}
}

func TestSaveGeneralUpserts(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	g := fullConfig().General()
	if err := repo.SaveGeneral(ctx, g); err != nil {
		t.Fatal(err)
	}
	g.SalaryUSD = settingstest.D("4000.25")
	g.BudgetIncludesExtraIncome = true
	if err := repo.SaveGeneral(ctx, g); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.SalaryUSD.String() != "4000.25" || !got.BudgetIncludesExtraIncome {
		t.Fatalf("general = %+v", got.General())
	}
}

func TestImportIsAtomic(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	bad := fullConfig()
	bad.InstrumentByCategory = map[string]string{"Inversiones": "does-not-exist"} // violates the FK

	if err := repo.Import(ctx, bad); err == nil {
		t.Fatal("Import with a dangling instrument reference should fail")
	}
	if empty, err := repo.IsEmpty(ctx); err != nil || !empty {
		t.Fatalf("failed import left data behind: empty=%v err=%v", empty, err)
	}
}
