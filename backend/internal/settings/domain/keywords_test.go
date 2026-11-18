package domain_test

import (
	"errors"
	"reflect"
	"testing"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func gasto(name string, kws ...string) domain.Category {
	return domain.Category{Name: name, Kind: ledger.KindExpense, Keywords: kws}
}

func TestCleanKeywords(t *testing.T) {
	got := domain.CleanKeywords([]string{"  Heb ", "", "   ", "heb", "CAFÉ", "cafe", "Café", "uber"})
	want := []string{"Heb", "CAFÉ", "uber"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CleanKeywords = %q, want %q (trimmed, no empties, first spelling kept)", got, want)
	}
	if got := domain.CleanKeywords(nil); got == nil || len(got) != 0 {
		t.Errorf("CleanKeywords(nil) = %#v, want an empty non-nil slice", got)
	}
}

func TestCleanCategoriesDoesNotMutateTheInput(t *testing.T) {
	in := []domain.Category{gasto("A", " x ", "X")}
	out := domain.CleanCategories(in)
	if !reflect.DeepEqual(out[0].Keywords, []string{"x"}) || !reflect.DeepEqual(in[0].Keywords, []string{" x ", "X"}) {
		t.Errorf("out = %q in = %q", out[0].Keywords, in[0].Keywords)
	}
}

func TestCategoryNamesAreUniqueIgnoringCaseAndAccents(t *testing.T) {
	for name, cats := range map[string][]domain.Category{
		"case":   {gasto("Súper"), gasto("SÚPER")},
		"accent": {gasto("Super"), gasto("Súper")},
	} {
		if err := domain.ValidateCategories(cats); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if err := domain.ValidateCategories([]domain.Category{gasto("Super"), gasto("Salud")}); err != nil {
		t.Errorf("distinct names: %v", err)
	}
}

func TestCheckKeywordOwnership(t *testing.T) {
	stored := []domain.Category{gasto("Súper", "heb"), gasto("Salud", "farmacia")}

	t.Run("a gained keyword owned by another category is rejected", func(t *testing.T) {
		updated := []domain.Category{gasto("Súper", "heb"), gasto("Salud", "farmacia", "HEB")}
		err := domain.CheckKeywordOwnership(stored, updated)
		var inUse *domain.KeywordInUseError
		if !errors.As(err, &inUse) || inUse.Keyword != "HEB" || inUse.Category != "Súper" {
			t.Fatalf("err = %v, want keyword HEB owned by Súper", err)
		}
	})
	t.Run("accents and case do not escape the check", func(t *testing.T) {
		updated := []domain.Category{gasto("Súper", "café"), gasto("Salud", "farmacia"), gasto("Ocio", "CAFE")}
		if err := domain.CheckKeywordOwnership(stored, updated); err == nil {
			t.Error("café and CAFE in two categories were accepted")
		}
	})
	t.Run("a new category is checked on all its keywords", func(t *testing.T) {
		updated := append(append([]domain.Category{}, stored...), gasto("Mascotas", "croquetas", "Farmacia"))
		var inUse *domain.KeywordInUseError
		if err := domain.CheckKeywordOwnership(stored, updated); !errors.As(err, &inUse) || inUse.Category != "Salud" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("kinds are independent", func(t *testing.T) {
		updated := append(append([]domain.Category{}, stored...),
			domain.Category{Name: "Ventas", Kind: ledger.KindIncome, Keywords: []string{"heb"}})
		if err := domain.CheckKeywordOwnership(stored, updated); err != nil {
			t.Errorf("an income keyword clashed with a Gasto one: %v", err)
		}
	})
	t.Run("a legacy conflict never blocks an unrelated category", func(t *testing.T) {
		legacy := []domain.Category{gasto("A", "dup"), gasto("B", "dup"), gasto("C", "x")}
		updated := []domain.Category{gasto("A", "dup"), gasto("B", "dup"), gasto("C", "x", "y")}
		if err := domain.CheckKeywordOwnership(legacy, updated); err != nil {
			t.Errorf("legacy conflict blocked saving C: %v", err)
		}
	})
	t.Run("the same keyword stays with its own category", func(t *testing.T) {
		if err := domain.CheckKeywordOwnership(stored, stored); err != nil {
			t.Error(err)
		}
	})
}

func TestFindKeywordConflicts(t *testing.T) {
	cats := []domain.Category{gasto("A", "Dup", "solo"), gasto("B", "dup"), gasto("C", "DÚP"),
		{Name: "I", Kind: ledger.KindIncome, Keywords: []string{"dup"}}}
	got := domain.FindKeywordConflicts(cats)
	if len(got) != 1 || got[0].Keyword != "dup" || !reflect.DeepEqual(got[0].Categories, []string{"A", "B", "C"}) {
		t.Errorf("conflicts = %+v", got)
	}
}
