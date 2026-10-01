package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	futuredomain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// Creation limit per user (fixed window, per process).
const (
	CreateHourlyLimit = 20
	CreateWindow      = time.Hour

	keyCreate = "expense-request:create:"
	// mailTimeout bounds the best-effort emails of one call.
	mailTimeout = 10 * time.Second
)

// Deps are the collaborators of Service.
type Deps struct {
	Repo     Repo
	Expenses Expenses
	Settings Settings
	Mailer   Mailer
	Limiter  RateLimiter
	// AppBaseURL is the public URL of the app, used for the link of the emails.
	AppBaseURL string
	// Now is the clock in TZ_NAME ("today" is never UTC). Nil selects time.Now.
	Now    func() time.Time
	Logger *slog.Logger
}

// Service implements the expense request use cases.
type Service struct{ Deps }

// NewService builds a Service.
func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{Deps: d}
}

func (s *Service) today() string { return s.Now().Format(domain.DateLayout) }

// authenticated is the defense in depth behind the HTTP gate: only a known
// role with a user id may use the module.
func authenticated(actor Identity) error {
	if actor.UserID == "" || !actor.Role.Valid() {
		return domain.ErrForbidden
	}
	return nil
}

func requireOwner(actor Identity) error {
	if actor.UserID == "" || actor.Role != session.RoleOwner {
		return domain.ErrForbidden
	}
	return nil
}

// gastoCategories returns the names of the Gasto categories.
func (s *Service) gastoCategories(ctx context.Context) ([]string, error) {
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	names := cfg.Catalog().CategoryNames(ledger.KindExpense)
	if names == nil {
		names = []string{}
	}
	return names, nil
}

// Categories returns only the names of the Gasto categories, so a household
// user can pick a suggestion without reading the settings (no budgets).
func (s *Service) Categories(ctx context.Context, actor Identity) ([]string, error) {
	if err := authenticated(actor); err != nil {
		return nil, err
	}
	return s.gastoCategories(ctx)
}

// Create stores a pending request of the caller and tells the owners.
func (s *Service) Create(ctx context.Context, actor Identity, in domain.Input) (domain.Request, error) {
	if err := authenticated(actor); err != nil {
		return domain.Request{}, err
	}
	cats, err := s.gastoCategories(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	v, err := domain.Validate(in, cats, s.today())
	if err != nil {
		return domain.Request{}, err
	}
	// Only a valid request consumes the budget of the window.
	if !s.Limiter.Allow(keyCreate+actor.UserID, CreateHourlyLimit, CreateWindow) {
		return domain.Request{}, domain.ErrRateLimited
	}
	req, err := s.Repo.Create(ctx, actor.UserID, v)
	if err != nil {
		return domain.Request{}, err
	}
	s.notifyOwners(ctx, req)
	return req, nil
}

// List returns the requests, newest first. A household caller sees only their
// own; the owner sees every request and may filter by status.
func (s *Service) List(ctx context.Context, actor Identity, status string) ([]domain.Request, error) {
	if err := authenticated(actor); err != nil {
		return nil, err
	}
	f := ListFilter{}
	if status != "" {
		st, err := domain.ParseStatus(status)
		if err != nil {
			return nil, err
		}
		f.Status = st
	}
	if actor.Role != session.RoleOwner {
		f.RequesterID = actor.UserID
	}
	return s.Repo.List(ctx, f)
}

// Cancel withdraws a pending request. Only the requester may.
func (s *Service) Cancel(ctx context.Context, actor Identity, id int) (domain.Request, error) {
	if err := authenticated(actor); err != nil {
		return domain.Request{}, err
	}
	req, err := s.Repo.Get(ctx, id)
	if err != nil {
		return domain.Request{}, err
	}
	if req.RequesterID != actor.UserID {
		return domain.Request{}, domain.ErrForbidden
	}
	return s.Repo.Cancel(ctx, id, actor.UserID)
}

// BudgetCheck is the owner's preview of approving a request as a Gasto of the
// given category on the given date (the request's own date when empty): the
// budget of the pay cycle that contains the date, what is spent, and what the
// request would do to both. It stores nothing and never blocks the approval.
func (s *Service) BudgetCheck(ctx context.Context, actor Identity, id int, category, date string) (domain.BudgetCheck, error) {
	if err := requireOwner(actor); err != nil {
		return domain.BudgetCheck{}, err
	}
	req, err := s.Repo.Get(ctx, id)
	if err != nil {
		return domain.BudgetCheck{}, err
	}
	cats, err := s.gastoCategories(ctx)
	if err != nil {
		return domain.BudgetCheck{}, err
	}
	if err := domain.ValidateCategory(category, cats); err != nil {
		return domain.BudgetCheck{}, err
	}
	date = strings.TrimSpace(date)
	if date == "" {
		date = req.ExpenseDate
	} else if _, err := domain.ParseDate("date", date); err != nil {
		return domain.BudgetCheck{}, err
	}
	fb, err := s.Expenses.BudgetFor(ctx, category, date)
	if err != nil {
		return domain.BudgetCheck{}, err
	}
	return domain.NewBudgetCheck(category, fb.Budget, fb.Spent, req.Amount), nil
}

// ApproveInput is the owner's decision: where the money goes.
type ApproveInput struct {
	Destination domain.Destination
	// Category, Date and PaymentMethod apply to DestinationExpense. Date
	// defaults to the date of the request and PaymentMethod to Débito.
	Category      string
	Date          string
	PaymentMethod string
	// DueDate applies to DestinationFuture.
	DueDate string
}

// ApproveResult is the approved request and, for a Gasto, the budget feedback
// of its category after the expense was registered (nil when it cannot be
// computed: the approval is already committed).
type ApproveResult struct {
	Request  domain.Request
	Feedback *expensesapp.BudgetFeedback
}

// Approve registers the Gasto or the future expense and marks the request
// aprobada in one transaction. A Gasto that does not fit its budget is still
// approved: the check is informational. The future expense path never looks at
// the budget.
func (s *Service) Approve(ctx context.Context, actor Identity, id int, in ApproveInput) (ApproveResult, error) {
	if err := requireOwner(actor); err != nil {
		return ApproveResult{}, err
	}
	req, err := s.Repo.Get(ctx, id)
	if err != nil {
		return ApproveResult{}, err
	}
	if req.Status != domain.StatusPending {
		return ApproveResult{}, domain.ErrInvalidState
	}
	var a Approval
	switch in.Destination {
	case domain.DestinationExpense:
		cats, err := s.gastoCategories(ctx)
		if err != nil {
			return ApproveResult{}, err
		}
		if err := domain.ValidateCategory(in.Category, cats); err != nil {
			return ApproveResult{}, err
		}
		date := strings.TrimSpace(in.Date)
		if date == "" {
			date = req.ExpenseDate
		} else if _, err := domain.ParseDate("date", date); err != nil {
			return ApproveResult{}, err
		}
		m, err := s.Expenses.Build(ctx, expensesapp.Input{
			Date: date, Description: req.Description, Category: in.Category,
			PaymentMethod: in.PaymentMethod, Amount: req.Amount,
		})
		if err != nil {
			return ApproveResult{}, err
		}
		a = Approval{Destination: in.Destination, Expense: &m}
	case domain.DestinationFuture:
		v, err := futuredomain.Validate(futuredomain.Input{Name: req.Description, Target: req.Amount, DueDate: in.DueDate})
		if err != nil {
			return ApproveResult{}, err
		}
		a = Approval{Destination: in.Destination, Future: &v}
	default:
		return ApproveResult{}, fmt.Errorf("%w: destination must be gasto or gasto_futuro", domain.ErrInvalidInput)
	}

	approved, err := s.Repo.Approve(ctx, id, actor.UserID, a)
	if err != nil {
		return ApproveResult{}, err
	}
	out := ApproveResult{Request: approved}
	if a.Expense != nil {
		fb, err := s.Expenses.BudgetFor(ctx, a.Expense.Category, a.Expense.Date)
		if err != nil {
			s.Logger.Warn("request approved but the budget feedback could not be computed", "request", id, "error", err)
		} else {
			out.Feedback = &fb
		}
	}
	s.notifyRequester(ctx, approved)
	return out, nil
}

// Reject moves a pending request to rechazada. The comment is required.
func (s *Service) Reject(ctx context.Context, actor Identity, id int, comment string) (domain.Request, error) {
	if err := requireOwner(actor); err != nil {
		return domain.Request{}, err
	}
	c, err := domain.ValidateComment(comment)
	if err != nil {
		return domain.Request{}, err
	}
	req, err := s.Repo.Reject(ctx, id, actor.UserID, c)
	if err != nil {
		return domain.Request{}, err
	}
	s.notifyRequester(ctx, req)
	return req, nil
}

// Revert undoes an approval: the Gasto or the active future expense it created
// is deleted and the request goes back to solicitada, in one transaction. Only
// an aprobada request can be reverted (409 otherwise) and a future expense that
// is already paid refuses it. The requester is told, best-effort.
func (s *Service) Revert(ctx context.Context, actor Identity, id int) (domain.Request, error) {
	if err := requireOwner(actor); err != nil {
		return domain.Request{}, err
	}
	req, err := s.Repo.Revert(ctx, id)
	if err != nil {
		return domain.Request{}, err
	}
	s.notifyReverted(ctx, req)
	return req, nil
}
