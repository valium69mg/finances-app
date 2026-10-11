package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// The owner's read-only view: the target is validated by a directory and every
// write stays household-only.

const (
	herID     = "11111111-1111-4111-8111-111111111111"
	otherID   = "22222222-2222-4222-8222-222222222222"
	ownerID   = "33333333-3333-4333-8333-333333333333"
	unknownID = "44444444-4444-4444-8444-444444444444"
)

type fakeDirectory map[string]session.Role

func (f fakeDirectory) Role(_ context.Context, id string) (session.Role, error) {
	if role, ok := f[id]; ok {
		return role, nil
	}
	return "", domain.ErrNotFound
}

var (
	realOwner = session.Identity{UserID: ownerID, Role: session.RoleOwner}
	herReal   = session.Identity{UserID: herID, Role: session.RoleHousehold}
)

func readSvc(r *fakeRepo) *app.Service {
	return newSvc(r, noCfg).WithUsers(fakeDirectory{herID: session.RoleHousehold, otherID: session.RoleHousehold, ownerID: session.RoleOwner})
}

func TestOwnerReadsEveryRouteOfAHouseholdTarget(t *testing.T) {
	r := newRepo()
	svc := readSvc(r)
	her := herReal
	gasto := gastoID(t, svc, her, "Ropa")
	if _, err := svc.CreateMovement(ctx, her, domain.MovementInput{Kind: "Gasto", CategoryID: gasto, Amount: d("100"), Description: "Blusa", Date: "2026-10-02"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateFutureExpense(ctx, her, domain.FutureInput{Name: "Viaje", Target: d("1000"), DueDate: "2026-12-20"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateWish(ctx, her, domain.WishInput{Name: "Bolsa", Price: d("50"), CategoryID: gasto, Priority: "media"}); err != nil {
		t.Fatal(err)
	}
	seeds := r.seeds

	cats, err := svc.Categories(ctx, realOwner, herID, "")
	if err != nil || len(cats) == 0 {
		t.Fatalf("categories = %v, %v", cats, err)
	}
	moves, err := svc.Movements(ctx, realOwner, app.ListInput{UserID: herID, Month: "2026-10"})
	if err != nil || len(moves) != 1 {
		t.Fatalf("movements = %v, %v", moves, err)
	}
	dash, err := svc.Dashboard(ctx, realOwner, herID, "2026-10")
	if err != nil || dash.Totals.Expenses.String() != "100" {
		t.Fatalf("dashboard = %+v, %v", dash, err)
	}
	if _, err := svc.Period(ctx, realOwner, herID, "2026-10"); err != nil {
		t.Fatalf("period: %v", err)
	}
	list, err := svc.FutureExpenses(ctx, realOwner, herID)
	if err != nil || len(list.Plan.Items) != 1 {
		t.Fatalf("future expenses = %+v, %v", list, err)
	}
	one, err := svc.FutureExpense(ctx, realOwner, herID, list.Plan.Items[0].ID)
	if err != nil || one.Name != "Viaje" {
		t.Fatalf("future expense = %+v, %v", one, err)
	}
	wish, err := svc.Wishlist(ctx, realOwner, herID)
	if err != nil || len(wish) != 1 {
		t.Fatalf("wishlist = %v, %v", wish, err)
	}
	if r.seeds != seeds {
		t.Errorf("owner reads seeded categories %d times; a read-only view never writes", r.seeds-seeds)
	}
}

func TestOwnerReadScopesRowsToTheTarget(t *testing.T) {
	r := newRepo()
	svc := readSvc(r)
	other := session.Identity{UserID: otherID, Role: session.RoleHousehold}
	if _, err := svc.CreateWish(ctx, other, domain.WishInput{Name: "Ajena", Price: d("5"), CategoryID: gastoID(t, svc, other, "Ropa"), Priority: "media"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Wishlist(ctx, realOwner, herID); len(got) != 0 {
		t.Errorf("owner reading her got %d rows of another user", len(got))
	}
}

func TestOwnerReadDoesNotSeed(t *testing.T) {
	r := newRepo()
	svc := readSvc(r)
	cats, err := svc.Categories(ctx, realOwner, herID, "")
	if err != nil || len(cats) != 0 || r.seeds != 0 {
		t.Fatalf("categories = %v, err %v, seeds %d; want none and no seeding", cats, err, r.seeds)
	}
	if _, err := svc.Categories(ctx, herReal, "", ""); err != nil || r.seeds != 1 {
		t.Errorf("her own read must still seed: err %v, seeds %d", err, r.seeds)
	}
}

func TestOwnerNeedsAValidHouseholdTarget(t *testing.T) {
	svc := readSvc(newRepo())
	reads := func(target string) map[string]error {
		out := map[string]error{}
		_, out["categories"] = svc.Categories(ctx, realOwner, target, "")
		_, out["movements"] = svc.Movements(ctx, realOwner, app.ListInput{UserID: target})
		_, out["period"] = svc.Period(ctx, realOwner, target, "")
		_, out["dashboard"] = svc.Dashboard(ctx, realOwner, target, "")
		_, out["list"] = svc.FutureExpenses(ctx, realOwner, target)
		_, out["get"] = svc.FutureExpense(ctx, realOwner, target, 1)
		_, out["wishlist"] = svc.Wishlist(ctx, realOwner, target)
		return out
	}
	for target, want := range map[string]error{
		"": domain.ErrInvalidInput, "  ": domain.ErrInvalidInput, "nope": domain.ErrInvalidInput, "1": domain.ErrInvalidInput,
		ownerID: domain.ErrNotFound, unknownID: domain.ErrNotFound,
	} {
		for name, err := range reads(target) {
			if !errors.Is(err, want) {
				t.Errorf("%s with user_id %q: %v; want %v", name, target, err, want)
			}
		}
	}
}

func TestOwnerReadFailsClosedWithoutDirectory(t *testing.T) {
	if _, err := newSvc(newRepo(), noCfg).Wishlist(ctx, realOwner, herID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("error = %v; want ErrForbidden", err)
	}
}

func TestOwnerCanReadAnInactiveHouseholdUser(t *testing.T) {
	// The directory only reports the role: an inactive account keeps its history
	// readable for the owner.
	if _, err := readSvc(newRepo()).Wishlist(ctx, realOwner, herID); err != nil {
		t.Errorf("error = %v", err)
	}
}

func TestHouseholdIgnoresUserID(t *testing.T) {
	r := newRepo()
	svc := readSvc(r)
	other := session.Identity{UserID: otherID, Role: session.RoleHousehold}
	if _, err := svc.CreateWish(ctx, other, domain.WishInput{Name: "Ajena", Price: d("5"), CategoryID: gastoID(t, svc, other, "Ropa"), Priority: "media"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateWish(ctx, herReal, domain.WishInput{Name: "Mia", Price: d("5"), CategoryID: gastoID(t, svc, herReal, "Ropa"), Priority: "media"}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"", otherID, "garbage", ownerID} {
		got, err := svc.Wishlist(ctx, herReal, target)
		if err != nil || len(got) != 1 || got[0].Name != "Mia" {
			t.Errorf("user_id %q: %v, %v; want only her own row", target, got, err)
		}
	}
}

func TestOwnerCannotWriteEvenWithADirectory(t *testing.T) {
	svc := readSvc(newRepo()).WithRequests(&fakeRequests{})
	calls := map[string]error{}
	_, calls["create category"] = svc.CreateCategory(ctx, realOwner, "x")
	_, calls["rename category"] = svc.RenameCategory(ctx, realOwner, 1, "x")
	_, calls["create movement"] = svc.CreateMovement(ctx, realOwner, domain.MovementInput{})
	_, calls["update movement"] = svc.UpdateMovement(ctx, realOwner, 1, domain.MovementInput{})
	calls["delete movement"] = svc.DeleteMovement(ctx, realOwner, 1)
	_, calls["create future"] = svc.CreateFutureExpense(ctx, realOwner, domain.FutureInput{})
	_, calls["update future"] = svc.UpdateFutureExpense(ctx, realOwner, 1, domain.FutureInput{})
	calls["delete future"] = svc.DeleteFutureExpense(ctx, realOwner, 1)
	_, _, calls["contribute"] = svc.ContributeFuture(ctx, realOwner, 1, app.FutureSavingInput{})
	_, calls["assign"] = svc.AssignFuture(ctx, realOwner, 1, app.FutureAssignInput{})
	_, _, calls["pay"] = svc.PayFuture(ctx, realOwner, 1, app.FuturePayInput{})
	_, calls["create wish"] = svc.CreateWish(ctx, realOwner, domain.WishInput{})
	_, calls["update wish"] = svc.UpdateWish(ctx, realOwner, 1, domain.WishInput{})
	calls["delete wish"] = svc.DeleteWish(ctx, realOwner, 1)
	_, calls["convert future"] = svc.ConvertWishToFuture(ctx, realOwner, 1, app.WishFutureInput{})
	_, calls["convert expense"] = svc.ConvertWishToExpense(ctx, realOwner, 1, app.WishExpenseInput{})
	_, calls["convert request"] = svc.ConvertWishToRequest(ctx, realOwner, 1, app.WishRequestInput{})
	for name, err := range calls {
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s as owner: %v; want ErrForbidden", name, err)
		}
	}
}
