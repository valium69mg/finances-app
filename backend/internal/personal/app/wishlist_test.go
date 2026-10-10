package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

type fakeWish struct {
	user string
	domain.WishItem
}

func (r *fakeRepo) wish(user string, id int) (*fakeWish, error) {
	for _, w := range r.wishes {
		if w.ID == id && w.user == user {
			return w, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) CreateWish(_ context.Context, user string, v domain.ValidWish) (domain.WishItem, error) {
	w := &fakeWish{user: user, WishItem: domain.WishItem{
		ID: len(r.wishes) + 1, Name: v.Name, Price: v.Price, CategoryID: v.CategoryID, StoreURL: v.StoreURL,
		Priority: v.Priority, Note: v.Note, CreatedAt: time.Date(2026, 10, 1, 0, 0, len(r.wishes), 0, time.UTC),
	}}
	r.wishes = append(r.wishes, w)
	return w.WishItem, nil
}

func (r *fakeRepo) GetWish(_ context.Context, user string, id int) (domain.WishItem, error) {
	w, err := r.wish(user, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	return w.WishItem, nil
}

func (r *fakeRepo) ListWish(_ context.Context, user string) ([]domain.WishItem, error) {
	out := []domain.WishItem{}
	for i := len(r.wishes) - 1; i >= 0; i-- {
		if r.wishes[i].user == user {
			out = append(out, r.wishes[i].WishItem)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateWish(_ context.Context, user string, id int, v domain.ValidWish) (domain.WishItem, error) {
	w, err := r.wish(user, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	w.Name, w.Price, w.CategoryID, w.StoreURL, w.Priority, w.Note = v.Name, v.Price, v.CategoryID, v.StoreURL, v.Priority, v.Note
	return w.WishItem, nil
}

func (r *fakeRepo) DeleteWish(_ context.Context, user string, id int) error {
	for i, w := range r.wishes {
		if w.ID == id && w.user == user {
			r.wishes = append(r.wishes[:i], r.wishes[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeRepo) activeWish(user string, id int) (*fakeWish, error) {
	w, err := r.wish(user, id)
	if err != nil {
		return nil, err
	}
	if w.Status() != domain.WishActive {
		return nil, domain.ErrAlreadyConverted
	}
	return w, nil
}

func (r *fakeRepo) ConvertWishToFuture(_ context.Context, user string, id int, plan func(domain.WishItem) (domain.FutureValidated, error)) (domain.WishItem, error) {
	w, err := r.activeWish(user, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	v, err := plan(w.WishItem)
	if err != nil {
		return domain.WishItem{}, err
	}
	f, _ := r.CreateFutureExpense(context.Background(), user, v)
	w.Link = &domain.Conversion{Type: domain.ConvertedFuture, TargetID: f.ID}
	return w.WishItem, nil
}

func (r *fakeRepo) ConvertWishToExpense(_ context.Context, user string, id int, plan func(domain.WishItem) (domain.ValidMovement, error)) (domain.WishItem, error) {
	w, err := r.activeWish(user, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	v, err := plan(w.WishItem)
	if err != nil {
		return domain.WishItem{}, err
	}
	m, _ := r.CreateMovement(context.Background(), user, v)
	w.Link = &domain.Conversion{Type: domain.ConvertedExpense, TargetID: m.ID}
	return w.WishItem, nil
}

func (r *fakeRepo) LinkWishRequest(_ context.Context, user string, id, requestID int) (domain.WishItem, error) {
	if r.linkErr != nil {
		return domain.WishItem{}, r.linkErr
	}
	w, err := r.activeWish(user, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	w.Link = &domain.Conversion{Type: domain.ConvertedRequest, TargetID: requestID, RequestState: "solicitada"}
	return w.WishItem, nil
}

type fakeRequests struct {
	created   []app.RequestDraft
	cancelled []int
	err       error
	cancelErr error
}

func (f *fakeRequests) Create(_ context.Context, _ app.Identity, d app.RequestDraft) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.created = append(f.created, d)
	return 100 + len(f.created), nil
}

func (f *fakeRequests) Cancel(_ context.Context, _ app.Identity, id int) error {
	f.cancelled = append(f.cancelled, id)
	return f.cancelErr
}

func gastoID(t *testing.T, svc *app.Service, user app.Identity, name string) int {
	t.Helper()
	cats, err := svc.Categories(ctx, user, "Gasto")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("no Gasto category %q", name)
	return 0
}

func newWish(t *testing.T, svc *app.Service, user app.Identity, name, price, priority string) domain.WishItem {
	t.Helper()
	w, err := svc.CreateWish(ctx, user, domain.WishInput{
		Name: name, Price: d(price), CategoryID: gastoID(t, svc, user, "Ropa"), Priority: priority,
	})
	if err != nil {
		t.Fatalf("create wish: %v", err)
	}
	return w
}

func TestWishlistForbidsTheOwnerAndAnonymous(t *testing.T) {
	svc := newSvc(newRepo(), noCfg).WithRequests(&fakeRequests{})
	for _, actor := range []app.Identity{owner, {}, {UserID: "x", Role: "admin"}} {
		calls := map[string]error{}
		_, calls["list"] = svc.Wishlist(ctx, actor)
		_, calls["create"] = svc.CreateWish(ctx, actor, domain.WishInput{})
		_, calls["update"] = svc.UpdateWish(ctx, actor, 1, domain.WishInput{})
		calls["delete"] = svc.DeleteWish(ctx, actor, 1)
		_, calls["future"] = svc.ConvertWishToFuture(ctx, actor, 1, app.WishFutureInput{})
		_, calls["expense"] = svc.ConvertWishToExpense(ctx, actor, 1, app.WishExpenseInput{})
		_, calls["request"] = svc.ConvertWishToRequest(ctx, actor, 1, app.WishRequestInput{})
		for name, err := range calls {
			if !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("%s as %+v: error = %v; want ErrForbidden", name, actor, err)
			}
		}
	}
}

func TestWishCategoryMustBeHerGasto(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	mine := gastoID(t, svc, her, "Ropa")
	theirs := gastoID(t, svc, other, "Ropa")
	income := func() int {
		cats, _ := svc.Categories(ctx, her, "Ingreso")
		return cats[0].ID
	}()
	for name, id := range map[string]int{"foreign": theirs, "income": income, "unknown": 9999, "zero": 0} {
		_, err := svc.CreateWish(ctx, her, domain.WishInput{Name: "x", Price: d("10"), CategoryID: id})
		if !errors.Is(err, domain.ErrWishInvalid) {
			t.Errorf("%s category: error = %v; want ErrWishInvalid", name, err)
		}
	}
	if _, err := svc.CreateWish(ctx, her, domain.WishInput{Name: "x", Price: d("10"), CategoryID: mine}); err != nil {
		t.Fatalf("own category: %v", err)
	}
	w := newWish(t, svc, her, "Bolsa", "500", "")
	if _, err := svc.UpdateWish(ctx, her, w.ID, domain.WishInput{Name: "x", Price: d("10"), CategoryID: theirs}); !errors.Is(err, domain.ErrWishInvalid) {
		t.Errorf("update with a foreign category: %v; want ErrWishInvalid", err)
	}
}

func TestWishlistScopedAndOrdered(t *testing.T) {
	svc := newSvc(newRepo(), noCfg)
	low := newWish(t, svc, her, "baja vieja", "10", "baja")
	high := newWish(t, svc, her, "alta", "10", "alta")
	mid := newWish(t, svc, her, "media", "10", "")
	high2 := newWish(t, svc, her, "alta nueva", "10", "alta")
	foreign := newWish(t, svc, other, "ajena", "10", "alta")
	if _, err := svc.ConvertWishToExpense(ctx, her, high.ID, app.WishExpenseInput{}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Wishlist(ctx, her)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	want := []int{high2.ID, mid.ID, low.ID, high.ID}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v; want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v; want active by priority then newest, converted last: %v", ids, want)
		}
	}
	if _, err := svc.UpdateWish(ctx, her, foreign.ID, domain.WishInput{Name: "x", Price: d("1"), CategoryID: gastoID(t, svc, her, "Ropa")}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("update of another user's item: %v; want ErrNotFound", err)
	}
	if err := svc.DeleteWish(ctx, her, foreign.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("delete of another user's item: %v; want ErrNotFound", err)
	}
	if _, err := svc.ConvertWishToExpense(ctx, her, foreign.ID, app.WishExpenseInput{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("convert of another user's item: %v; want ErrNotFound", err)
	}
}

func TestConvertToFutureDefaultsAndRules(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	w := newWish(t, svc, her, "Bolsa", "500.50", "alta")
	if _, err := svc.ConvertWishToFuture(ctx, her, w.ID, app.WishFutureInput{}); !errors.Is(err, domain.ErrFutureInvalid) {
		t.Fatalf("missing due date: %v; want ErrFutureInvalid", err)
	}
	got, err := svc.ConvertWishToFuture(ctx, her, w.ID, app.WishFutureInput{DueDate: "2026-12-01"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Link == nil || got.Link.Type != domain.ConvertedFuture || got.Status() != domain.WishConverted {
		t.Fatalf("link = %+v", got.Link)
	}
	if len(r.items) != 1 || r.items[0].Name != "Bolsa" || !r.items[0].Target.Equal(d("500.50")) {
		t.Fatalf("future expense = %+v; want named after the item with the price as target", r.items)
	}
	if _, err := svc.ConvertWishToFuture(ctx, her, w.ID, app.WishFutureInput{DueDate: "2026-12-01"}); !errors.Is(err, domain.ErrAlreadyConverted) {
		t.Errorf("second conversion: %v; want ErrAlreadyConverted", err)
	}
	// An explicit target replaces the price.
	w2 := newWish(t, svc, her, "Zapatos", "800", "")
	target := d("700")
	if _, err := svc.ConvertWishToFuture(ctx, her, w2.ID, app.WishFutureInput{TargetAmount: &target, DueDate: "2026-12-01"}); err != nil {
		t.Fatal(err)
	}
	if !r.items[1].Target.Equal(d("700")) {
		t.Errorf("target = %s; want 700", r.items[1].Target)
	}
}

func TestConvertToExpenseDefaultsAndRules(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	w := newWish(t, svc, her, "Bolsa", "500", "")
	got, err := svc.ConvertWishToExpense(ctx, her, w.ID, app.WishExpenseInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Link == nil || got.Link.Type != domain.ConvertedExpense {
		t.Fatalf("link = %+v", got.Link)
	}
	m := r.moves[0]
	if m.Kind != domain.KindExpense || !m.Amount.Equal(d("500")) || m.Date != "2026-10-10" || m.CategoryID != w.CategoryID || m.Description != "Bolsa" {
		t.Errorf("expense = %+v; want the price, today in the item's category", m)
	}
	// Overrides, a foreign category and a bad amount.
	w2 := newWish(t, svc, her, "Labial", "200", "")
	amount := d("150.25")
	other := gastoID(t, svc, her, "Belleza")
	if _, err := svc.ConvertWishToExpense(ctx, her, w2.ID, app.WishExpenseInput{Amount: &amount, Date: "2026-10-05", CategoryID: other}); err != nil {
		t.Fatal(err)
	}
	if m := r.moves[1]; !m.Amount.Equal(d("150.25")) || m.Date != "2026-10-05" || m.CategoryID != other {
		t.Errorf("overridden expense = %+v", m)
	}
	w3 := newWish(t, svc, her, "Rimel", "100", "")
	bad := d("1.234")
	if _, err := svc.ConvertWishToExpense(ctx, her, w3.ID, app.WishExpenseInput{Amount: &bad}); !errors.Is(err, domain.ErrWishInvalid) {
		t.Errorf("3 decimals: %v; want ErrWishInvalid", err)
	}
	if _, err := svc.ConvertWishToExpense(ctx, her, w3.ID, app.WishExpenseInput{CategoryID: gastoID(t, svc, newOtherIdentity(), "Ropa")}); !errors.Is(err, domain.ErrWishInvalid) {
		t.Errorf("foreign category: %v; want ErrWishInvalid", err)
	}
	if _, err := svc.ConvertWishToExpense(ctx, her, w3.ID, app.WishExpenseInput{Date: "10/10/2026"}); !errors.Is(err, domain.ErrWishInvalid) {
		t.Errorf("bad date: %v; want ErrWishInvalid", err)
	}
	if len(r.moves) != 2 {
		t.Errorf("%d expenses; a failed conversion must not write one", len(r.moves))
	}
}

func newOtherIdentity() app.Identity { return other }

func TestConvertToRequest(t *testing.T) {
	r := newRepo()
	reqs := &fakeRequests{}
	svc := newSvc(r, noCfg).WithRequests(reqs)
	w := newWish(t, svc, her, "Bolsa", "500", "")
	if _, err := svc.ConvertWishToRequest(ctx, her, w.ID, app.WishRequestInput{}); !errors.Is(err, domain.ErrWishInvalid) {
		t.Fatalf("missing category: %v; want ErrWishInvalid", err)
	}
	if len(reqs.created) != 0 {
		t.Fatal("a request was created without a category")
	}
	got, err := svc.ConvertWishToRequest(ctx, her, w.ID, app.WishRequestInput{Category: "Personal"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Link == nil || got.Link.Type != domain.ConvertedRequest || got.Link.TargetID != 101 || got.Status() != domain.WishConverted {
		t.Fatalf("link = %+v", got.Link)
	}
	if d0 := reqs.created[0]; !d0.Amount.Equal(d("500")) || d0.Description != "Bolsa" || d0.Category != "Personal" {
		t.Errorf("draft = %+v", d0)
	}
	if _, err := svc.ConvertWishToRequest(ctx, her, w.ID, app.WishRequestInput{Category: "Personal"}); !errors.Is(err, domain.ErrAlreadyConverted) {
		t.Errorf("second conversion: %v; want ErrAlreadyConverted", err)
	}
	if len(reqs.created) != 1 {
		t.Errorf("%d requests; a converted item must not create another", len(reqs.created))
	}
}

func TestConvertToRequestErrorsAndCompensation(t *testing.T) {
	r := newRepo()
	reqs := &fakeRequests{err: errors.New("rate limited")}
	svc := newSvc(r, noCfg).WithRequests(reqs)
	w := newWish(t, svc, her, "Bolsa", "500", "")
	if _, err := svc.ConvertWishToRequest(ctx, her, w.ID, app.WishRequestInput{Category: "Personal"}); err == nil || err.Error() != "rate limited" {
		t.Fatalf("request error = %v; want it passed through", err)
	}
	if got, _ := svc.Wishlist(ctx, her); got[0].Status() != domain.WishActive {
		t.Error("the item must stay active when the request fails")
	}

	// The link fails after the request exists: the request is withdrawn.
	reqs.err = nil
	r.linkErr = errors.New("db down")
	if _, err := svc.ConvertWishToRequest(ctx, her, w.ID, app.WishRequestInput{Category: "Personal"}); err == nil {
		t.Fatal("want the link error")
	}
	if len(reqs.cancelled) != 1 || reqs.cancelled[0] != 101 {
		t.Errorf("cancelled = %v; want the created request withdrawn", reqs.cancelled)
	}

	// When the withdrawal fails too, both errors are reported.
	reqs.cancelErr = errors.New("cancel down")
	if _, err := svc.ConvertWishToRequest(ctx, her, w.ID, app.WishRequestInput{Category: "Personal"}); err == nil || !errors.Is(err, r.linkErr) {
		t.Fatalf("error = %v; want the link error joined", err)
	}

	// Without the requests module wired the conversion fails.
	if _, err := newSvc(r, noCfg).ConvertWishToRequest(ctx, her, w.ID, app.WishRequestInput{Category: "x"}); err == nil {
		t.Error("want an error when requests are not wired")
	}
}

func TestEditAndDeleteKeepTheLink(t *testing.T) {
	r := newRepo()
	svc := newSvc(r, noCfg)
	w := newWish(t, svc, her, "Bolsa", "500", "")
	if _, err := svc.ConvertWishToExpense(ctx, her, w.ID, app.WishExpenseInput{}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.UpdateWish(ctx, her, w.ID, domain.WishInput{Name: "Bolsa grande", Price: d("600"), CategoryID: w.CategoryID, Priority: "baja"})
	if err != nil || got.Link == nil || got.Name != "Bolsa grande" {
		t.Fatalf("update = %+v, %v; want the link kept", got, err)
	}
	if err := svc.DeleteWish(ctx, her, w.ID); err != nil {
		t.Fatal(err)
	}
	if len(r.moves) != 1 {
		t.Errorf("%d expenses; deleting the item must not touch what it became", len(r.moves))
	}
}
