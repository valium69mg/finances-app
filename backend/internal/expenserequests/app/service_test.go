package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	futuredomain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

var (
	owner     = session.Identity{UserID: "u-owner", Role: session.RoleOwner}
	spouse    = session.Identity{UserID: "u-spouse", Role: session.RoleHousehold}
	otherHome = session.Identity{UserID: "u-guest", Role: session.RoleHousehold}
)

// --- fakes ---------------------------------------------------------------

type fakeRepo struct {
	mu       sync.Mutex
	rows     map[int]domain.Request
	nextID   int
	lastList app.ListFilter
	approval app.Approval
	owners   []string
	// revertErr makes the next Revert of an approved request fail.
	revertErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[int]domain.Request{}, nextID: 1, owners: []string{"owner@example.com"}}
}

func (f *fakeRepo) Create(_ context.Context, requesterID string, v domain.Validated) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := domain.Request{
		ID: f.nextID, RequesterID: requesterID, RequesterEmail: requesterID + "@example.com", Amount: v.Amount,
		Description: v.Description, SuggestedCategory: v.SuggestedCategory, ExpenseDate: v.Date, Status: domain.StatusPending,
	}
	f.nextID++
	f.rows[r.ID] = r
	return r, nil
}
func (f *fakeRepo) Get(_ context.Context, id int) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return domain.Request{}, domain.ErrNotFound
	}
	return r, nil
}
func (f *fakeRepo) List(_ context.Context, fl app.ListFilter) ([]domain.Request, error) {
	f.lastList = fl
	return nil, nil
}
func (f *fakeRepo) decide(id int, from func(r *domain.Request) error) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return domain.Request{}, domain.ErrNotFound
	}
	if err := from(&r); err != nil {
		return domain.Request{}, err
	}
	f.rows[id] = r
	return r, nil
}
func (f *fakeRepo) Cancel(_ context.Context, id int, requesterID string) (domain.Request, error) {
	return f.decide(id, func(r *domain.Request) error {
		if r.RequesterID != requesterID {
			return domain.ErrForbidden
		}
		if r.Status != domain.StatusPending {
			return domain.ErrInvalidState
		}
		r.Status = domain.StatusCancelled
		return nil
	})
}
func (f *fakeRepo) Reject(_ context.Context, id int, by, comment string) (domain.Request, error) {
	return f.decide(id, func(r *domain.Request) error {
		if r.Status != domain.StatusPending {
			return domain.ErrInvalidState
		}
		r.Status, r.DecidedBy, r.DecisionComment = domain.StatusRejected, by, comment
		return nil
	})
}
func (f *fakeRepo) Approve(_ context.Context, id int, by string, a app.Approval) (domain.Request, error) {
	return f.decide(id, func(r *domain.Request) error {
		if r.Status != domain.StatusPending {
			return domain.ErrInvalidState
		}
		f.approval = a
		r.Status, r.DecidedBy, r.ResultKind = domain.StatusApproved, by, a.Destination
		return nil
	})
}
func (f *fakeRepo) Revert(_ context.Context, id int) (domain.Request, error) {
	return f.decide(id, func(r *domain.Request) error {
		if r.Status != domain.StatusApproved {
			return domain.ErrInvalidState
		}
		if f.revertErr != nil {
			return f.revertErr
		}
		now := time.Now()
		r.Status, r.DecidedBy, r.ResultKind = domain.StatusPending, "", ""
		r.RevertCount, r.RevertedAt = r.RevertCount+1, &now
		return nil
	})
}
func (f *fakeRepo) OwnerEmails(context.Context) ([]string, error) { return f.owners, nil }

type fakeExpenses struct {
	built       []expensesapp.Input
	budgetCalls []string
	budget      *decimal.Decimal
	spent       decimal.Decimal
	budgetErr   error
}

func (f *fakeExpenses) Build(_ context.Context, in expensesapp.Input) (ledger.Movement, error) {
	f.built = append(f.built, in)
	if in.PaymentMethod == "Bitcoin" {
		return ledger.Movement{}, errors.New("invalid: payment method")
	}
	date, method := in.Date, in.PaymentMethod
	if method == "" {
		method = "Débito"
	}
	return ledger.Movement{Date: date, Description: in.Description, Category: in.Category, Kind: ledger.KindExpense,
		PaymentMethod: method, Currency: "MXN", Amount: in.Amount, AmountMXN: in.Amount}, nil
}
func (f *fakeExpenses) BudgetFor(_ context.Context, category, date string) (expensesapp.BudgetFeedback, error) {
	f.budgetCalls = append(f.budgetCalls, category+"@"+date)
	if f.budgetErr != nil {
		return expensesapp.BudgetFeedback{}, f.budgetErr
	}
	fb := expensesapp.BudgetFeedback{Month: date[:7], Category: category, Budget: f.budget, Spent: f.spent}
	if f.budget != nil {
		rem := f.budget.Sub(f.spent)
		fb.Remaining = &rem
	}
	return fb, nil
}

type fakeSettings struct{ cfg settings.Config }

func (f fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, nil }

type sentMail struct{ to, subject, html, text string }

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	err  error
}

func (f *fakeMailer) Send(_ context.Context, to, subject, html, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{to, subject, html, text})
	return f.err
}

type countingLimiter struct{ calls map[string]int }

func (l *countingLimiter) Allow(key string, limit int, _ time.Duration) bool {
	l.calls[key]++
	return l.calls[key] <= limit
}

type env struct {
	svc      *app.Service
	repo     *fakeRepo
	expenses *fakeExpenses
	mailer   *fakeMailer
}

func newEnv(t *testing.T) env {
	t.Helper()
	e := env{repo: newFakeRepo(), expenses: &fakeExpenses{}, mailer: &fakeMailer{}}
	e.svc = app.NewService(app.Deps{
		Repo: e.repo, Expenses: e.expenses, Settings: fakeSettings{cfg: settingstest.RealConfig()}, Mailer: e.mailer,
		Limiter: &countingLimiter{calls: map[string]int{}}, AppBaseURL: "https://app.example.com/",
		Now:    func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC) },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return e
}

func (e env) request(t *testing.T, amount string) domain.Request {
	t.Helper()
	r, err := e.svc.Create(context.Background(), spouse, domain.Input{Amount: d(amount), Description: "Tacos", SuggestedCategory: "Comida fuera"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	e.mailer.sent = nil
	return r
}

// --- create ---------------------------------------------------------------

func TestCreateDefaultsTheDateAndNotifiesTheOwners(t *testing.T) {
	e := newEnv(t)
	e.repo.owners = []string{"owner@example.com", "second@example.com"}
	r, err := e.svc.Create(context.Background(), spouse, domain.Input{Amount: d("250.50"), Description: " Tacos ", SuggestedCategory: "Comida fuera"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.RequesterID != spouse.UserID || r.ExpenseDate != "2026-10-15" || r.Description != "Tacos" || r.Status != domain.StatusPending {
		t.Errorf("created = %+v", r)
	}
	if len(e.mailer.sent) != 2 || e.mailer.sent[0].to != "owner@example.com" || e.mailer.sent[1].to != "second@example.com" {
		t.Fatalf("emails = %+v, want one per owner", e.mailer.sent)
	}
	m := e.mailer.sent[0]
	if m.subject != "Nueva petición de gasto" || !strings.Contains(m.text, "$250.50 MXN") || !strings.Contains(m.text, "Tacos") ||
		!strings.Contains(m.text, "https://app.example.com/peticiones") {
		t.Errorf("owner email = %+v", m)
	}
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	bad := []domain.Input{
		{Amount: d("0"), Description: "x"},
		{Amount: d("-5"), Description: "x"},
		{Amount: d("1.999"), Description: "x"},
		{Amount: d("1e999999999"), Description: "x"},
		{Amount: d("5"), Description: ""},
		{Amount: d("5"), Description: strings.Repeat("a", 121)},
		{Amount: d("5"), Description: "x", SuggestedCategory: "Sueldo"},              // an Ingreso category
		{Amount: d("5"), Description: "x", SuggestedCategory: "Fondo de emergencia"}, // an Ahorro category
		{Amount: d("5"), Description: "x", SuggestedCategory: "mandado"},             // wrong case
		{Amount: d("5"), Description: "x", Date: "15/10/2026"},
	}
	for i, in := range bad {
		if _, err := e.svc.Create(ctx, spouse, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("case %d: err = %v, want ErrInvalidInput", i, err)
		}
	}
	if len(e.repo.rows) != 0 || len(e.mailer.sent) != 0 {
		t.Errorf("an invalid request stored %d rows and sent %d emails", len(e.repo.rows), len(e.mailer.sent))
	}
	if _, err := e.svc.Create(ctx, session.Identity{}, domain.Input{Amount: d("5"), Description: "x"}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("anonymous create = %v, want ErrForbidden", err)
	}
}

func TestCreateIsRateLimitedPerUserAndOnlyCountsValidRequests(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 0; i < 30; i++ { // invalid requests never use the budget
		_, _ = e.svc.Create(ctx, spouse, domain.Input{Amount: d("0"), Description: "x"})
	}
	for i := 0; i < app.CreateHourlyLimit; i++ {
		if _, err := e.svc.Create(ctx, spouse, domain.Input{Amount: d("5"), Description: "x"}); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	if _, err := e.svc.Create(ctx, spouse, domain.Input{Amount: d("5"), Description: "x"}); !errors.Is(err, domain.ErrRateLimited) {
		t.Errorf("request over the limit = %v, want ErrRateLimited", err)
	}
	if _, err := e.svc.Create(ctx, otherHome, domain.Input{Amount: d("5"), Description: "x"}); err != nil {
		t.Errorf("another user shares the budget: %v", err)
	}
}

func TestEmailFailuresNeverFailTheCall(t *testing.T) {
	e := newEnv(t)
	e.mailer.err = errors.New("resend is down")
	r, err := e.svc.Create(context.Background(), spouse, domain.Input{Amount: d("5"), Description: "x"})
	if err != nil || r.ID == 0 {
		t.Fatalf("Create with a failing mailer = %+v, %v", r, err)
	}
	if _, err := e.svc.Reject(context.Background(), owner, r.ID, "no"); err != nil {
		t.Errorf("Reject with a failing mailer = %v", err)
	}
	if len(e.mailer.sent) != 2 {
		t.Errorf("emails attempted = %d, want 2", len(e.mailer.sent))
	}
}

// --- list, categories, cancel ----------------------------------------------

func TestListScopesHouseholdToTheirOwnRequests(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.List(ctx, spouse, ""); err != nil || e.repo.lastList.RequesterID != spouse.UserID {
		t.Errorf("household filter = %+v, %v; want her own id", e.repo.lastList, err)
	}
	// A household user cannot widen the scope by asking for a status.
	if _, err := e.svc.List(ctx, spouse, "aprobada"); err != nil || e.repo.lastList.RequesterID != spouse.UserID || e.repo.lastList.Status != domain.StatusApproved {
		t.Errorf("household filter with status = %+v, %v", e.repo.lastList, err)
	}
	if _, err := e.svc.List(ctx, owner, "solicitada"); err != nil || e.repo.lastList.RequesterID != "" || e.repo.lastList.Status != domain.StatusPending {
		t.Errorf("owner filter = %+v, %v; want everyone's pending", e.repo.lastList, err)
	}
	if _, err := e.svc.List(ctx, owner, "bogus"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("unknown status = %v", err)
	}
	if _, err := e.svc.List(ctx, session.Identity{UserID: "x", Role: "intruder"}, ""); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("unknown role = %v", err)
	}
}

func TestCategoriesAreOnlyTheGastoNames(t *testing.T) {
	e := newEnv(t)
	got, err := e.svc.Categories(context.Background(), spouse)
	if err != nil {
		t.Fatal(err)
	}
	has := func(n string) bool {
		for _, g := range got {
			if g == n {
				return true
			}
		}
		return false
	}
	if !has("Mandado") || !has("Comida fuera") || has("Sueldo") || has("Fondo de emergencia") || has("Gastos futuros") {
		t.Errorf("categories = %v, want Gasto names only", got)
	}
}

func TestCancelIsOnlyForTheRequesterWhilePending(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "50")
	if _, err := e.svc.Cancel(ctx, otherHome, r.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("cancel by another household user = %v", err)
	}
	if _, err := e.svc.Cancel(ctx, owner, r.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("cancel by the owner = %v, want ErrForbidden (only the requester cancels)", err)
	}
	got, err := e.svc.Cancel(ctx, spouse, r.ID)
	if err != nil || got.Status != domain.StatusCancelled {
		t.Fatalf("cancel = %+v, %v", got, err)
	}
	if _, err := e.svc.Cancel(ctx, spouse, r.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("cancel twice = %v", err)
	}
	if _, err := e.svc.Cancel(ctx, spouse, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cancel unknown = %v", err)
	}
}

// --- budget check ------------------------------------------------------------

func TestBudgetCheckNumbers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	budget := d("1000")
	e.expenses.budget = &budget

	cases := []struct {
		name, spent string
		fits        bool
		projected   string
		overBy      string
	}{
		{"fits", "500", true, "200", "0"},
		{"exactly fits", "700", true, "0", "0"},
		{"exceeds", "800", false, "-100", "100"},
	}
	for _, c := range cases {
		e.expenses.spent = d(c.spent)
		got, err := e.svc.BudgetCheck(ctx, owner, r.ID, "Transporte", "")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Fits == nil || *got.Fits != c.fits || !got.ProjectedRemaining.Equal(d(c.projected)) || !got.OverBy.Equal(d(c.overBy)) ||
			!got.Budget.Equal(d("1000")) || !got.Spent.Equal(d(c.spent)) || !got.ProjectedSpent.Equal(d(c.spent).Add(d("300"))) || !got.Amount.Equal(d("300")) {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
	// The date defaults to the date of the request.
	if last := e.expenses.budgetCalls[len(e.expenses.budgetCalls)-1]; last != "Transporte@2026-10-15" {
		t.Errorf("budget read for %q, want the request date", last)
	}
	if _, err := e.svc.BudgetCheck(ctx, owner, r.ID, "Transporte", "2026-11-20"); err != nil ||
		e.expenses.budgetCalls[len(e.expenses.budgetCalls)-1] != "Transporte@2026-11-20" {
		t.Errorf("explicit date: calls %v, %v", e.expenses.budgetCalls, err)
	}

	e.expenses.budget = nil
	got, err := e.svc.BudgetCheck(ctx, owner, r.ID, "Ocio", "")
	if err != nil || got.Budget != nil || got.Fits != nil || got.Remaining != nil || got.ProjectedRemaining != nil || !got.OverBy.IsZero() {
		t.Errorf("no budget = %+v, %v; want nil budget and nil fits", got, err)
	}
}

func TestBudgetCheckRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	if _, err := e.svc.BudgetCheck(ctx, spouse, r.ID, "Transporte", ""); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("household budget check = %v, want ErrForbidden", err)
	}
	for _, category := range []string{"", "Sueldo", "transporte", "Nope"} {
		if _, err := e.svc.BudgetCheck(ctx, owner, r.ID, category, ""); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("category %q = %v, want ErrInvalidInput", category, err)
		}
	}
	if _, err := e.svc.BudgetCheck(ctx, owner, r.ID, "Transporte", "yesterday"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad date = %v", err)
	}
	if _, err := e.svc.BudgetCheck(ctx, owner, 999, "Transporte", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown request = %v", err)
	}
}

// --- approve -------------------------------------------------------------------

func TestApproveAsExpenseUsesTheRequestAndStillApprovesWhatDoesNotFit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	budget := d("1000")
	e.expenses.budget, e.expenses.spent = &budget, d("900") // over the budget once the Gasto is registered

	res, err := e.svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationExpense, Category: "Transporte"})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if res.Request.Status != domain.StatusApproved || res.Request.DecidedBy != owner.UserID {
		t.Errorf("request = %+v", res.Request)
	}
	in := e.expenses.built[0]
	if !in.Amount.Equal(d("300")) || in.Description != "Tacos" || in.Category != "Transporte" || in.Date != "2026-10-15" || in.PaymentMethod != "" {
		t.Errorf("the Gasto was built from %+v", in)
	}
	if e.repo.approval.Expense == nil || e.repo.approval.Future != nil || e.repo.approval.Expense.Category != "Transporte" {
		t.Errorf("approval = %+v", e.repo.approval)
	}
	if res.Feedback == nil || res.Feedback.Category != "Transporte" || !res.Feedback.Spent.Equal(d("900")) {
		t.Errorf("feedback = %+v, want the expenses feedback echoed", res.Feedback)
	}
	if len(e.mailer.sent) != 1 || e.mailer.sent[0].to != "u-spouse@example.com" || e.mailer.sent[0].subject != "Tu petición de gasto fue aprobada" ||
		!strings.Contains(e.mailer.sent[0].text, "se registró como gasto") {
		t.Errorf("requester email = %+v", e.mailer.sent)
	}
}

func TestApproveAsExpenseWithExplicitDateAndPaymentMethod(t *testing.T) {
	e := newEnv(t)
	r := e.request(t, "300")
	_, err := e.svc.Approve(context.Background(), owner, r.ID, app.ApproveInput{
		Destination: domain.DestinationExpense, Category: "Mandado", Date: "2026-10-20", PaymentMethod: "Efectivo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if in := e.expenses.built[0]; in.Date != "2026-10-20" || in.PaymentMethod != "Efectivo" {
		t.Errorf("built from %+v", in)
	}
}

func TestApproveAsFutureExpenseNeverRunsTheBudgetCheck(t *testing.T) {
	e := newEnv(t)
	r := e.request(t, "1800")
	res, err := e.svc.Approve(context.Background(), owner, r.ID, app.ApproveInput{Destination: domain.DestinationFuture, DueDate: "2026-12-20"})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	f := e.repo.approval.Future
	if f == nil || e.repo.approval.Expense != nil || f.Name != "Tacos" || !f.Target.Equal(d("1800")) || f.DueDate != "2026-12-20" {
		t.Errorf("approval = %+v", e.repo.approval)
	}
	if len(e.expenses.budgetCalls) != 0 || len(e.expenses.built) != 0 || res.Feedback != nil {
		t.Errorf("the future path touched the budget: calls %v built %v feedback %v", e.expenses.budgetCalls, e.expenses.built, res.Feedback)
	}
	if len(e.mailer.sent) != 1 || !strings.Contains(e.mailer.sent[0].text, "gastos futuros") {
		t.Errorf("requester email = %+v", e.mailer.sent)
	}
}

func TestApproveRejectsInvalidDecisionsWithoutChangingTheRequest(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	bad := map[string]app.ApproveInput{
		"no destination":      {},
		"unknown destination": {Destination: "regalo"},
		"expense no category": {Destination: domain.DestinationExpense},
		"expense Ingreso":     {Destination: domain.DestinationExpense, Category: "Sueldo"},
		"expense wrong case":  {Destination: domain.DestinationExpense, Category: "mandado"},
		"expense bad date":    {Destination: domain.DestinationExpense, Category: "Mandado", Date: "mañana"},
		"future no due date":  {Destination: domain.DestinationFuture},
		"future bad due date": {Destination: domain.DestinationFuture, DueDate: "2026-13-45"},
	}
	for name, in := range bad {
		// The future expense rules answer with their own invalid-input error.
		_, err := e.svc.Approve(ctx, owner, r.ID, in)
		if !errors.Is(err, domain.ErrInvalidInput) && !errors.Is(err, futuredomain.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want an invalid input error", name, err)
		}
	}
	// The expenses rules (payment method) decide the Gasto too.
	if _, err := e.svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationExpense, Category: "Mandado", PaymentMethod: "Bitcoin"}); err == nil {
		t.Error("a payment method refused by the expenses rules was approved")
	}
	if got, _ := e.repo.Get(ctx, r.ID); got.Status != domain.StatusPending {
		t.Errorf("status = %s after invalid approvals, want solicitada", got.Status)
	}
	if len(e.mailer.sent) != 0 {
		t.Errorf("%d emails for failed approvals", len(e.mailer.sent))
	}
}

func TestApproveAndRejectAreOwnerOnlyAndDecidedRequestsConflict(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	in := app.ApproveInput{Destination: domain.DestinationExpense, Category: "Mandado"}
	if _, err := e.svc.Approve(ctx, spouse, r.ID, in); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("household approve = %v", err)
	}
	if _, err := e.svc.Reject(ctx, spouse, r.ID, "no"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("household reject = %v", err)
	}
	if _, err := e.svc.Approve(ctx, owner, r.ID, in); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(ctx, owner, r.ID, in); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("second approve = %v, want ErrInvalidState", err)
	}
	if _, err := e.svc.Reject(ctx, owner, r.ID, "tarde"); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("reject after approve = %v", err)
	}
	if _, err := e.svc.Cancel(ctx, spouse, r.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("cancel after approve = %v", err)
	}
	if _, err := e.svc.Approve(ctx, owner, 999, in); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("approve unknown = %v", err)
	}
}

func TestRejectNeedsACommentAndTellsTheRequester(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	for _, c := range []string{"", "   ", strings.Repeat("a", 501)} {
		if _, err := e.svc.Reject(ctx, owner, r.ID, c); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("comment %q = %v, want ErrInvalidInput", c, err)
		}
	}
	got, err := e.svc.Reject(ctx, owner, r.ID, "  Lo vemos el mes que entra ")
	if err != nil || got.Status != domain.StatusRejected || got.DecisionComment != "Lo vemos el mes que entra" {
		t.Fatalf("Reject = %+v, %v", got, err)
	}
	if len(e.mailer.sent) != 1 || e.mailer.sent[0].to != "u-spouse@example.com" || e.mailer.sent[0].subject != "Tu petición de gasto fue rechazada" ||
		!strings.Contains(e.mailer.sent[0].text, "Lo vemos el mes que entra") {
		t.Errorf("email = %+v", e.mailer.sent)
	}
}

func TestEmailsEscapeUserText(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Create(context.Background(), spouse, domain.Input{Amount: d("5"), Description: `<script>alert(1)</script>`})
	if err != nil {
		t.Fatal(err)
	}
	if h := e.mailer.sent[0].html; strings.Contains(h, "<script>") || !strings.Contains(h, "&lt;script&gt;") {
		t.Errorf("html body is not escaped: %s", h)
	}
}

func TestRevertIsOwnerOnlyAndOnlyForApprovedRequests(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	if _, err := e.svc.Revert(ctx, owner, r.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("revert of a pending request = %v, want ErrInvalidState", err)
	}
	if _, err := e.svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationExpense, Category: "Mandado"}); err != nil {
		t.Fatal(err)
	}
	e.mailer.sent = nil
	for _, who := range []session.Identity{spouse, otherHome, {}} {
		if _, err := e.svc.Revert(ctx, who, r.ID); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("revert as %+v = %v, want ErrForbidden", who, err)
		}
	}
	if len(e.mailer.sent) != 0 {
		t.Errorf("a refused revert sent %d emails", len(e.mailer.sent))
	}
	got, err := e.svc.Revert(ctx, owner, r.ID)
	if err != nil || got.Status != domain.StatusPending || got.RevertCount != 1 || got.RevertedAt == nil || got.ResultKind != "" {
		t.Fatalf("Revert = %+v, %v", got, err)
	}
	if _, err := e.svc.Revert(ctx, owner, r.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("second revert = %v, want ErrInvalidState", err)
	}
	if _, err := e.svc.Revert(ctx, owner, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("revert unknown = %v", err)
	}
	// It is a normal pending request again: it can be approved once more.
	if _, err := e.svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationFuture, DueDate: "2026-12-20"}); err != nil {
		t.Errorf("approve after revert = %v", err)
	}
}

func TestRevertTellsTheRequesterButNeverFailsOnEmailErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	if _, err := e.svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationExpense, Category: "Mandado"}); err != nil {
		t.Fatal(err)
	}
	e.mailer.sent = nil
	if _, err := e.svc.Revert(ctx, owner, r.ID); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 1 || e.mailer.sent[0].to != "u-spouse@example.com" || e.mailer.sent[0].subject != "Tu petición de gasto volvió a solicitada" ||
		!strings.Contains(e.mailer.sent[0].text, "volvió a solicitada") || !strings.Contains(e.mailer.sent[0].text, "https://app.example.com/peticiones") {
		t.Errorf("email = %+v", e.mailer.sent)
	}

	// A failing mailer is logged, never an error of the revert.
	if _, err := e.svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationExpense, Category: "Mandado"}); err != nil {
		t.Fatal(err)
	}
	e.mailer.err = errors.New("resend is down")
	if got, err := e.svc.Revert(ctx, owner, r.ID); err != nil || got.RevertCount != 2 {
		t.Errorf("Revert with a failing mailer = %+v, %v", got, err)
	}
}

func TestRevertRefusedForAPaidFutureExpenseSendsNoEmail(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.request(t, "300")
	if _, err := e.svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationFuture, DueDate: "2026-12-20"}); err != nil {
		t.Fatal(err)
	}
	e.mailer.sent = nil
	e.repo.revertErr = domain.ErrFutureExpensePaid
	if _, err := e.svc.Revert(ctx, owner, r.ID); !errors.Is(err, domain.ErrFutureExpensePaid) {
		t.Errorf("Revert = %v, want ErrFutureExpensePaid", err)
	}
	if len(e.mailer.sent) != 0 {
		t.Errorf("%d emails after a refused revert", len(e.mailer.sent))
	}
}
