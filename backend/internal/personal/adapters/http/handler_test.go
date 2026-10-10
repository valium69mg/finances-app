package personalhttp_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	personalhttp "github.com/valium69mg/finances-app/backend/internal/personal/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var created = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type fakeService struct {
	err        error
	categories []domain.Category
	category   domain.Category
	movements  []domain.Movement
	movement   domain.Movement

	gotActor session.Identity
	gotKind  string
	gotName  string
	gotID    int
	gotList  app.ListInput
	gotInput domain.MovementInput
	deleted  int
}

func (f *fakeService) Categories(_ context.Context, a session.Identity, kind string) ([]domain.Category, error) {
	f.gotActor, f.gotKind = a, kind
	return f.categories, f.err
}
func (f *fakeService) CreateCategory(_ context.Context, a session.Identity, name string) (domain.Category, error) {
	f.gotActor, f.gotName = a, name
	return f.category, f.err
}
func (f *fakeService) RenameCategory(_ context.Context, a session.Identity, id int, name string) (domain.Category, error) {
	f.gotActor, f.gotID, f.gotName = a, id, name
	return f.category, f.err
}
func (f *fakeService) Movements(_ context.Context, a session.Identity, in app.ListInput) ([]domain.Movement, error) {
	f.gotActor, f.gotList = a, in
	return f.movements, f.err
}
func (f *fakeService) CreateMovement(_ context.Context, a session.Identity, in domain.MovementInput) (domain.Movement, error) {
	f.gotActor, f.gotInput = a, in
	return f.movement, f.err
}
func (f *fakeService) UpdateMovement(_ context.Context, a session.Identity, id int, in domain.MovementInput) (domain.Movement, error) {
	f.gotActor, f.gotID, f.gotInput = a, id, in
	return f.movement, f.err
}
func (f *fakeService) DeleteMovement(_ context.Context, a session.Identity, id int) error {
	f.gotActor, f.deleted = a, id
	return f.err
}

var her = session.Identity{UserID: "u-her", Role: session.RoleHousehold}

// requireGoodToken stands in for the auth middleware: it sets the identity.
func requireGoodToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(session.With(r.Context(), her)))
	})
}

func server(svc *fakeService) http.Handler {
	mux := http.NewServeMux()
	personalhttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func ropaMovement() domain.Movement {
	return domain.Movement{
		ID: 7, Kind: domain.KindExpense, CategoryID: 5, CategoryName: "Ropa", Amount: d("199.90"),
		Description: "Blusa", Date: "2026-10-04", CreatedAt: created, UpdatedAt: created,
	}
}

const ropaJSON = `{"id":7,"kind":"Gasto","category_id":5,"category":"Ropa","amount":"199.9","description":"Blusa","date":"2026-10-04","created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z"}`

func TestRoutesRequireAuth(t *testing.T) {
	h := server(&fakeService{})
	for _, r := range []struct{ method, path string }{
		{"GET", "/personal/categories"}, {"POST", "/personal/categories"}, {"PUT", "/personal/categories/1"},
		{"GET", "/personal/movements"}, {"POST", "/personal/movements"}, {"PUT", "/personal/movements/1"}, {"DELETE", "/personal/movements/1"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestCategories(t *testing.T) {
	svc := &fakeService{categories: []domain.Category{
		{ID: 1, Kind: domain.KindIncome, Name: "Trabajo"}, {ID: 4, Kind: domain.KindExpense, Name: "Ropa"},
	}}
	rec := do(server(svc), "GET", "/personal/categories?kind=Gasto", "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	want := `[{"id":1,"kind":"Ingreso","name":"Trabajo","fixed":true},{"id":4,"kind":"Gasto","name":"Ropa","fixed":false}]`
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("body = %s", rec.Body)
	}
	if svc.gotKind != "Gasto" || svc.gotActor != her {
		t.Errorf("service got kind %q actor %+v", svc.gotKind, svc.gotActor)
	}
}

func TestEmptyListsAreArrays(t *testing.T) {
	h := server(&fakeService{})
	for _, path := range []string{"/personal/categories", "/personal/movements"} {
		if body := strings.TrimSpace(do(h, "GET", path, "").Body.String()); body != "[]" {
			t.Errorf("GET %s empty body = %s, want []", path, body)
		}
	}
}

func TestCreateAndRenameCategory(t *testing.T) {
	svc := &fakeService{category: domain.Category{ID: 9, Kind: domain.KindExpense, Name: "Café"}}
	h := server(svc)
	rec := do(h, "POST", "/personal/categories", `{"name":"Café"}`)
	if rec.Code != 201 || svc.gotName != "Café" || !strings.Contains(rec.Body.String(), `"id":9`) {
		t.Errorf("create: %d %s (name %q)", rec.Code, rec.Body, svc.gotName)
	}
	rec = do(h, "PUT", "/personal/categories/9", `{"name":"Cafetería"}`)
	if rec.Code != 200 || svc.gotID != 9 || svc.gotName != "Cafetería" {
		t.Errorf("rename: %d %s (id %d name %q)", rec.Code, rec.Body, svc.gotID, svc.gotName)
	}
	if rec := do(h, "PUT", "/personal/categories/abc", `{"name":"x"}`); rec.Code != 404 {
		t.Errorf("malformed id: status %d, want 404", rec.Code)
	}
	if rec := do(h, "POST", "/personal/categories", `{"name":"x","kind":"Ingreso"}`); rec.Code != 400 {
		t.Errorf("unknown field: status %d, want 400", rec.Code)
	}
}

func TestMovementsQuery(t *testing.T) {
	svc := &fakeService{movements: []domain.Movement{ropaMovement()}}
	h := server(svc)
	rec := do(h, "GET", "/personal/movements?month=2026-10&kind=Gasto&category_id=5", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "["+ropaJSON+"]" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if svc.gotList != (app.ListInput{Month: "2026-10", Kind: "Gasto", CategoryID: 5}) {
		t.Errorf("service got %+v", svc.gotList)
	}
	do(h, "GET", "/personal/movements", "")
	if svc.gotList != (app.ListInput{}) {
		t.Errorf("no query: service got %+v", svc.gotList)
	}
	for _, bad := range []string{"abc", "0", "-3"} {
		if rec := do(h, "GET", "/personal/movements?category_id="+bad, ""); rec.Code != 400 {
			t.Errorf("category_id=%s: status %d, want 400", bad, rec.Code)
		}
	}
}

func TestCreateAndUpdateMovement(t *testing.T) {
	svc := &fakeService{movement: ropaMovement()}
	h := server(svc)
	body := `{"kind":"Gasto","category_id":5,"amount":"199.90","description":"Blusa","date":"2026-10-04"}`
	rec := do(h, "POST", "/personal/movements", body)
	if rec.Code != 201 || strings.TrimSpace(rec.Body.String()) != ropaJSON {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	want := domain.MovementInput{Kind: "Gasto", CategoryID: 5, Amount: d("199.90"), Description: "Blusa", Date: "2026-10-04"}
	if svc.gotInput.Kind != want.Kind || svc.gotInput.CategoryID != 5 || !svc.gotInput.Amount.Equal(want.Amount) || svc.gotInput.Description != "Blusa" || svc.gotInput.Date != want.Date {
		t.Errorf("service got %+v", svc.gotInput)
	}
	if svc.gotActor != her {
		t.Errorf("service got actor %+v", svc.gotActor)
	}
	rec = do(h, "PUT", "/personal/movements/7", body)
	if rec.Code != 200 || svc.gotID != 7 {
		t.Errorf("update: %d %s (id %d)", rec.Code, rec.Body, svc.gotID)
	}
	if rec := do(h, "POST", "/personal/movements", `{"kind":"Gasto","bogus":1}`); rec.Code != 400 {
		t.Errorf("unknown field: status %d, want 400", rec.Code)
	}
	if rec := do(h, "POST", "/personal/movements", `not json`); rec.Code != 400 {
		t.Errorf("malformed body: status %d, want 400", rec.Code)
	}
}

func TestDeleteMovement(t *testing.T) {
	svc := &fakeService{}
	rec := do(server(svc), "DELETE", "/personal/movements/7", "")
	if rec.Code != 204 || svc.deleted != 7 || rec.Body.Len() != 0 {
		t.Errorf("delete: %d %q (id %d)", rec.Code, rec.Body, svc.deleted)
	}
	if rec := do(server(svc), "DELETE", "/personal/movements/0", ""); rec.Code != 404 {
		t.Errorf("id 0: status %d, want 404", rec.Code)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: amount must be greater than zero", domain.ErrInvalidInput), 400, "invalid_personal"},
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrNotFound, 404, "not_found"},
		{domain.ErrCategoryFixed, 409, "category_fixed"},
		{domain.ErrDuplicateCategory, 409, "category_exists"},
		{settings.ErrMissingConfig, 422, "settings_incomplete"},
		{errors.New("database exploded: secret detail"), 500, "internal_error"},
	}
	for _, c := range cases {
		rec := do(server(&fakeService{err: c.err}), "POST", "/personal/movements", `{}`)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), `"error":"`+c.code+`"`) {
			t.Errorf("%v: got %d %s, want %d %s", c.err, rec.Code, rec.Body, c.status, c.code)
		}
		if c.status == 500 && strings.Contains(rec.Body.String(), "secret detail") {
			t.Error("an internal error leaked its cause")
		}
	}
}
