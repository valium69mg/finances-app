package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

func validWish() domain.WishInput {
	return domain.WishInput{Name: "Bolsa", Price: d("499.99"), CategoryID: 3}
}

func TestValidateWish(t *testing.T) {
	v, err := domain.ValidateWish(domain.WishInput{
		Name: "  Bolsa  ", Price: d("10"), CategoryID: 1, StoreURL: " https://tienda.example/x?y=1 ", Priority: "alta", Note: " nota ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "Bolsa" || v.StoreURL != "https://tienda.example/x?y=1" || v.Priority != domain.PriorityHigh || v.Note != "nota" {
		t.Errorf("valid = %+v; want trimmed values", v)
	}
	if v, _ := domain.ValidateWish(validWish()); v.Priority != domain.PriorityMedium || v.StoreURL != "" {
		t.Errorf("defaults = %+v; want priority media and no link", v)
	}

	bad := map[string]func(*domain.WishInput){
		"empty name":       func(i *domain.WishInput) { i.Name = "  " },
		"long name":        func(i *domain.WishInput) { i.Name = strings.Repeat("a", 121) },
		"zero price":       func(i *domain.WishInput) { i.Price = d("0") },
		"negative price":   func(i *domain.WishInput) { i.Price = d("-1") },
		"3 decimals":       func(i *domain.WishInput) { i.Price = d("1.005") },
		"huge price":       func(i *domain.WishInput) { i.Price = d("99999999999999999999") },
		"no category":      func(i *domain.WishInput) { i.CategoryID = 0 },
		"ftp url":          func(i *domain.WishInput) { i.StoreURL = "ftp://x.example/a" },
		"javascript url":   func(i *domain.WishInput) { i.StoreURL = "javascript:alert(1)" },
		"no host":          func(i *domain.WishInput) { i.StoreURL = "https://" },
		"relative url":     func(i *domain.WishInput) { i.StoreURL = "tienda.example/x" },
		"url with space":   func(i *domain.WishInput) { i.StoreURL = "https://a.example/a b" },
		"long url":         func(i *domain.WishInput) { i.StoreURL = "https://a.example/" + strings.Repeat("a", 500) },
		"unknown priority": func(i *domain.WishInput) { i.Priority = "urgente" },
		"long note":        func(i *domain.WishInput) { i.Note = strings.Repeat("a", 501) },
	}
	for name, mutate := range bad {
		in := validWish()
		mutate(&in)
		if _, err := domain.ValidateWish(in); !errors.Is(err, domain.ErrWishInvalid) {
			t.Errorf("%s: error = %v; want ErrWishInvalid", name, err)
		}
	}
}

func TestWishStatusIsDerivedFromTheLink(t *testing.T) {
	cases := []struct {
		name string
		link *domain.Conversion
		want domain.WishStatus
	}{
		{"no link", nil, domain.WishActive},
		{"future expense", &domain.Conversion{Type: domain.ConvertedFuture, TargetID: 1}, domain.WishConverted},
		{"expense", &domain.Conversion{Type: domain.ConvertedExpense, TargetID: 1}, domain.WishConverted},
		{"request solicitada", &domain.Conversion{Type: domain.ConvertedRequest, TargetID: 1, RequestState: "solicitada"}, domain.WishConverted},
		{"request aprobada", &domain.Conversion{Type: domain.ConvertedRequest, TargetID: 1, RequestState: "aprobada"}, domain.WishConverted},
		{"request rechazada", &domain.Conversion{Type: domain.ConvertedRequest, TargetID: 1, RequestState: "rechazada"}, domain.WishActive},
		{"request cancelada", &domain.Conversion{Type: domain.ConvertedRequest, TargetID: 1, RequestState: "cancelada"}, domain.WishActive},
	}
	for _, c := range cases {
		if got := (domain.WishItem{Link: c.link}).Status(); got != c.want {
			t.Errorf("%s: status = %s; want %s", c.name, got, c.want)
		}
	}
}

func TestSortWishlist(t *testing.T) {
	at := func(n int) time.Time { return time.Date(2026, 10, 1, 0, 0, n, 0, time.UTC) }
	conv := &domain.Conversion{Type: domain.ConvertedExpense, TargetID: 1}
	items := []domain.WishItem{
		{ID: 1, Priority: domain.PriorityLow, CreatedAt: at(1)},
		{ID: 2, Priority: domain.PriorityHigh, CreatedAt: at(2), Link: conv},
		{ID: 3, Priority: domain.PriorityHigh, CreatedAt: at(3)},
		{ID: 4, Priority: domain.PriorityMedium, CreatedAt: at(4)},
		{ID: 5, Priority: domain.PriorityHigh, CreatedAt: at(5)},
		{ID: 6, Priority: domain.PriorityLow, CreatedAt: at(6), Link: conv},
	}
	domain.SortWishlist(items)
	var got []int
	for _, it := range items {
		got = append(got, it.ID)
	}
	want := []int{5, 3, 4, 1, 6, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v; want %v", got, want)
		}
	}
}
