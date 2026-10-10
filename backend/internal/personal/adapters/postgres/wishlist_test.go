package postgres_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

func wish(f fixture, name, price string) domain.ValidWish {
	return domain.ValidWish{Name: name, Price: d(price), CategoryID: f.ropa.ID, Priority: domain.PriorityMedium}
}

func (f fixture) wishItem(t *testing.T, user, name string) domain.WishItem {
	t.Helper()
	w := wish(f, name, "500")
	if user == f.other {
		w.CategoryID = f.otherRopa.ID
	}
	it, err := f.repo.CreateWish(ctx, user, w)
	if err != nil {
		t.Fatalf("create wish: %v", err)
	}
	return it
}

// request inserts an expense request of the user in the given state straight
// into the table (the wishlist only reads its state).
func (f fixture) request(t *testing.T, user, status string) int {
	t.Helper()
	var id int
	var err error
	switch status {
	case "solicitada":
		err = f.pool.QueryRow(ctx, `INSERT INTO expense_requests (requester_id, amount, description) VALUES ($1::text::uuid, 10, 'x') RETURNING id`, user).Scan(&id)
	case "cancelada":
		err = f.pool.QueryRow(ctx, `INSERT INTO expense_requests (requester_id, amount, description, status, decided_at) VALUES ($1::text::uuid, 10, 'x', 'cancelada', now()) RETURNING id`, user).Scan(&id)
	case "rechazada":
		err = f.pool.QueryRow(ctx, `INSERT INTO expense_requests (requester_id, amount, description, status, decided_at, decided_by, decision_comment) VALUES ($1::text::uuid, 10, 'x', 'rechazada', now(), $1::text::uuid, 'no') RETURNING id`, user).Scan(&id)
	case "aprobada":
		err = f.pool.QueryRow(ctx, `INSERT INTO expense_requests (requester_id, amount, description, status, decided_at, decided_by, result_kind) VALUES ($1::text::uuid, 10, 'x', 'aprobada', now(), $1::text::uuid, 'gasto') RETURNING id`, user).Scan(&id)
	}
	if err != nil {
		t.Fatalf("insert %s request: %v", status, err)
	}
	return id
}

func (f fixture) setRequestStatus(t *testing.T, id int, status string) {
	t.Helper()
	q := map[string]string{
		"rechazada": `UPDATE expense_requests SET status='rechazada', decided_at=now(), decided_by=requester_id, decision_comment='no' WHERE id=$1`,
		"cancelada": `UPDATE expense_requests SET status='cancelada', decided_at=now() WHERE id=$1`,
	}[status]
	if _, err := f.pool.Exec(ctx, q, id); err != nil {
		t.Fatal(err)
	}
}

func TestWishCRUDAndScoping(t *testing.T) {
	f := setup(t)
	in := domain.ValidWish{Name: "Bolsa", Price: d("499.90"), CategoryID: f.ropa.ID, StoreURL: "https://tienda.example/b", Priority: domain.PriorityHigh, Note: "roja"}
	it, err := f.repo.CreateWish(ctx, f.her, in)
	if err != nil {
		t.Fatal(err)
	}
	if it.CategoryName != "Ropa" || !it.Price.Equal(d("499.90")) || it.StoreURL != in.StoreURL || it.Priority != domain.PriorityHigh || it.Note != "roja" || it.Link != nil || it.Status() != domain.WishActive {
		t.Errorf("created = %+v", it)
	}
	bare, err := f.repo.CreateWish(ctx, f.her, wish(f, "Sin extras", "10"))
	if err != nil || bare.StoreURL != "" || bare.Note != "" {
		t.Errorf("bare = %+v, %v; want empty optional fields", bare, err)
	}

	up := in
	up.Name, up.Priority, up.StoreURL, up.Note = "Bolsa grande", domain.PriorityLow, "", ""
	got, err := f.repo.UpdateWish(ctx, f.her, it.ID, up)
	if err != nil || got.Name != "Bolsa grande" || got.Priority != domain.PriorityLow || got.StoreURL != "" || got.Note != "" {
		t.Errorf("updated = %+v, %v", got, err)
	}

	list, _ := f.repo.ListWish(ctx, f.her)
	if len(list) != 2 || list[0].ID != bare.ID {
		t.Errorf("list = %+v; want 2 items newest first", list)
	}
	if other, _ := f.repo.ListWish(ctx, f.other); len(other) != 0 {
		t.Errorf("another user sees %d items", len(other))
	}
	if _, err := f.repo.GetWish(ctx, f.other, it.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign get: %v; want ErrNotFound", err)
	}
	if _, err := f.repo.UpdateWish(ctx, f.other, it.ID, wish(f, "x", "1")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign update: %v; want ErrNotFound", err)
	}
	if err := f.repo.DeleteWish(ctx, f.other, it.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign delete: %v; want ErrNotFound", err)
	}
	if err := f.repo.DeleteWish(ctx, f.her, it.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DeleteWish(ctx, f.her, it.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete: %v; want ErrNotFound", err)
	}
}

func TestWishCategoryOwnershipAndKindAreEnforcedByTheDatabase(t *testing.T) {
	f := setup(t)
	for name, id := range map[string]int{"another user's": f.otherRopa.ID, "an Ingreso": f.trabajo.ID, "an Ahorro": f.ahorro.ID, "unknown": 999999} {
		in := wish(f, "x", "10")
		in.CategoryID = id
		if _, err := f.repo.CreateWish(ctx, f.her, in); !errors.Is(err, domain.ErrWishInvalid) {
			t.Errorf("create with %s category: %v; want ErrWishInvalid", name, err)
		}
	}
	it := f.wishItem(t, f.her, "Bolsa")
	in := wish(f, "x", "10")
	in.CategoryID = f.otherRopa.ID
	if _, err := f.repo.UpdateWish(ctx, f.her, it.ID, in); !errors.Is(err, domain.ErrWishInvalid) {
		t.Errorf("update with a foreign category: %v; want ErrWishInvalid", err)
	}
	// Renaming a category keeps the item pointing at it.
	if _, err := f.repo.RenameCategory(ctx, f.her, f.ropa.ID, "Vestuario"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.repo.GetWish(ctx, f.her, it.ID); got.CategoryName != "Vestuario" {
		t.Errorf("category = %q; want the rename to propagate", got.CategoryName)
	}
}

func TestWishAtMostOneLinkCheck(t *testing.T) {
	f := setup(t)
	it := f.wishItem(t, f.her, "Bolsa")
	fut := f.item(t, f.her, "100")
	req := f.request(t, f.her, "solicitada")
	_, err := f.pool.Exec(ctx, `UPDATE personal_wishlist_items SET future_expense_id = $2, expense_request_id = $3 WHERE id = $1`, int64(it.ID), int64(fut.ID), int64(req))
	if err == nil || !strings.Contains(err.Error(), "personal_wishlist_one_link") {
		t.Errorf("two links error = %v; want personal_wishlist_one_link", err)
	}
	m, _ := f.repo.CreateMovement(ctx, f.her, mv(domain.KindExpense, f.ropa, "10", "2026-10-01"))
	_, err = f.pool.Exec(ctx, `UPDATE personal_wishlist_items SET future_expense_id = $2, expense_movement_id = $3 WHERE id = $1`, int64(it.ID), int64(fut.ID), int64(m.ID))
	if err == nil || !strings.Contains(err.Error(), "personal_wishlist_one_link") {
		t.Errorf("future + expense error = %v; want personal_wishlist_one_link", err)
	}
	// A link across users is rejected by the composite foreign keys.
	foreignFut := f.item(t, f.other, "100")
	if _, err := f.pool.Exec(ctx, `UPDATE personal_wishlist_items SET future_expense_id = $2 WHERE id = $1`, int64(it.ID), int64(foreignFut.ID)); err == nil {
		t.Error("linking another user's future expense must fail")
	}
	otherCat, _ := f.repo.ListCategories(ctx, f.other, domain.KindExpense)
	om, _ := f.repo.CreateMovement(ctx, f.other, mv(domain.KindExpense, find(t, otherCat, "Ropa"), "10", "2026-10-01"))
	if _, err := f.pool.Exec(ctx, `UPDATE personal_wishlist_items SET expense_movement_id = $2 WHERE id = $1`, int64(it.ID), int64(om.ID)); err == nil {
		t.Error("linking another user's Gasto must fail")
	}
}

func TestWishStoreURLCheck(t *testing.T) {
	f := setup(t)
	it := f.wishItem(t, f.her, "Bolsa")
	if _, err := f.pool.Exec(ctx, `UPDATE personal_wishlist_items SET store_url = 'javascript:alert(1)' WHERE id = $1`, int64(it.ID)); err == nil {
		t.Error("the database must reject a non http(s) link")
	}
}

func TestWishConvertToFuture(t *testing.T) {
	f := setup(t)
	it := f.wishItem(t, f.her, "Bolsa")
	got, err := f.repo.ConvertWishToFuture(ctx, f.her, it.ID, func(w domain.WishItem) (domain.FutureValidated, error) {
		return domain.FutureValidated{Name: w.Name, Target: w.Price, DueDate: "2026-12-20"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Link == nil || got.Link.Type != domain.ConvertedFuture || got.Status() != domain.WishConverted {
		t.Fatalf("link = %+v", got.Link)
	}
	fut, err := f.repo.GetFutureExpense(ctx, f.her, got.Link.TargetID)
	if err != nil || fut.Name != "Bolsa" || !fut.Target.Equal(d("500")) {
		t.Errorf("future expense = %+v, %v", fut, err)
	}
	// Converting a converted item is refused and writes nothing.
	_, err = f.repo.ConvertWishToFuture(ctx, f.her, it.ID, func(w domain.WishItem) (domain.FutureValidated, error) {
		return domain.FutureValidated{Name: w.Name, Target: w.Price, DueDate: "2026-12-20"}, nil
	})
	if !errors.Is(err, domain.ErrAlreadyConverted) {
		t.Errorf("second conversion: %v; want ErrAlreadyConverted", err)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_future_expenses WHERE user_id = $1::text::uuid`, f.her); n != 1 {
		t.Errorf("%d future expenses; want 1", n)
	}

	// Deleting the future expense reactivates the item (link cleared by SET NULL).
	if err := f.repo.DeleteFutureExpense(ctx, f.her, fut.ID); err != nil {
		t.Fatal(err)
	}
	back, _ := f.repo.GetWish(ctx, f.her, it.ID)
	if back.Link != nil || back.Status() != domain.WishActive {
		t.Errorf("after deleting the future expense: %+v; want active with no link", back)
	}
}

func TestWishConvertRollsBackWhenAnyStepFails(t *testing.T) {
	f := setup(t)
	it := f.wishItem(t, f.her, "Bolsa")

	// The plan fails: nothing is written.
	boom := errors.New("plan failed")
	if _, err := f.repo.ConvertWishToFuture(ctx, f.her, it.ID, func(domain.WishItem) (domain.FutureValidated, error) {
		return domain.FutureValidated{}, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("error = %v; want the plan error", err)
	}
	// The insert fails after the transaction began (an Ingreso category on a Gasto row).
	if _, err := f.repo.ConvertWishToExpense(ctx, f.her, it.ID, func(domain.WishItem) (domain.ValidMovement, error) {
		return domain.ValidMovement{Kind: domain.KindExpense, CategoryID: f.otherRopa.ID, Amount: d("10"), Description: "x", Date: "2026-10-01"}, nil
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v; want ErrInvalidInput from the insert", err)
	}
	// The link step fails (a request that does not exist): the future expense
	// inserted before it must roll back with it.
	if _, err := f.repo.LinkWishRequest(ctx, f.her, it.ID, 999999); err == nil {
		t.Fatal("linking a missing request must fail")
	}
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE kind = 'Gasto'`); n != 0 {
		t.Errorf("%d Gasto rows after a failed conversion; want 0", n)
	}
	got, _ := f.repo.GetWish(ctx, f.her, it.ID)
	if got.Link != nil {
		t.Errorf("link = %+v; want none", got.Link)
	}

	// A failure AFTER the future expense was inserted (the link UPDATE breaks on
	// a temporary CHECK): the insert must roll back with it.
	if _, err := f.pool.Exec(ctx, `ALTER TABLE personal_wishlist_items ADD CONSTRAINT test_never CHECK (updated_at < now() - interval '1 day') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.ConvertWishToFuture(ctx, f.her, it.ID, func(w domain.WishItem) (domain.FutureValidated, error) {
		return domain.FutureValidated{Name: w.Name, Target: w.Price, DueDate: "2026-12-20"}, nil
	}); err == nil {
		t.Fatal("the link update must fail")
	}
	if n := count(t, f, `SELECT count(*) FROM personal_future_expenses`); n != 0 {
		t.Errorf("%d future expenses after a failed link; want the insert rolled back", n)
	}
	if _, err := f.pool.Exec(ctx, `ALTER TABLE personal_wishlist_items DROP CONSTRAINT test_never`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.ConvertWishToExpense(ctx, f.her, it.ID, func(w domain.WishItem) (domain.ValidMovement, error) {
		return domain.ValidMovement{Kind: domain.KindExpense, CategoryID: w.CategoryID, Amount: w.Price, Description: w.Name, Date: "2026-10-01"}, nil
	}); err != nil {
		t.Fatalf("a clean conversion after the failures: %v", err)
	}
}

func TestWishConvertToExpenseAndDeletingTheGastoReactivates(t *testing.T) {
	f := setup(t)
	it := f.wishItem(t, f.her, "Bolsa")
	got, err := f.repo.ConvertWishToExpense(ctx, f.her, it.ID, func(w domain.WishItem) (domain.ValidMovement, error) {
		return domain.ValidMovement{Kind: domain.KindExpense, CategoryID: w.CategoryID, Amount: w.Price, Description: w.Name, Date: "2026-10-03"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Link == nil || got.Link.Type != domain.ConvertedExpense || got.Status() != domain.WishConverted {
		t.Fatalf("link = %+v", got.Link)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE id = $1 AND kind = 'Gasto' AND amount = 500`, int64(got.Link.TargetID)); n != 1 {
		t.Errorf("the linked Gasto is missing")
	}
	if err := f.repo.DeleteMovement(ctx, f.her, got.Link.TargetID); err != nil {
		t.Fatal(err)
	}
	back, _ := f.repo.GetWish(ctx, f.her, it.ID)
	if back.Link != nil || back.Status() != domain.WishActive {
		t.Errorf("after deleting the Gasto: %+v; want active", back)
	}
	// Reactivated: it converts again, replacing the old link.
	again, err := f.repo.ConvertWishToFuture(ctx, f.her, it.ID, func(w domain.WishItem) (domain.FutureValidated, error) {
		return domain.FutureValidated{Name: w.Name, Target: w.Price, DueDate: "2026-12-20"}, nil
	})
	if err != nil || again.Link.Type != domain.ConvertedFuture {
		t.Errorf("second life: %+v, %v", again, err)
	}
}

func TestWishRequestStatusIsDerived(t *testing.T) {
	f := setup(t)
	cases := []struct {
		state string
		want  domain.WishStatus
	}{
		{"solicitada", domain.WishConverted},
		{"aprobada", domain.WishConverted},
		{"rechazada", domain.WishActive},
		{"cancelada", domain.WishActive},
	}
	for _, c := range cases {
		it := f.wishItem(t, f.her, "item "+c.state)
		got, err := f.repo.LinkWishRequest(ctx, f.her, it.ID, f.request(t, f.her, c.state))
		if err != nil {
			t.Fatalf("%s: %v", c.state, err)
		}
		if got.Link == nil || got.Link.Type != domain.ConvertedRequest || got.Link.RequestState != c.state || got.Status() != c.want {
			t.Errorf("%s: link %+v status %s; want %s", c.state, got.Link, got.Status(), c.want)
		}
	}

	// A request that moves on: pending -> rechazada reactivates, and the item
	// can be converted again (the old link is replaced).
	it := f.wishItem(t, f.her, "Bolsa")
	reqID := f.request(t, f.her, "solicitada")
	if _, err := f.repo.LinkWishRequest(ctx, f.her, it.ID, reqID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.LinkWishRequest(ctx, f.her, it.ID, f.request(t, f.her, "solicitada")); !errors.Is(err, domain.ErrAlreadyConverted) {
		t.Fatalf("link while pending: %v; want ErrAlreadyConverted", err)
	}
	f.setRequestStatus(t, reqID, "rechazada")
	if got, _ := f.repo.GetWish(ctx, f.her, it.ID); got.Status() != domain.WishActive {
		t.Fatalf("status after rejection = %s; want activa", got.Status())
	}
	newReq := f.request(t, f.her, "solicitada")
	got, err := f.repo.LinkWishRequest(ctx, f.her, it.ID, newReq)
	if err != nil || got.Link.TargetID != newReq || got.Status() != domain.WishConverted {
		t.Errorf("relink = %+v, %v; want the new request", got, err)
	}
	// Converting to a future expense replaces the request link and clears it.
	f.setRequestStatus(t, newReq, "cancelada")
	to, err := f.repo.ConvertWishToFuture(ctx, f.her, it.ID, func(w domain.WishItem) (domain.FutureValidated, error) {
		return domain.FutureValidated{Name: w.Name, Target: w.Price, DueDate: "2026-12-20"}, nil
	})
	if err != nil || to.Link.Type != domain.ConvertedFuture {
		t.Fatalf("convert after cancellation: %+v, %v", to, err)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_wishlist_items WHERE id = $1 AND expense_request_id IS NULL`, int64(it.ID)); n != 1 {
		t.Error("the old request link must be cleared by the new conversion")
	}
}

func TestWishDeleteKeepsWhatItBecame(t *testing.T) {
	f := setup(t)
	it := f.wishItem(t, f.her, "Bolsa")
	got, err := f.repo.ConvertWishToExpense(ctx, f.her, it.ID, func(w domain.WishItem) (domain.ValidMovement, error) {
		return domain.ValidMovement{Kind: domain.KindExpense, CategoryID: w.CategoryID, Amount: w.Price, Description: w.Name, Date: "2026-10-03"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DeleteWish(ctx, f.her, it.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_movements WHERE id = $1`, int64(got.Link.TargetID)); n != 1 {
		t.Error("deleting the item removed its Gasto")
	}
	// Deleting the user cascades.
	f.wishItem(t, f.other, "ajena")
	if _, err := f.pool.Exec(ctx, `DELETE FROM users WHERE id = $1::text::uuid`, f.other); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_wishlist_items WHERE user_id = $1::text::uuid`, f.other); n != 0 {
		t.Error("the items of a deleted user must go with it")
	}
}

func TestWishConcurrentConversionsOnlyOneWins(t *testing.T) {
	f := setup(t)
	it := f.wishItem(t, f.her, "Bolsa")
	var wg sync.WaitGroup
	results := make([]error, 6)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.repo.ConvertWishToFuture(ctx, f.her, it.ID, func(w domain.WishItem) (domain.FutureValidated, error) {
				return domain.FutureValidated{Name: w.Name, Target: w.Price, DueDate: "2026-12-20"}, nil
			})
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, domain.ErrAlreadyConverted):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Errorf("%d conversions won; want exactly 1", wins)
	}
	if n := count(t, f, `SELECT count(*) FROM personal_future_expenses`); n != 1 {
		t.Errorf("%d future expenses; want 1", n)
	}
}
