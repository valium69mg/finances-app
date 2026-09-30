package app_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	billsapp "github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	"github.com/valium69mg/finances-app/backend/internal/reminders/app"
	"github.com/valium69mg/finances-app/backend/internal/reminders/domain"
)

var mexico = time.FixedZone("CST", -6*60*60)

type fakeBills struct {
	list []billsapp.Status
	err  error
}

func (f *fakeBills) List(_ context.Context, includeInactive bool) ([]billsapp.Status, error) {
	if includeInactive {
		panic("reminders must list active bills only")
	}
	return f.list, f.err
}

type sentMail struct{ to, subject, html, text string }

type fakeMailer struct {
	mu     sync.Mutex
	sent   []sentMail
	failOn func(subject string) bool
}

func (f *fakeMailer) Send(_ context.Context, to, subject, html, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn != nil && f.failOn(subject) {
		return errors.New("resend is down")
	}
	f.sent = append(f.sent, sentMail{to, subject, html, text})
	return nil
}

func (f *fakeMailer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type fakeLog struct {
	keys      map[string]time.Time
	now       func() time.Time
	recordErr error
	hasErr    error
}

func newFakeLog(now func() time.Time) *fakeLog {
	return &fakeLog{keys: map[string]time.Time{}, now: now}
}

func (f *fakeLog) Has(_ context.Context, key string) (bool, error) {
	_, ok := f.keys[key]
	return ok, f.hasErr
}

func (f *fakeLog) Record(_ context.Context, key string) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	if _, ok := f.keys[key]; !ok {
		f.keys[key] = f.now()
	}
	return nil
}

func (f *fakeLog) LastSent(_ context.Context, prefix string) (*time.Time, error) {
	var last *time.Time
	for k, at := range f.keys {
		if strings.HasPrefix(k, prefix) && (last == nil || at.After(*last)) {
			at := at
			last = &at
		}
	}
	return last, nil
}

type fakeDisk struct {
	used float64
	err  error
}

func (f *fakeDisk) UsedPercent() (float64, error) { return f.used, f.err }

type fakeOwner struct {
	email string
	err   error
}

func (f fakeOwner) OwnerEmail(context.Context) (string, error) { return f.email, f.err }

// rig wires a Runner with fakes and a clock the test moves.
type rig struct {
	now    time.Time
	bills  *fakeBills
	mailer *fakeMailer
	log    *fakeLog
	disk   *fakeDisk
	owner  fakeOwner
	logs   *bytes.Buffer
	runner *app.Runner
}

// newRig starts on Monday 2026-10-05 at 09:00 Mexico time.
func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		now:    time.Date(2026, 10, 5, 9, 0, 0, 0, mexico),
		bills:  &fakeBills{},
		mailer: &fakeMailer{},
		disk:   &fakeDisk{used: 10},
		owner:  fakeOwner{email: "owner@example.com"},
		logs:   &bytes.Buffer{},
	}
	r.log = newFakeLog(func() time.Time { return r.now })
	r.build()
	return r
}

func (r *rig) build() {
	r.runner = app.NewRunner(app.Deps{
		Bills: r.bills, Mailer: r.mailer, Log: r.log, Disk: r.disk, Owner: r.owner,
		AppBaseURL: "https://app.example.com", DiskAlertPct: 80,
		Now:    func() time.Time { return r.now },
		Logger: slog.New(slog.NewTextHandler(r.logs, nil)),
	})
}

func (r *rig) tick() { r.runner.Tick(context.Background()) }

func (r *rig) advance(d time.Duration) { r.now = r.now.Add(d) }

func status(occurrence int, name, due string, days int, lead int, amount string) billsapp.Status {
	b := bills.Bill{
		ID: occurrence, Name: name, Currency: "MXN", ReminderLeadDays: lead, Active: true, NextDueDate: due,
		Pending: &bills.Occurrence{ID: occurrence, BillID: occurrence, DueDate: due, Status: bills.StatusPending},
	}
	if amount != "" {
		v := decimal.RequireFromString(amount)
		b.Amount = &v
	}
	return billsapp.Status{
		Bill: b, Overdue: days < 0, DueSoon: days >= 0 && days <= lead, DaysUntilDue: days,
	}
}

func TestNothingBeforeEightLocal(t *testing.T) {
	r := newRig(t)
	r.bills.list = []billsapp.Status{status(1, "Luz", "2026-10-06", 1, 3, "550")}
	r.disk.used = 95
	r.now = time.Date(2026, 10, 5, 7, 59, 0, 0, mexico)

	r.tick()
	if n := r.mailer.count(); n != 0 {
		t.Fatalf("sent %d emails before 08:00", n)
	}

	r.now = time.Date(2026, 10, 5, 8, 0, 0, 0, mexico)
	r.tick()
	if n := r.mailer.count(); n == 0 {
		t.Fatal("nothing sent at 08:00")
	}
}

func TestDueSoonSentOnceAndDeduped(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico) // a Tuesday: no weekly summary
	r.bills.list = []billsapp.Status{status(7, "Luz", "2026-10-08", 2, 3, "550")}

	r.tick()
	r.tick() // same day again (the 30 minute ticker)
	r.advance(24 * time.Hour)
	r.bills.list = []billsapp.Status{status(7, "Luz", "2026-10-08", 1, 3, "550")}
	r.tick() // next day, same occurrence

	if n := r.mailer.count(); n != 1 {
		t.Fatalf("sent %d emails, want exactly 1", n)
	}
	m := r.mailer.sent[0]
	if m.to != "owner@example.com" || !strings.Contains(m.subject, "Luz") {
		t.Errorf("unexpected email %+v", m)
	}
	if _, ok := r.log.keys["bill_due:7"]; !ok {
		t.Errorf("key not recorded: %v", r.log.keys)
	}
	if !strings.Contains(m.text, "550.00 MXN") || !strings.Contains(m.text, "08/10/2026") || !strings.Contains(m.text, "https://app.example.com") {
		t.Errorf("text = %q", m.text)
	}
}

func TestNotDueSoonSendsNothing(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	r.bills.list = []billsapp.Status{status(1, "Luz", "2026-10-20", 14, 3, "550")}
	r.tick()
	if n := r.mailer.count(); n != 0 {
		t.Fatalf("sent %d emails", n)
	}
}

func TestSendFailureIsRetriedAndDoesNotBlockOthers(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	r.bills.list = []billsapp.Status{
		status(1, "Luz", "2026-10-07", 1, 3, "550"),
		status(2, "Agua", "2026-10-07", 1, 3, ""),
	}
	failing := true
	r.mailer.failOn = func(subject string) bool { return failing && strings.Contains(subject, "Luz") }

	r.tick()
	if n := r.mailer.count(); n != 1 || !strings.Contains(r.mailer.sent[0].subject, "Agua") {
		t.Fatalf("the failing email blocked the other: %+v", r.mailer.sent)
	}
	if _, ok := r.log.keys["bill_due:1"]; ok {
		t.Error("the key of a failed send was recorded")
	}
	if !strings.Contains(r.logs.String(), "bill_due:1") {
		t.Errorf("the failure was not logged: %s", r.logs.String())
	}

	failing = false
	r.advance(30 * time.Minute)
	r.tick()
	if n := r.mailer.count(); n != 2 {
		t.Fatalf("the failed email was not retried: %d sent", n)
	}
	r.tick()
	if n := r.mailer.count(); n != 2 {
		t.Fatalf("duplicates after the retry: %d sent", n)
	}
}

func TestVariableBillAndNilPending(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	noPending := status(2, "Fantasma", "2026-10-07", 1, 3, "10")
	noPending.Bill.Pending = nil
	r.bills.list = []billsapp.Status{status(1, "Agua", "2026-10-07", 1, 3, ""), noPending}

	r.tick()
	if n := r.mailer.count(); n != 1 {
		t.Fatalf("sent %d emails, want 1 (the bill without a pending occurrence is skipped)", n)
	}
	if !strings.Contains(r.mailer.sent[0].text, "monto variable") {
		t.Errorf("a variable bill must say so: %q", r.mailer.sent[0].text)
	}
}

func TestOverdueRepeatsEveryThreeDays(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico) // Tuesday
	overdue := func(days int) { r.bills.list = []billsapp.Status{status(5, "Renta", "2026-10-05", -days, 3, "9000")} }

	want := map[int]int{1: 1, 2: 1, 3: 1, 4: 2, 5: 2, 6: 2, 7: 3}
	for day := 1; day <= 7; day++ {
		overdue(day)
		r.tick()
		n := 0 // the Monday summary is not what this test counts
		for _, m := range r.mailer.sent {
			if strings.Contains(m.subject, "Pago vencido") {
				n++
			}
		}
		if n != want[day] {
			t.Fatalf("after %d days overdue: %d overdue emails, want %d", day, n, want[day])
		}
		r.advance(24 * time.Hour)
	}
	if !strings.Contains(r.mailer.sent[0].subject, "Pago vencido") {
		t.Errorf("subject = %q", r.mailer.sent[0].subject)
	}
}

func TestWeeklySummaryOnMondayOnly(t *testing.T) {
	r := newRig(t) // Monday
	r.bills.list = []billsapp.Status{
		status(1, "Luz", "2026-10-09", 4, 3, "550"), // outside its lead window, inside the week
		status(2, "Renta", "2026-10-03", -2, 3, "9000"),
		status(3, "Lejano", "2026-11-20", 46, 3, ""),
	}

	r.tick()
	r.tick()
	if n := r.mailer.count(); n != 2 { // overdue email + weekly summary, each once
		t.Fatalf("sent %d emails, want 2: %+v", n, r.mailer.sent)
	}
	var weekly *sentMail
	for i := range r.mailer.sent {
		if strings.Contains(r.mailer.sent[i].subject, "Resumen semanal") {
			weekly = &r.mailer.sent[i]
		}
	}
	if weekly == nil {
		t.Fatalf("no weekly summary: %+v", r.mailer.sent)
	}
	if !strings.Contains(weekly.text, "Luz") || !strings.Contains(weekly.text, "Renta") || strings.Contains(weekly.text, "Lejano") {
		t.Errorf("weekly text = %q", weekly.text)
	}
	if _, ok := r.log.keys["weekly:2026-10-05"]; !ok {
		t.Errorf("weekly key missing: %v", r.log.keys)
	}

	before := r.mailer.count()
	r.advance(24 * time.Hour) // Tuesday
	r.tick()
	for _, m := range r.mailer.sent[before:] {
		if strings.Contains(m.subject, "Resumen semanal") {
			t.Fatal("weekly summary sent on a Tuesday")
		}
	}
}

func TestWeeklySummarySkippedWhenEmpty(t *testing.T) {
	r := newRig(t)
	r.bills.list = []billsapp.Status{status(3, "Lejano", "2026-11-20", 46, 3, "")}
	r.tick()
	if n := r.mailer.count(); n != 0 {
		t.Fatalf("sent %d emails, want none", n)
	}
	if len(r.log.keys) != 0 {
		t.Errorf("keys recorded for nothing: %v", r.log.keys)
	}
}

func TestDiskAlert(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico) // Tuesday

	r.disk.used = 79.9
	r.tick()
	if n := r.mailer.count(); n != 0 {
		t.Fatalf("alert below the threshold: %d", n)
	}

	r.disk.used = 80
	r.tick()
	r.tick()
	if n := r.mailer.count(); n != 1 {
		t.Fatalf("sent %d alerts at the threshold, want 1", n)
	}
	if !strings.Contains(r.mailer.sent[0].subject, "80%") {
		t.Errorf("subject = %q", r.mailer.sent[0].subject)
	}
	if _, ok := r.log.keys["disk:2026-10-06"]; !ok {
		t.Errorf("disk key missing: %v", r.log.keys)
	}

	r.advance(6 * 24 * time.Hour)
	r.tick()
	if n := r.mailer.count(); n != 1 {
		t.Fatalf("re-alerted after 6 days: %d", n)
	}

	r.advance(24 * time.Hour)
	r.tick()
	if n := r.mailer.count(); n != 2 {
		t.Fatalf("no re-alert after 7 days: %d", n)
	}
}

func TestDiskProbeFailureAndDisabledProbe(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	r.disk.err = errors.New("statfs failed")
	r.tick()
	if n := r.mailer.count(); n != 0 {
		t.Fatalf("sent %d emails on a probe error", n)
	}
	if !strings.Contains(r.logs.String(), "could not measure the disk") {
		t.Errorf("probe failure not logged: %s", r.logs.String())
	}

	r.runner = app.NewRunner(app.Deps{
		Bills: r.bills, Mailer: r.mailer, Log: r.log, Owner: r.owner, DiskAlertPct: 80,
		Now: func() time.Time { return r.now }, Logger: slog.New(slog.NewTextHandler(r.logs, nil)),
	})
	r.tick() // nil Disk must not panic nor send
	if n := r.mailer.count(); n != 0 {
		t.Fatalf("sent %d emails without a probe", n)
	}
}

func TestRecordFailureAfterSendIsReported(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	r.bills.list = []billsapp.Status{status(1, "Luz", "2026-10-07", 1, 3, "550")}
	r.log.recordErr = errors.New("db down")

	r.tick()
	if n := r.mailer.count(); n != 1 {
		t.Fatalf("sent %d emails, want 1", n)
	}
	if !strings.Contains(r.logs.String(), "could not be recorded") {
		t.Errorf("record failure not reported: %s", r.logs.String())
	}
}

func TestSentLogFailureSendsNothing(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	r.bills.list = []billsapp.Status{status(1, "Luz", "2026-10-07", 1, 3, "550")}
	r.log.hasErr = errors.New("db down")
	r.tick()
	if n := r.mailer.count(); n != 0 {
		t.Fatalf("sent %d emails without knowing the log", n)
	}
}

func TestOwnerMissingOrFailing(t *testing.T) {
	for _, err := range []error{domain.ErrNoOwner, errors.New("db down")} {
		r := newRig(t)
		r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
		r.owner = fakeOwner{err: err}
		r.build()
		r.bills.list = []billsapp.Status{status(1, "Luz", "2026-10-07", 1, 3, "550")}
		r.tick()
		if n := r.mailer.count(); n != 0 || len(r.log.keys) != 0 {
			t.Fatalf("%v: sent %d emails, keys %v", err, n, r.log.keys)
		}
	}
}

func TestBillsFailureStillChecksTheDisk(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	r.bills.err = errors.New("db down")
	r.disk.used = 90
	r.tick()
	if n := r.mailer.count(); n != 1 || !strings.Contains(r.mailer.sent[0].subject, "disco") {
		t.Fatalf("sent %+v, want only the disk alert", r.mailer.sent)
	}
}

func TestRunTicksAndStopsOnCancel(t *testing.T) {
	r := newRig(t)
	r.now = time.Date(2026, 10, 6, 9, 0, 0, 0, mexico)
	r.bills.list = []billsapp.Status{status(1, "Luz", "2026-10-07", 1, 3, "550")}
	r.runner = app.NewRunner(app.Deps{
		Bills: r.bills, Mailer: r.mailer, Log: r.log, Owner: r.owner, DiskAlertPct: 80,
		Now: func() time.Time { return r.now }, Logger: slog.New(slog.NewTextHandler(r.logs, nil)),
		StartDelay: time.Millisecond, Interval: time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.runner.Run(ctx); close(done) }()

	deadline := time.After(2 * time.Second)
	for r.mailer.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("Run did not tick after the start delay")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}
