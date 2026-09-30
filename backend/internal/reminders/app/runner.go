// Package app runs the email reminders: every tick it asks which bills call
// for an email today (per-payment, overdue repeat, weekly summary), checks the
// disk usage, sends what was not sent yet and records each send. The keys are
// recorded only after a successful send, so a failed email is retried on the
// next tick and one failure never blocks the others.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	billsapp "github.com/valium69mg/finances-app/backend/internal/bills/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	"github.com/valium69mg/finances-app/backend/internal/reminders/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Default cadence of the runner.
const (
	DefaultInterval   = 30 * time.Minute
	DefaultStartDelay = 15 * time.Second
)

// BillLister lists the bills with their flags for today. Implemented by the
// bills service.
type BillLister interface {
	List(ctx context.Context, includeInactive bool) ([]billsapp.Status, error)
}

// TaxPending lists the periods with issued invoices and no registered filing,
// each with its deadline. Implemented by the taxfiling service.
type TaxPending interface {
	Pending(ctx context.Context) ([]taxfiling.PendingPeriod, error)
}

// InvoiceLister lists invoices of a YYYY-MM period and status. Implemented by
// the invoices service.
type InvoiceLister interface {
	List(ctx context.Context, period string, status invoices.Status) ([]invoices.Invoice, error)
}

// Mailer sends one email.
type Mailer interface {
	Send(ctx context.Context, to, subject, html, text string) error
}

// SentLog remembers which reminder emails went out.
type SentLog interface {
	// Has reports whether the key was recorded.
	Has(ctx context.Context, key string) (bool, error)
	// Record stores the key; recording an existing key is not an error.
	Record(ctx context.Context, key string) error
	// LastSent returns when the most recent key starting with prefix was
	// recorded, or nil when there is none.
	LastSent(ctx context.Context, prefix string) (*time.Time, error)
}

// DiskProbe measures the used percentage (0..100) of the monitored volume.
type DiskProbe interface {
	UsedPercent() (float64, error)
}

// OwnerEmail returns the address the reminders go to, or domain.ErrNoOwner.
type OwnerEmail interface {
	OwnerEmail(ctx context.Context) (string, error)
}

// Deps are the collaborators and settings of a Runner. Disk, Tax and Invoices
// may be nil to turn the disk alert, the tax filing reminders and the salary
// invoice reminder off. Now must return the time in the zone that defines
// "today" (and the 08:00 gate); nil selects time.Now.
type Deps struct {
	Bills        BillLister
	Mailer       Mailer
	Log          SentLog
	Disk         DiskProbe
	Tax          TaxPending
	Invoices     InvoiceLister
	Owner        OwnerEmail
	AppBaseURL   string
	DiskAlertPct int
	Now          func() time.Time
	Logger       *slog.Logger
	// Interval and StartDelay default to DefaultInterval and DefaultStartDelay.
	Interval   time.Duration
	StartDelay time.Duration
}

// Runner is the in-process reminder job.
type Runner struct {
	Deps
}

// NewRunner builds a Runner, filling the defaults of nil or zero fields.
func NewRunner(d Deps) *Runner {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Interval <= 0 {
		d.Interval = DefaultInterval
	}
	if d.StartDelay <= 0 {
		d.StartDelay = DefaultStartDelay
	}
	return &Runner{Deps: d}
}

// Run ticks shortly after start and then every Interval until ctx is done. It
// returns only after the tick in progress, if any, has finished.
func (r *Runner) Run(ctx context.Context) {
	timer := time.NewTimer(r.StartDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.Tick(ctx)
			timer.Reset(r.Interval)
		}
	}
}

// Tick evaluates every reminder once. Nothing is sent before SendFromHour
// local time. A panic is logged instead of taking the API down.
func (r *Runner) Tick(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			r.Logger.Error("reminders tick panicked", "panic", fmt.Sprint(p))
		}
	}()

	now := r.Now()
	if now.Hour() < domain.SendFromHour {
		return
	}
	today := now.Format("2006-01-02")

	r.billReminders(ctx, today)
	r.taxReminders(ctx, today)
	r.salaryReminder(ctx, today)
	r.diskAlert(ctx, now, today)
}

// taxReminders emails about the periods still to file. A lister failure is
// logged and leaves the other reminders untouched.
func (r *Runner) taxReminders(ctx context.Context, today string) {
	if r.Tax == nil || ctx.Err() != nil {
		return
	}
	pending, err := r.Tax.Pending(ctx)
	if err != nil {
		r.Logger.Error("reminders: could not list the pending tax filings", "error", err)
		return
	}
	periods := make([]domain.TaxPeriod, 0, len(pending))
	for _, p := range pending {
		periods = append(periods, domain.TaxPeriod{Period: p.Period, DueDate: p.DueDate})
	}
	for _, e := range domain.TaxEmails(periods, today) {
		if ctx.Err() != nil {
			return
		}
		r.sendOnce(ctx, e.Key, func() domain.Message { return domain.Compose(e, r.AppBaseURL) })
	}
}

// salaryReminder emails on the last day of the month unless an invoice of that
// month is already issued. When that cannot be checked nothing is sent, and the
// next tick tries again.
func (r *Runner) salaryReminder(ctx context.Context, today string) {
	month, ok := domain.SalaryMonth(today)
	if !ok || r.Invoices == nil || ctx.Err() != nil {
		return
	}
	issued, err := r.Invoices.List(ctx, month, invoices.StatusIssued)
	if err != nil {
		r.Logger.Error("reminders: could not list the issued invoices", "month", month, "error", err)
		return
	}
	if e, ok := domain.SalaryEmail(month, len(issued) > 0); ok {
		r.sendOnce(ctx, e.Key, func() domain.Message { return domain.Compose(e, r.AppBaseURL) })
	}
}

func (r *Runner) billReminders(ctx context.Context, today string) {
	statuses, err := r.Bills.List(ctx, false)
	if err != nil {
		r.Logger.Error("reminders: could not list the bills", "error", err)
		return
	}
	items := make([]domain.Item, 0, len(statuses))
	for _, st := range statuses {
		if it, ok := toItem(st); ok {
			items = append(items, it)
		}
	}

	emails := domain.BillEmails(items)
	if weekly, ok := domain.WeeklyEmail(items, today); ok {
		emails = append(emails, weekly)
	}
	for _, e := range emails {
		if ctx.Err() != nil {
			return
		}
		r.sendOnce(ctx, e.Key, func() domain.Message { return domain.Compose(e, r.AppBaseURL) })
	}
}

func (r *Runner) diskAlert(ctx context.Context, now time.Time, today string) {
	if r.Disk == nil || ctx.Err() != nil {
		return
	}
	used, err := r.Disk.UsedPercent()
	if err != nil {
		r.Logger.Error("reminders: could not measure the disk", "error", err)
		return
	}
	if used < float64(r.DiskAlertPct) {
		return
	}
	last, err := r.Log.LastSent(ctx, domain.DiskKeyPrefix)
	if err != nil {
		r.Logger.Error("reminders: could not read the disk alert log", "error", err)
		return
	}
	if !domain.DiskAlertDue(used, r.DiskAlertPct, last, now) {
		return
	}
	r.sendOnce(ctx, domain.DiskKey(today), func() domain.Message { return domain.DiskMessage(used, r.DiskAlertPct) })
}

// sendOnce sends the message of key unless the key was already recorded, and
// records it after a successful send. Every failure is logged and swallowed.
func (r *Runner) sendOnce(ctx context.Context, key string, build func() domain.Message) {
	sent, err := r.Log.Has(ctx, key)
	if err != nil {
		r.Logger.Error("reminders: could not read the sent log", "key", key, "error", err)
		return
	}
	if sent {
		return
	}
	to, err := r.Owner.OwnerEmail(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNoOwner) {
			r.Logger.Warn("reminders: there is no verified user to notify yet", "key", key)
		} else {
			r.Logger.Error("reminders: could not find the owner email", "key", key, "error", err)
		}
		return
	}
	m := build()
	if err := r.Mailer.Send(ctx, to, m.Subject, m.HTML, m.Text); err != nil {
		r.Logger.Error("reminders: the email was not sent, it will be retried", "key", key, "error", err)
		return
	}
	// The email is out: record it even if shutdown began meanwhile, or the next
	// start would send it again.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := r.Log.Record(recordCtx, key); err != nil {
		// The email went out but the key is lost: the next tick may send it again.
		r.Logger.Error("reminders: the email was sent but its key could not be recorded", "key", key, "error", err)
		return
	}
	r.Logger.Info("reminder sent", "key", key)
}

// toItem maps a bill status to a reminder item. A bill without a pending
// occurrence has nothing to remind about.
func toItem(st billsapp.Status) (domain.Item, bool) {
	p := st.Bill.Pending
	if p == nil {
		return domain.Item{}, false
	}
	it := domain.Item{
		OccurrenceID: p.ID, Name: st.Bill.Name, Currency: st.Bill.Currency, DueDate: p.DueDate,
		DaysUntilDue: st.DaysUntilDue, DueSoon: st.DueSoon, Overdue: st.Overdue,
	}
	if st.Bill.Amount != nil {
		it.Amount = st.Bill.Amount.StringFixed(2)
	}
	return it, true
}
