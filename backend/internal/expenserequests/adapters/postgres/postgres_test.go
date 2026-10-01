package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	futuredomain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

const missingUser = "00000000-0000-0000-0000-000000000000"

// newRepo returns the repository on a throwaway schema holding a fresh copy of
// the migrations it depends on, so the tests never touch real data. It skips
// the test when TEST_DATABASE_URL is not set.
func newRepo(t *testing.T) (*postgres.Repo, *pgxpool.Pool) {
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
	schema := "requests_test_" + hex.EncodeToString(suffix)
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

	for _, name := range []string{"000002_users.up.sql", "000006_movements.up.sql", "000009_movements_transfer_id.up.sql",
		"000016_future_expenses.up.sql", "000017_users_roles.up.sql", "000018_expense_requests.up.sql", "000019_expense_request_reverts.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return postgres.NewRepo(pool), pool
}

func addUser(t *testing.T, pool *pgxpool.Pool, email, role string, active, verified bool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, verified, role, active) VALUES ($1, 'x', $2, $3, $4) RETURNING id::text`,
		email, verified, role, active).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

type fixture struct {
	repo           *postgres.Repo
	pool           *pgxpool.Pool
	owner, spouse  string
	otherHousehold string
}

func setup(t *testing.T) fixture {
	repo, pool := newRepo(t)
	return fixture{
		repo: repo, pool: pool,
		owner:          addUser(t, pool, "owner@example.com", "owner", true, true),
		spouse:         addUser(t, pool, "spouse@example.com", "household", true, true),
		otherHousehold: addUser(t, pool, "guest@example.com", "household", true, true),
	}
}

func (f fixture) create(t *testing.T, requester, amount, description string) domain.Request {
	t.Helper()
	r, err := f.repo.Create(context.Background(), requester, domain.Validated{
		Amount: d(amount), Description: description, SuggestedCategory: "Comida", Date: "2026-10-03",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return r
}

func expense(amount string) *ledger.Movement {
	m := ledger.Movement{
		Date: "2026-10-03", Description: "Tacos", Category: "Comida", Kind: ledger.KindExpense,
		PaymentMethod: "Débito", Currency: "MXN", Amount: d(amount), AmountMXN: d(amount),
	}
	return &m
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateAndGet(t *testing.T) {
	f := setup(t)
	r := f.create(t, f.spouse, "250.50", "Tacos")
	if r.ID == 0 || r.Status != domain.StatusPending || !r.Amount.Equal(d("250.50")) || r.RequesterEmail != "spouse@example.com" ||
		r.SuggestedCategory != "Comida" || r.ExpenseDate != "2026-10-03" || r.DecidedAt != nil || r.DecidedBy != "" || r.ResultKind != "" {
		t.Errorf("created = %+v", r)
	}
	got, err := f.repo.Get(context.Background(), r.ID)
	if err != nil || got.ID != r.ID {
		t.Errorf("Get = %+v, %v", got, err)
	}
	if _, err := f.repo.Get(context.Background(), 9999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get(unknown) = %v, want ErrNotFound", err)
	}
	noCategory, err := f.repo.Create(context.Background(), f.spouse, domain.Validated{Amount: d("1"), Description: "x", Date: "2026-10-03"})
	if err != nil || noCategory.SuggestedCategory != "" {
		t.Errorf("a request without a suggested category = %+v, %v", noCategory, err)
	}
}

func TestTableConstraints(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.create(t, f.spouse, "10", "x")
	bad := map[string]string{
		"zero amount":              `UPDATE expense_requests SET amount = 0 WHERE id = %d`,
		"blank description":        `UPDATE expense_requests SET description = '  ' WHERE id = %d`,
		"long description":         `UPDATE expense_requests SET description = repeat('a', 121) WHERE id = %d`,
		"unknown status":           `UPDATE expense_requests SET status = 'pendiente' WHERE id = %d`,
		"pending with a decider":   `UPDATE expense_requests SET decided_by = '` + f.owner + `' WHERE id = %d`,
		"pending with a comment":   `UPDATE expense_requests SET decision_comment = 'no' WHERE id = %d`,
		"rejected without comment": `UPDATE expense_requests SET status = 'rechazada', decided_by = '` + f.owner + `', decided_at = now() WHERE id = %d`,
		"rejected blank comment":   `UPDATE expense_requests SET status = 'rechazada', decided_by = '` + f.owner + `', decided_at = now(), decision_comment = ' ' WHERE id = %d`,
		"approved without kind":    `UPDATE expense_requests SET status = 'aprobada', decided_by = '` + f.owner + `', decided_at = now() WHERE id = %d`,
		"approved without decider": `UPDATE expense_requests SET status = 'aprobada', decided_at = now(), result_kind = 'gasto' WHERE id = %d`,
		"gasto with a future link": `UPDATE expense_requests SET status = 'aprobada', decided_by = '` + f.owner + `', decided_at = now(), result_kind = 'gasto', result_future_expense_id = 1 WHERE id = %d`,
		"cancelled with a decider": `UPDATE expense_requests SET status = 'cancelada', decided_by = '` + f.owner + `', decided_at = now() WHERE id = %d`,
		"unknown result kind":      `UPDATE expense_requests SET status = 'aprobada', decided_by = '` + f.owner + `', decided_at = now(), result_kind = 'otro' WHERE id = %d`,
	}
	for name, sql := range bad {
		if _, err := f.pool.Exec(ctx, fmt.Sprintf(sql, r.ID)); err == nil {
			t.Errorf("%s: the CHECK did not reject it", name)
		}
	}
}

func TestListScopesByRequesterAndStatusNewestFirst(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a := f.create(t, f.spouse, "1", "first")
	b := f.create(t, f.otherHousehold, "2", "second")
	c := f.create(t, f.spouse, "3", "third")
	if _, err := f.repo.Cancel(ctx, a.ID, f.spouse); err != nil {
		t.Fatal(err)
	}

	own, err := f.repo.List(ctx, app.ListFilter{RequesterID: f.spouse})
	if err != nil || len(own) != 2 || own[0].ID != c.ID || own[1].ID != a.ID {
		t.Errorf("own list = %+v, %v; want only the spouse's, newest first", own, err)
	}
	for _, r := range own {
		if r.RequesterID != f.spouse {
			t.Errorf("a request of %s leaked into the list of %s", r.RequesterID, f.spouse)
		}
	}
	all, err := f.repo.List(ctx, app.ListFilter{})
	if err != nil || len(all) != 3 || all[0].ID != c.ID || all[1].ID != b.ID {
		t.Errorf("full list = %+v, %v", all, err)
	}
	pending, err := f.repo.List(ctx, app.ListFilter{Status: domain.StatusPending})
	if err != nil || len(pending) != 2 {
		t.Errorf("pending list = %+v, %v", pending, err)
	}
	mine, err := f.repo.List(ctx, app.ListFilter{RequesterID: f.spouse, Status: domain.StatusCancelled})
	if err != nil || len(mine) != 1 || mine[0].ID != a.ID {
		t.Errorf("cancelled list = %+v, %v", mine, err)
	}
	if empty, err := f.repo.List(ctx, app.ListFilter{RequesterID: f.otherHousehold, Status: domain.StatusApproved}); err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty list = %#v, %v, want an empty non-nil slice", empty, err)
	}
}

func TestApproveAsExpenseIsAtomicAndAttributedToTheRequester(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.create(t, f.spouse, "250.50", "Tacos")

	got, err := f.repo.Approve(ctx, r.ID, f.owner, app.Approval{Destination: domain.DestinationExpense, Expense: expense("250.50")})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if got.Status != domain.StatusApproved || got.DecidedBy != f.owner || got.DecidedAt == nil || got.ResultKind != domain.DestinationExpense ||
		got.ResultMovementID == nil || got.ResultFutureExpenseID != nil {
		t.Fatalf("approved = %+v", got)
	}
	var createdBy, kind, category, amount string
	if err := f.pool.QueryRow(ctx, `SELECT created_by::text, kind, category, amount::text FROM movements WHERE id = $1`, *got.ResultMovementID).
		Scan(&createdBy, &kind, &category, &amount); err != nil {
		t.Fatalf("read the movement: %v", err)
	}
	if createdBy != f.spouse || kind != "Gasto" || category != "Comida" || amount != "250.5" {
		t.Errorf("movement created_by=%s kind=%s category=%s amount=%s; want the requester as author", createdBy, kind, category, amount)
	}
	if n := count(t, f.pool, "future_expenses"); n != 0 {
		t.Errorf("an expense approval created %d future expenses", n)
	}

	// Deleting the Gasto later keeps the request and unlinks it.
	if _, err := f.pool.Exec(ctx, `DELETE FROM movements WHERE id = $1`, *got.ResultMovementID); err != nil {
		t.Fatal(err)
	}
	after, err := f.repo.Get(ctx, r.ID)
	if err != nil || after.Status != domain.StatusApproved || after.ResultMovementID != nil {
		t.Errorf("after deleting the Gasto = %+v, %v", after, err)
	}
}

func TestApproveAsFutureExpenseCreatesTheItem(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.create(t, f.spouse, "1800", "Regalo")

	got, err := f.repo.Approve(ctx, r.ID, f.owner, app.Approval{
		Destination: domain.DestinationFuture,
		Future:      &futuredomain.Validated{Name: "Regalo", Target: d("1800"), DueDate: "2026-12-20"},
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if got.Status != domain.StatusApproved || got.ResultKind != domain.DestinationFuture || got.ResultFutureExpenseID == nil || got.ResultMovementID != nil {
		t.Fatalf("approved = %+v", got)
	}
	var name, target, due, status string
	if err := f.pool.QueryRow(ctx, `SELECT name, target_amount::text, due_date::text, status FROM future_expenses WHERE id = $1`, *got.ResultFutureExpenseID).
		Scan(&name, &target, &due, &status); err != nil {
		t.Fatalf("read the item: %v", err)
	}
	if name != "Regalo" || target != "1800.00" || due != "2026-12-20" || status != "active" {
		t.Errorf("item = %s %s %s %s", name, target, due, status)
	}
	if n := count(t, f.pool, "movements"); n != 0 {
		t.Errorf("a future expense approval created %d movements (it must not touch this cycle)", n)
	}
}

func TestApproveRollsBackWhenADownstreamStepFails(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	t.Run("the Gasto cannot be stored", func(t *testing.T) {
		r := f.create(t, f.spouse, "10", "bad date")
		m := expense("10")
		m.Date = "not-a-date"
		if _, err := f.repo.Approve(ctx, r.ID, f.owner, app.Approval{Destination: domain.DestinationExpense, Expense: m}); err == nil {
			t.Fatal("approve with a broken Gasto succeeded")
		}
		assertStillPending(t, f, r.ID)
	})
	t.Run("the status change fails after the Gasto was inserted", func(t *testing.T) {
		r := f.create(t, f.spouse, "10", "unknown decider")
		// The movement insert succeeds, then the UPDATE violates the decided_by
		// foreign key: the movement must disappear with the rollback.
		if _, err := f.repo.Approve(ctx, r.ID, missingUser, app.Approval{Destination: domain.DestinationExpense, Expense: expense("10")}); err == nil {
			t.Fatal("approve with an unknown decider succeeded")
		}
		assertStillPending(t, f, r.ID)
		if n := count(t, f.pool, "movements"); n != 0 {
			t.Errorf("%d movements left behind by a rolled back approval", n)
		}
	})
	t.Run("the future expense cannot be stored", func(t *testing.T) {
		r := f.create(t, f.spouse, "10", "bad due date")
		_, err := f.repo.Approve(ctx, r.ID, f.owner, app.Approval{
			Destination: domain.DestinationFuture,
			Future:      &futuredomain.Validated{Name: "x", Target: d("10"), DueDate: "nope"},
		})
		if err == nil {
			t.Fatal("approve with a broken item succeeded")
		}
		assertStillPending(t, f, r.ID)
	})
	t.Run("the status change fails after the item was inserted", func(t *testing.T) {
		r := f.create(t, f.spouse, "10", "unknown decider again")
		if _, err := f.repo.Approve(ctx, r.ID, missingUser, app.Approval{
			Destination: domain.DestinationFuture,
			Future:      &futuredomain.Validated{Name: "x", Target: d("10"), DueDate: "2026-12-01"},
		}); err == nil {
			t.Fatal("approve with an unknown decider succeeded")
		}
		assertStillPending(t, f, r.ID)
		if n := count(t, f.pool, "future_expenses"); n != 0 {
			t.Errorf("%d future expenses left behind by a rolled back approval", n)
		}
	})
}

func assertStillPending(t *testing.T, f fixture, id int) {
	t.Helper()
	got, err := f.repo.Get(context.Background(), id)
	if err != nil || got.Status != domain.StatusPending || got.DecidedBy != "" || got.ResultKind != "" {
		t.Errorf("request after a failed approval = %+v, %v; want it untouched", got, err)
	}
}

func TestDecidedRequestsAreImmutable(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	approve := func(id int) error {
		_, err := f.repo.Approve(ctx, id, f.owner, app.Approval{Destination: domain.DestinationExpense, Expense: expense("10")})
		return err
	}

	// Approving twice is a conflict and registers one Gasto only.
	r := f.create(t, f.spouse, "10", "twice")
	if err := approve(r.ID); err != nil {
		t.Fatal(err)
	}
	if err := approve(r.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("second approve = %v, want ErrInvalidState", err)
	}
	if n := count(t, f.pool, "movements"); n != 1 {
		t.Errorf("%d movements after approving twice, want 1", n)
	}
	if _, err := f.repo.Reject(ctx, r.ID, f.owner, "no"); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("reject after approve = %v", err)
	}
	if _, err := f.repo.Cancel(ctx, r.ID, f.spouse); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("cancel after approve = %v", err)
	}

	// Rejected, then everything else conflicts.
	rej := f.create(t, f.spouse, "10", "rejected")
	got, err := f.repo.Reject(ctx, rej.ID, f.owner, "No este mes")
	if err != nil || got.Status != domain.StatusRejected || got.DecisionComment != "No este mes" || got.DecidedBy != f.owner || got.DecidedAt == nil {
		t.Fatalf("Reject = %+v, %v", got, err)
	}
	if err := approve(rej.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("approve after reject = %v", err)
	}
	if _, err := f.repo.Reject(ctx, rej.ID, f.owner, "otra vez"); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("reject twice = %v", err)
	}
	if _, err := f.repo.Cancel(ctx, rej.ID, f.spouse); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("cancel after reject = %v", err)
	}

	// Cancelled, then approve and reject conflict.
	can := f.create(t, f.spouse, "10", "cancelled")
	got, err = f.repo.Cancel(ctx, can.ID, f.spouse)
	if err != nil || got.Status != domain.StatusCancelled || got.DecidedAt == nil || got.DecidedBy != "" {
		t.Fatalf("Cancel = %+v, %v", got, err)
	}
	if err := approve(can.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("approve after cancel = %v", err)
	}
	if _, err := f.repo.Reject(ctx, can.ID, f.owner, "x"); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("reject after cancel = %v", err)
	}
	if _, err := f.repo.Cancel(ctx, can.ID, f.spouse); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("cancel twice = %v", err)
	}

	for _, id := range []int{9999} {
		if err := approve(id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("approve(unknown) = %v", err)
		}
		if _, err := f.repo.Reject(ctx, id, f.owner, "x"); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("reject(unknown) = %v", err)
		}
		if _, err := f.repo.Cancel(ctx, id, f.spouse); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("cancel(unknown) = %v", err)
		}
	}
}

func TestCancelIsOnlyForTheRequester(t *testing.T) {
	f := setup(t)
	r := f.create(t, f.spouse, "10", "mine")
	if _, err := f.repo.Cancel(context.Background(), r.ID, f.otherHousehold); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("cancel by someone else = %v, want ErrForbidden", err)
	}
	assertStillPending(t, f, r.ID)
}

func TestConcurrentApprovalsRegisterOneGasto(t *testing.T) {
	f := setup(t)
	r := f.create(t, f.spouse, "10", "race")
	const workers = 5
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := f.repo.Approve(context.Background(), r.ID, f.owner, app.Approval{Destination: domain.DestinationExpense, Expense: expense("10")})
			errs <- err
		}()
	}
	ok, conflicts := 0, 0
	for i := 0; i < workers; i++ {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrInvalidState):
			conflicts++
		default:
			t.Errorf("unexpected error %v", err)
		}
	}
	if ok != 1 || conflicts != workers-1 || count(t, f.pool, "movements") != 1 {
		t.Errorf("ok=%d conflicts=%d movements=%d, want exactly one winner", ok, conflicts, count(t, f.pool, "movements"))
	}
}

func TestOwnerEmailsAreActiveVerifiedOwnersOnly(t *testing.T) {
	f := setup(t)
	addUser(t, f.pool, "second-owner@example.com", "owner", true, true)
	addUser(t, f.pool, "inactive-owner@example.com", "owner", false, true)
	addUser(t, f.pool, "unverified-owner@example.com", "owner", true, false)
	got, err := f.repo.OwnerEmails(context.Background())
	if err != nil || len(got) != 2 || got[0] != "owner@example.com" || got[1] != "second-owner@example.com" {
		t.Errorf("OwnerEmails = %v, %v", got, err)
	}
}

// --- revert ---------------------------------------------------------------

func (f fixture) approveExpense(t *testing.T, amount string) domain.Request {
	t.Helper()
	r := f.create(t, f.spouse, amount, "Tacos")
	got, err := f.repo.Approve(context.Background(), r.ID, f.owner, app.Approval{Destination: domain.DestinationExpense, Expense: expense(amount)})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	return got
}

func (f fixture) approveFuture(t *testing.T, amount string) domain.Request {
	t.Helper()
	r := f.create(t, f.spouse, amount, "Regalo")
	got, err := f.repo.Approve(context.Background(), r.ID, f.owner, app.Approval{
		Destination: domain.DestinationFuture,
		Future:      &futuredomain.Validated{Name: "Regalo", Target: d(amount), DueDate: "2026-12-20"},
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	return got
}

// assertPendingAgain checks the request is a clean solicitada one (the CHECK
// would reject anything else) with the given audit count.
func assertPendingAgain(t *testing.T, got domain.Request, reverts int) {
	t.Helper()
	if got.Status != domain.StatusPending || got.DecidedBy != "" || got.DecidedAt != nil || got.DecisionComment != "" || got.ResultKind != "" ||
		got.ResultMovementID != nil || got.ResultFutureExpenseID != nil || got.RevertCount != reverts || got.RevertedAt == nil {
		t.Errorf("reverted request = %+v, want solicitada with %d revert(s)", got, reverts)
	}
}

func TestRevertOfAGastoApprovalDeletesTheMovementAndResetsTheRequest(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	approved := f.approveExpense(t, "250.50")
	// A neighbour Gasto must survive.
	other := f.approveExpense(t, "10")

	got, err := f.repo.Revert(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}
	assertPendingAgain(t, got, 1)
	if got.UpdatedAt.Before(approved.UpdatedAt) || !got.UpdatedAt.After(approved.CreatedAt) {
		t.Errorf("updated_at = %v, want refreshed (approval %v)", got.UpdatedAt, approved.UpdatedAt)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM movements WHERE id = $1`, *approved.ResultMovementID).Scan(&n); err != nil || n != 0 {
		t.Errorf("the reverted Gasto still exists (count %d, %v)", n, err)
	}
	if count(t, f.pool, "movements") != 1 {
		t.Errorf("movements = %d, want only the other request's Gasto", count(t, f.pool, "movements"))
	}
	if still, err := f.repo.Get(ctx, other.ID); err != nil || still.Status != domain.StatusApproved || still.ResultMovementID == nil {
		t.Errorf("the other request = %+v, %v", still, err)
	}

	// Second revert conflicts and changes nothing.
	if _, err := f.repo.Revert(ctx, approved.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("double revert = %v, want ErrInvalidState", err)
	}
	if again, _ := f.repo.Get(ctx, approved.ID); again.RevertCount != 1 {
		t.Errorf("revert_count = %d after a refused revert", again.RevertCount)
	}

	// It is a normal pending request: approve, revert again, the audit counts both.
	if _, err := f.repo.Approve(ctx, approved.ID, f.owner, app.Approval{Destination: domain.DestinationExpense, Expense: expense("250.50")}); err != nil {
		t.Fatalf("approve after revert: %v", err)
	}
	again, err := f.repo.Revert(ctx, approved.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertPendingAgain(t, again, 2)
}

func TestRevertOnlyAppliesToApprovedRequests(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	pending := f.create(t, f.spouse, "10", "pending")
	rejected := f.create(t, f.spouse, "10", "rejected")
	if _, err := f.repo.Reject(ctx, rejected.ID, f.owner, "No"); err != nil {
		t.Fatal(err)
	}
	cancelled := f.create(t, f.spouse, "10", "cancelled")
	if _, err := f.repo.Cancel(ctx, cancelled.ID, f.spouse); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]int{"pending": pending.ID, "rejected": rejected.ID, "cancelled": cancelled.ID} {
		if _, err := f.repo.Revert(ctx, id); !errors.Is(err, domain.ErrInvalidState) {
			t.Errorf("revert of a %s request = %v, want ErrInvalidState", name, err)
		}
	}
	if got, _ := f.repo.Get(ctx, rejected.ID); got.Status != domain.StatusRejected || got.DecisionComment != "No" || got.RevertCount != 0 {
		t.Errorf("a refused revert changed the rejected request: %+v", got)
	}
	if _, err := f.repo.Revert(ctx, 9999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("revert(unknown) = %v, want ErrNotFound", err)
	}
}

func TestRevertOfAFutureExpenseApprovalDeletesTheItemAndFreesItsSavings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	approved := f.approveFuture(t, "1800")
	itemID := *approved.ResultFutureExpenseID
	// Two savings assigned to the item and one that was already free.
	for _, amount := range []string{"300", "200"} {
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO movements (date, description, category, kind, payment_method, currency, amount, amount_mxn, future_expense_id)
			VALUES ('2026-10-04', 'Ahorro regalo', 'Gastos futuros', 'Ahorro', 'Débito', 'MXN', $1::numeric, $1::numeric, $2)`, amount, int64(itemID)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO movements (date, description, category, kind, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-10-04', 'Libre', 'Gastos futuros', 'Ahorro', 'Débito', 'MXN', 50, 50)`); err != nil {
		t.Fatal(err)
	}

	got, err := f.repo.Revert(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}
	assertPendingAgain(t, got, 1)
	if n := count(t, f.pool, "future_expenses"); n != 0 {
		t.Errorf("%d future expenses left after the revert", n)
	}
	var total string
	var linked int
	if err := f.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_mxn), 0)::text, count(*) FILTER (WHERE future_expense_id IS NOT NULL)
		FROM movements WHERE kind = 'Ahorro' AND category = 'Gastos futuros' AND future_expense_id IS NULL`).Scan(&total, &linked); err != nil {
		t.Fatal(err)
	}
	if total != "550" || linked != 0 {
		t.Errorf("free balance = %s (linked %d), want the two savings back plus the free one = 550", total, linked)
	}
	if n := count(t, f.pool, "movements"); n != 3 {
		t.Errorf("movements = %d, want the 3 savings kept (never deleted)", n)
	}
}

func TestRevertIsRefusedForAPaidFutureExpenseAndChangesNothing(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	approved := f.approveFuture(t, "1800")
	itemID := *approved.ResultFutureExpenseID
	var expenseID int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO movements (date, description, category, kind, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-12-20', 'Regalo', 'Comida', 'Gasto', 'Débito', 'MXN', 1800, 1800) RETURNING id`).Scan(&expenseID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `
		UPDATE future_expenses SET status = 'paid', paid_at = '2026-12-20', amount_paid = 1800, expense_movement_id = $2 WHERE id = $1`,
		int64(itemID), expenseID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.repo.Revert(ctx, approved.ID); !errors.Is(err, domain.ErrFutureExpensePaid) {
		t.Fatalf("Revert = %v, want ErrFutureExpensePaid", err)
	}
	got, err := f.repo.Get(ctx, approved.ID)
	if err != nil || got.Status != domain.StatusApproved || got.ResultFutureExpenseID == nil || *got.ResultFutureExpenseID != itemID ||
		got.RevertCount != 0 || got.RevertedAt != nil || got.DecidedBy != f.owner {
		t.Errorf("request after the refusal = %+v, %v; want it untouched", got, err)
	}
	if count(t, f.pool, "future_expenses") != 1 || count(t, f.pool, "movements") != 1 {
		t.Errorf("rows changed by a refused revert: items %d, movements %d", count(t, f.pool, "future_expenses"), count(t, f.pool, "movements"))
	}
}

func TestRevertRollsBackWhenTheResetFails(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	approved := f.approveExpense(t, "10")
	// A trigger-free way to make the final UPDATE fail after the movement was
	// deleted: a revert_count ceiling the reset would exceed.
	if _, err := f.pool.Exec(ctx, `ALTER TABLE expense_requests ADD CONSTRAINT revert_cap CHECK (revert_count < 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Revert(ctx, approved.ID); err == nil {
		t.Fatal("revert with a failing reset succeeded")
	}
	got, err := f.repo.Get(ctx, approved.ID)
	if err != nil || got.Status != domain.StatusApproved || got.ResultMovementID == nil || count(t, f.pool, "movements") != 1 {
		t.Errorf("after the failed revert: %+v, %v, movements %d; want the Gasto restored by the rollback", got, err, count(t, f.pool, "movements"))
	}
}

func TestRevertIsIdempotentWhenTheLinkedRowWasAlreadyDeleted(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	t.Run("the Gasto was deleted by hand", func(t *testing.T) {
		approved := f.approveExpense(t, "10")
		if _, err := f.pool.Exec(ctx, `DELETE FROM movements WHERE id = $1`, *approved.ResultMovementID); err != nil {
			t.Fatal(err)
		}
		got, err := f.repo.Revert(ctx, approved.ID)
		if err != nil {
			t.Fatalf("Revert: %v", err)
		}
		assertPendingAgain(t, got, 1)
	})
	t.Run("the future expense was deleted by hand", func(t *testing.T) {
		approved := f.approveFuture(t, "10")
		if _, err := f.pool.Exec(ctx, `DELETE FROM future_expenses WHERE id = $1`, *approved.ResultFutureExpenseID); err != nil {
			t.Fatal(err)
		}
		got, err := f.repo.Revert(ctx, approved.ID)
		if err != nil {
			t.Fatalf("Revert: %v", err)
		}
		assertPendingAgain(t, got, 1)
	})
}

func TestRevertNeverDeletesAMovementOfAnotherKind(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	approved := f.approveExpense(t, "10")
	// The link was repointed (by hand) to an Ahorro: the Gasto-only delete skips it.
	var savingID int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO movements (date, description, category, kind, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-10-04', 'Ahorro', 'Gastos futuros', 'Ahorro', 'Débito', 'MXN', 5, 5) RETURNING id`).Scan(&savingID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE expense_requests SET result_movement_id = $2 WHERE id = $1`, int64(approved.ID), savingID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Revert(ctx, approved.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM movements WHERE id = $1`, savingID).Scan(&n); err != nil || n != 1 {
		t.Errorf("the Ahorro was deleted by a revert (count %d, %v)", n, err)
	}
}

func TestConcurrentRevertsHaveOneWinner(t *testing.T) {
	f := setup(t)
	approved := f.approveExpense(t, "10")
	const workers = 5
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := f.repo.Revert(context.Background(), approved.ID)
			errs <- err
		}()
	}
	ok, conflicts := 0, 0
	for i := 0; i < workers; i++ {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrInvalidState):
			conflicts++
		default:
			t.Errorf("unexpected error %v", err)
		}
	}
	got, _ := f.repo.Get(context.Background(), approved.ID)
	if ok != 1 || conflicts != workers-1 || got.RevertCount != 1 || count(t, f.pool, "movements") != 0 {
		t.Errorf("ok=%d conflicts=%d revert_count=%d movements=%d, want exactly one winner", ok, conflicts, got.RevertCount, count(t, f.pool, "movements"))
	}
}

// Reverts and approvals race on the same request. Whatever the interleaving,
// every success is one legal transition, and the final state is consistent:
// aprobada has exactly one live Gasto linked, solicitada has none.
func TestConcurrentRevertAndApproveStayConsistent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	approved := f.approveExpense(t, "10")
	const workers = 6
	var (
		wg                 sync.WaitGroup
		mu                 sync.Mutex
		reverts, approvals int
		unexpected         []error
	)
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := f.repo.Revert(ctx, approved.ID)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				reverts++
			case !errors.Is(err, domain.ErrInvalidState):
				unexpected = append(unexpected, err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := f.repo.Approve(ctx, approved.ID, f.owner, app.Approval{Destination: domain.DestinationExpense, Expense: expense("10")})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				approvals++
			case !errors.Is(err, domain.ErrInvalidState):
				unexpected = append(unexpected, err)
			}
		}()
	}
	wg.Wait()
	if len(unexpected) > 0 {
		t.Fatalf("unexpected errors: %v", unexpected)
	}
	got, err := f.repo.Get(ctx, approved.ID)
	if err != nil {
		t.Fatal(err)
	}
	live := count(t, f.pool, "movements")
	switch got.Status {
	case domain.StatusApproved:
		if live != 1 || got.ResultMovementID == nil || approvals != reverts {
			t.Errorf("approved: live Gastos %d, link %v, approvals %d, reverts %d", live, got.ResultMovementID, approvals, reverts)
		}
	case domain.StatusPending:
		if live != 0 || approvals+1 != reverts {
			t.Errorf("pending: live Gastos %d, approvals %d, reverts %d", live, approvals, reverts)
		}
	default:
		t.Errorf("final status %s", got.Status)
	}
	if got.RevertCount != reverts {
		t.Errorf("revert_count = %d, successful reverts = %d", got.RevertCount, reverts)
	}
}

func TestRevertAuditConstraint(t *testing.T) {
	f := setup(t)
	r := f.create(t, f.spouse, "10", "x")
	for name, sql := range map[string]string{
		"count without a time": `UPDATE expense_requests SET revert_count = 1 WHERE id = %d`,
		"time without a count": `UPDATE expense_requests SET reverted_at = now() WHERE id = %d`,
		"negative count":       `UPDATE expense_requests SET revert_count = -1, reverted_at = now() WHERE id = %d`,
	} {
		if _, err := f.pool.Exec(context.Background(), fmt.Sprintf(sql, r.ID)); err == nil {
			t.Errorf("%s: the CHECK did not reject it", name)
		}
	}
}
