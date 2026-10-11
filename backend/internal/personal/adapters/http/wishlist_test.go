package personalhttp_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	requestsdomain "github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

type wishCalls struct {
	list    []domain.WishItem
	item    domain.WishItem
	input   domain.WishInput
	future  app.WishFutureInput
	expense app.WishExpenseInput
	request app.WishRequestInput
	id      int
	deleted int
}

func (f *fakeService) wish() *wishCalls { return &f.wl }

func (f *fakeService) Wishlist(_ context.Context, a session.Identity, userID string) ([]domain.WishItem, error) {
	f.gotActor, f.gotUserID = a, userID
	return f.wish().list, f.err
}
func (f *fakeService) CreateWish(_ context.Context, a session.Identity, in domain.WishInput) (domain.WishItem, error) {
	f.gotActor, f.wish().input = a, in
	return f.wish().item, f.err
}
func (f *fakeService) UpdateWish(_ context.Context, a session.Identity, id int, in domain.WishInput) (domain.WishItem, error) {
	f.gotActor, f.wish().id, f.wish().input = a, id, in
	return f.wish().item, f.err
}
func (f *fakeService) DeleteWish(_ context.Context, a session.Identity, id int) error {
	f.gotActor, f.wish().deleted = a, id
	return f.err
}
func (f *fakeService) ConvertWishToFuture(_ context.Context, a session.Identity, id int, in app.WishFutureInput) (domain.WishItem, error) {
	f.gotActor, f.wish().id, f.wish().future = a, id, in
	return f.wish().item, f.err
}
func (f *fakeService) ConvertWishToExpense(_ context.Context, a session.Identity, id int, in app.WishExpenseInput) (domain.WishItem, error) {
	f.gotActor, f.wish().id, f.wish().expense = a, id, in
	return f.wish().item, f.err
}
func (f *fakeService) ConvertWishToRequest(_ context.Context, a session.Identity, id int, in app.WishRequestInput) (domain.WishItem, error) {
	f.gotActor, f.wish().id, f.wish().request = a, id, in
	return f.wish().item, f.err
}

func bolsa() domain.WishItem {
	return domain.WishItem{
		ID: 5, Name: "Bolsa", Price: d("499.9"), CategoryID: 4, CategoryName: "Ropa", StoreURL: "https://tienda.example/b",
		Priority: domain.PriorityHigh, Note: "roja", CreatedAt: created, UpdatedAt: created,
	}
}

const bolsaJSON = `{"id":5,"name":"Bolsa","price":"499.9","category_id":4,"category":"Ropa","store_url":"https://tienda.example/b","priority":"alta","note":"roja","status":"activa","conversion":null,"created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z"}`

func TestWishCreateUpdateDelete(t *testing.T) {
	svc := &fakeService{}
	svc.wish().item = bolsa()
	h := server(svc)
	body := `{"name":"Bolsa","price":"499.90","category_id":4,"store_url":"https://tienda.example/b","priority":"alta","note":"roja"}`
	rec := do(h, "POST", "/personal/wishlist", body)
	if rec.Code != 201 || strings.TrimSpace(rec.Body.String()) != bolsaJSON {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	in := svc.wish().input
	if in.Name != "Bolsa" || !in.Price.Equal(d("499.9")) || in.CategoryID != 4 || in.StoreURL != "https://tienda.example/b" || in.Priority != "alta" || in.Note != "roja" || svc.gotActor != her {
		t.Errorf("service got %+v as %+v", in, svc.gotActor)
	}
	if rec := do(h, "PUT", "/personal/wishlist/5", body); rec.Code != 200 || svc.wish().id != 5 {
		t.Errorf("update: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "DELETE", "/personal/wishlist/5", ""); rec.Code != 204 || svc.wish().deleted != 5 || rec.Body.Len() != 0 {
		t.Errorf("delete: %d %s", rec.Code, rec.Body)
	}
	for name, bad := range map[string]string{
		"status in the body": `{"name":"x","price":"1","category_id":1,"status":"convertida"}`,
		"link in the body":   `{"name":"x","price":"1","category_id":1,"future_expense_id":3}`,
	} {
		if rec := do(h, "POST", "/personal/wishlist", bad); rec.Code != 400 {
			t.Errorf("%s: status %d, want 400 (unknown field)", name, rec.Code)
		}
	}
	if rec := do(h, "PUT", "/personal/wishlist/abc", body); rec.Code != 404 {
		t.Errorf("malformed id: %d, want 404", rec.Code)
	}
}

func TestWishList(t *testing.T) {
	svc := &fakeService{}
	conv := bolsa()
	conv.ID = 6
	conv.StoreURL, conv.Note = "", ""
	conv.Link = &domain.Conversion{Type: domain.ConvertedRequest, TargetID: 31, RequestState: "solicitada"}
	svc.wish().list = []domain.WishItem{bolsa(), conv}
	rec := do(server(svc), "GET", "/personal/wishlist", "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		bolsaJSON,
		`"store_url":null,"priority":"alta","note":null,"status":"convertida","conversion":{"type":"peticion","target_id":31,"request_state":"solicitada"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}
	if empty := strings.TrimSpace(do(server(&fakeService{}), "GET", "/personal/wishlist", "").Body.String()); empty != "[]" {
		t.Errorf("empty list = %s; want []", empty)
	}

	// A rejected request leaves the item active but still names the old target.
	conv.Link.RequestState = "rechazada"
	svc.wish().list = []domain.WishItem{conv}
	if body := do(server(svc), "GET", "/personal/wishlist", "").Body.String(); !strings.Contains(body, `"status":"activa"`) || !strings.Contains(body, `"request_state":"rechazada"`) {
		t.Errorf("rejected request: %s", body)
	}
}

func TestWishConversions(t *testing.T) {
	svc := &fakeService{}
	conv := bolsa()
	conv.Link = &domain.Conversion{Type: domain.ConvertedFuture, TargetID: 8}
	svc.wish().item = conv
	h := server(svc)

	rec := do(h, "POST", "/personal/wishlist/5/convert/future-expense", `{"target_amount":"450","due_date":"2026-12-20"}`)
	if rec.Code != 200 || svc.wish().id != 5 || svc.wish().future.TargetAmount == nil || !svc.wish().future.TargetAmount.Equal(d("450")) || svc.wish().future.DueDate != "2026-12-20" {
		t.Fatalf("future: %d %s %+v", rec.Code, rec.Body, svc.wish().future)
	}
	if !strings.Contains(rec.Body.String(), `"status":"convertida","conversion":{"type":"gasto_futuro","target_id":8,"request_state":null}`) {
		t.Errorf("future body = %s", rec.Body)
	}
	if rec := do(h, "POST", "/personal/wishlist/5/convert/future-expense", `{"due_date":"2026-12-20"}`); rec.Code != 200 || svc.wish().future.TargetAmount != nil {
		t.Errorf("default target must stay nil: %d %+v", rec.Code, svc.wish().future)
	}

	rec = do(h, "POST", "/personal/wishlist/5/convert/expense", `{"amount":"400","date":"2026-10-09","category_id":7}`)
	if rec.Code != 200 || svc.wish().expense.CategoryID != 7 || svc.wish().expense.Date != "2026-10-09" || !svc.wish().expense.Amount.Equal(d("400")) {
		t.Errorf("expense: %d %s %+v", rec.Code, rec.Body, svc.wish().expense)
	}
	rec = do(h, "POST", "/personal/wishlist/5/convert/expense", `{}`)
	if rec.Code != 200 || svc.wish().expense.Amount != nil {
		t.Errorf("empty expense body: %d %+v", rec.Code, svc.wish().expense)
	}

	rec = do(h, "POST", "/personal/wishlist/5/convert/request", `{"category":"Personal","amount":"450","date":"2026-10-10"}`)
	if rec.Code != 200 || svc.wish().request.Category != "Personal" || !svc.wish().request.Amount.Equal(d("450")) || svc.wish().request.Date != "2026-10-10" || svc.gotActor != her {
		t.Errorf("request: %d %s %+v", rec.Code, rec.Body, svc.wish().request)
	}
}

func TestWishErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrWishInvalid, 400, "invalid_wishlist_item"},
		{domain.ErrFutureInvalid, 400, "invalid_future_expense"},
		{domain.ErrInvalidInput, 400, "invalid_personal"},
		{requestsdomain.ErrInvalidInput, 400, "invalid_expense_request"},
		{domain.ErrAlreadyConverted, 409, "already_converted"},
		{domain.ErrNotFound, 404, "not_found"},
		{domain.ErrForbidden, 403, "forbidden"},
		{requestsdomain.ErrRateLimited, 429, "rate_limited"},
		{errors.New("boom"), 500, "internal_error"},
	}
	for _, c := range cases {
		h := server(&fakeService{err: c.err})
		for _, path := range []string{"/personal/wishlist/5/convert/future-expense", "/personal/wishlist/5/convert/expense", "/personal/wishlist/5/convert/request"} {
			rec := do(h, "POST", path, `{}`)
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), `"error":"`+c.code+`"`) {
				t.Errorf("%s with %v: %d %s; want %d %s", path, c.err, rec.Code, rec.Body, c.status, c.code)
			}
		}
	}
	rec := do(server(&fakeService{err: requestsdomain.ErrRateLimited}), "POST", "/personal/wishlist/5/convert/request", `{}`)
	if rec.Header().Get("Retry-After") != "3600" {
		t.Errorf("Retry-After = %q; want 3600", rec.Header().Get("Retry-After"))
	}
	if rec := do(server(&fakeService{err: domain.ErrWishInvalid}), "POST", "/personal/wishlist", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("create with invalid input: %d", rec.Code)
	}
}
