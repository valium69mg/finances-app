package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		"000016_future_expenses.up.sql", "000017_users_roles.up.sql", "000018_expense_requests.up.sql"} {
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
