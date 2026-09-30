// Package app holds the Bills & Subscriptions use cases: managing bills,
// listing them with their next occurrence and overdue / due-soon flags, and
// resolving the next occurrence by paying it (which registers an expense
// through the expenses module) or skipping it. Nothing is sent or scheduled
// here: the email reminders of a later phase will read the lead time and the
// flags this module exposes.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// Status is a bill with the flags computed for a day. DaysUntilDue is negative
// when the pending occurrence is overdue; the flags are false without one.
type Status struct {
	Bill         bills.Bill
	Overdue      bool
	DueSoon      bool
	DaysUntilDue int
}

// Detail is a bill with its status and its resolved history, newest first.
type Detail struct {
	Status
	History []bills.Occurrence
}

// PayResult is the outcome of a payment: the bill with its next occurrence, the
// occurrence just paid and the expense registered for it.
type PayResult struct {
	Status
	Paid    bills.Occurrence
	Expense ledger.Movement
}

// Service implements the bills use cases.
type Service struct {
	repo     Repo
	expenses Expenses
	settings Settings
	now      func() time.Time
	logger   *slog.Logger
}

// NewService builds a Service. A nil now selects time.Now and a nil logger
// slog.Default().
func NewService(repo Repo, expenses Expenses, settings Settings, now func() time.Time, logger *slog.Logger) *Service {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, expenses: expenses, settings: settings, now: now, logger: logger}
}

func (s *Service) today() string { return s.now().Format(bills.DateLayout) }

func (s *Service) status(b bills.Bill) Status {
	st := Status{Bill: b}
	if b.Pending == nil {
		return st
	}
	today := s.today()
	st.Overdue = bills.IsOverdue(b.Pending.DueDate, today)
	st.DueSoon = bills.IsDueSoon(b.Pending.DueDate, today, b.ReminderLeadDays)
	st.DaysUntilDue, _ = bills.DaysUntil(b.Pending.DueDate, today) // dates were validated when stored
	return st
}

// validate checks the input and resolves its category to the canonical name of
// an expense category of the settings.
func (s *Service) validate(ctx context.Context, in bills.Input) (bills.Validated, error) {
	v, err := bills.Validate(in)
	if err != nil {
		return bills.Validated{}, err
	}
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return bills.Validated{}, err
	}
	name, ok := cfg.Catalog().FindCategory(v.Category, ledger.KindExpense)
	if !ok {
		return bills.Validated{}, fmt.Errorf("%w: %q is not an expense category", bills.ErrInvalidInput, v.Category)
	}
	v.Category = name
	return v, nil
}

// Create stores a bill and its first occurrence, due on in.NextDueDate.
func (s *Service) Create(ctx context.Context, in bills.Input) (Status, error) {
	v, err := s.validate(ctx, in)
	if err != nil {
		return Status{}, err
	}
	b, err := s.repo.Create(ctx, v)
	if err != nil {
		return Status{}, err
	}
	return s.status(b), nil
}

// List returns the bills sorted by the due date of their pending occurrence
// (overdue first), inactive ones only when asked for.
func (s *Service) List(ctx context.Context, includeInactive bool) ([]Status, error) {
	list, err := s.repo.List(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]Status, len(list))
	for i, b := range list {
		out[i] = s.status(b)
	}
	sort.SliceStable(out, func(i, j int) bool { return dueKey(out[i].Bill) < dueKey(out[j].Bill) })
	return out, nil
}

// dueKey sorts by the pending due date; a bill without one goes last.
func dueKey(b bills.Bill) string {
	if b.Pending == nil {
		return "9999-99-99"
	}
	return b.Pending.DueDate
}

// Get returns a bill with its history.
func (s *Service) Get(ctx context.Context, id int) (Detail, error) {
	b, err := s.repo.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	history, err := s.repo.History(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Status: s.status(b), History: history}, nil
}

// Update replaces the editable fields of a bill. Changing the next due date
// moves the pending occurrence and re-anchors the schedule on its day; sending
// the current due date unchanged keeps the stored anchor, so a bill due on the
// 31st does not drift after an edit made in a short month. The recurrence and
// amount apply to the following occurrences.
func (s *Service) Update(ctx context.Context, id int, in bills.Input) (Status, error) {
	v, err := s.validate(ctx, in)
	if err != nil {
		return Status{}, err
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Status{}, err
	}
	if current.NextDueDate == v.NextDueDate {
		v.AnchorDay = current.AnchorDay
	}
	b, err := s.repo.Update(ctx, id, v)
	if err != nil {
		return Status{}, err
	}
	return s.status(b), nil
}

// Deactivate turns a bill off: it stops being listed and cannot be paid or
// skipped, and its history stays. Expenses registered by past payments are never
// touched. Update with Active true reactivates it.
func (s *Service) Deactivate(ctx context.Context, id int) error {
	return s.repo.Deactivate(ctx, id)
}

// pending loads a bill that can be resolved: active and with a pending occurrence.
func (s *Service) pending(ctx context.Context, id int) (bills.Bill, error) {
	b, err := s.repo.Get(ctx, id)
	if err != nil {
		return bills.Bill{}, err
	}
	if !b.Active {
		return bills.Bill{}, bills.ErrInactive
	}
	if b.Pending == nil {
		return bills.Bill{}, bills.ErrNotPending
	}
	return b, nil
}

// Pay registers the payment of the bill's pending occurrence. The expense is
// created through the expenses module (amount defaults to the bill's, category
// to the bill's, date to today; all overridable), then the occurrence is marked
// paid and the next one generated, atomically. When that fails the expense is
// removed again, so no orphan movement is left behind.
func (s *Service) Pay(ctx context.Context, id int, in bills.PaymentInput) (PayResult, error) {
	b, err := s.pending(ctx, id)
	if err != nil {
		return PayResult{}, err
	}
	p, err := bills.ResolvePayment(b, in, s.today())
	if err != nil {
		return PayResult{}, err
	}
	next, err := bills.NextDue(b.Recurrence, b.AnchorDay, b.Pending.DueDate)
	if err != nil {
		return PayResult{}, err
	}
	res, err := s.expenses.Create(ctx, expensesapp.Input{
		Date: p.Date, Description: p.Description, Category: p.Category, Currency: b.Currency, Amount: p.Amount,
	})
	if err != nil {
		return PayResult{}, err
	}
	expense := res.Movement
	updated, err := s.repo.Resolve(ctx, Resolution{
		BillID: id, OccurrenceID: b.Pending.ID, Status: bills.StatusPaid,
		PaidOn: p.Date, AmountPaid: p.Amount, Currency: b.Currency, ExpenseID: &expense.ID, NextDueDate: next,
	})
	if err != nil {
		s.rollbackExpense(ctx, expense.ID)
		return PayResult{}, err
	}
	paid := *b.Pending
	amount := p.Amount
	paid.Status, paid.PaidOn, paid.AmountPaid, paid.Currency, paid.ExpenseID = bills.StatusPaid, p.Date, &amount, b.Currency, &expense.ID
	return PayResult{Status: s.status(updated), Paid: paid, Expense: expense}, nil
}

// Skip resolves the pending occurrence without paying it: no expense is
// registered and the next occurrence is generated.
func (s *Service) Skip(ctx context.Context, id int) (Status, error) {
	b, err := s.pending(ctx, id)
	if err != nil {
		return Status{}, err
	}
	next, err := bills.NextDue(b.Recurrence, b.AnchorDay, b.Pending.DueDate)
	if err != nil {
		return Status{}, err
	}
	updated, err := s.repo.Resolve(ctx, Resolution{
		BillID: id, OccurrenceID: b.Pending.ID, Status: bills.StatusSkipped, NextDueDate: next,
	})
	if err != nil {
		return Status{}, err
	}
	return s.status(updated), nil
}

// rollbackExpense removes an expense created for a payment that then failed. A
// failure is only logged: the caller already has the original error to report.
func (s *Service) rollbackExpense(ctx context.Context, id int) {
	if err := s.expenses.Delete(context.WithoutCancel(ctx), id); err != nil && !errors.Is(err, ledger.ErrNotFound) {
		s.logger.Error("could not remove the expense of a failed bill payment", "movement", id, "error", err)
	}
}
