package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/reminders/domain"
)

func TestBillEmails(t *testing.T) {
	tests := []struct {
		name     string
		item     domain.Item
		wantKey  string
		wantKind domain.Kind
	}{
		{"exactly at the lead days", domain.Item{OccurrenceID: 7, DaysUntilDue: 3, DueSoon: true}, "bill_due:7", domain.KindBillDue},
		{"day of", domain.Item{OccurrenceID: 7, DaysUntilDue: 0, DueSoon: true}, "bill_due:7", domain.KindBillDue},
		{"first overdue day is bucket 0", domain.Item{OccurrenceID: 7, DaysUntilDue: -1, Overdue: true}, "bill_overdue:7:0", domain.KindBillOverdue},
		{"third overdue day still bucket 0", domain.Item{OccurrenceID: 7, DaysUntilDue: -3, Overdue: true}, "bill_overdue:7:0", domain.KindBillOverdue},
		{"fourth overdue day repeats", domain.Item{OccurrenceID: 7, DaysUntilDue: -4, Overdue: true}, "bill_overdue:7:1", domain.KindBillOverdue},
		{"seventh overdue day repeats again", domain.Item{OccurrenceID: 7, DaysUntilDue: -7, Overdue: true}, "bill_overdue:7:2", domain.KindBillOverdue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.BillEmails([]domain.Item{tt.item})
			if len(got) != 1 || got[0].Key != tt.wantKey || got[0].Kind != tt.wantKind {
				t.Fatalf("got %+v, want one %s email with key %s", got, tt.wantKind, tt.wantKey)
			}
		})
	}
}

func TestBillEmailsSkipsItemsOutsideTheWindow(t *testing.T) {
	got := domain.BillEmails([]domain.Item{
		{OccurrenceID: 1, DaysUntilDue: 4}, // before the window opens
		{OccurrenceID: 2, DaysUntilDue: 30, DueSoon: false},
	})
	if len(got) != 0 {
		t.Fatalf("got %+v, want no email", got)
	}
}

func TestTaxEmails(t *testing.T) {
	const due = "2026-10-17"
	tests := []struct {
		name     string
		today    string
		wantKey  string // empty: no email
		wantKind domain.Kind
		wantDays int
	}{
		{"6 days before sends nothing", "2026-10-11", "", "", 0},
		{"exactly 5 days before", "2026-10-12", "tax_due_soon:2026-09", domain.KindTaxDueSoon, 5},
		{"1 day before", "2026-10-16", "tax_due_soon:2026-09", domain.KindTaxDueSoon, 1},
		{"due day", "2026-10-17", "tax_due_today:2026-09", domain.KindTaxDueToday, 0},
		{"day after due is bucket 0", "2026-10-18", "tax_overdue:2026-09:0", domain.KindTaxOverdue, -1},
		{"third overdue day still bucket 0", "2026-10-20", "tax_overdue:2026-09:0", domain.KindTaxOverdue, -3},
		{"fourth overdue day is bucket 1", "2026-10-21", "tax_overdue:2026-09:1", domain.KindTaxOverdue, -4},
		{"10th overdue day is bucket 3", "2026-10-27", "tax_overdue:2026-09:3", domain.KindTaxOverdue, -10},
		{"month boundary before the due date", "2026-09-30", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.TaxEmails([]domain.TaxPeriod{{Period: "2026-09", DueDate: due}}, tt.today)
			if tt.wantKey == "" {
				if len(got) != 0 {
					t.Fatalf("got %+v, want no email", got)
				}
				return
			}
			if len(got) != 1 || got[0].Key != tt.wantKey || got[0].Kind != tt.wantKind || got[0].Tax.DaysUntilDue != tt.wantDays {
				t.Fatalf("got %+v, want one %s email with key %s and %d days", got, tt.wantKind, tt.wantKey, tt.wantDays)
			}
		})
	}
}

func TestTaxEmailsSkipsBadDatesAndHandlesManyPeriods(t *testing.T) {
	got := domain.TaxEmails([]domain.TaxPeriod{
		{Period: "2026-07", DueDate: "2026-08-17"},
		{Period: "bad", DueDate: "nope"},
		{Period: "2026-09", DueDate: "2026-10-17"},
	}, "2026-10-17")
	if len(got) != 2 || got[0].Key != "tax_overdue:2026-07:20" || got[1].Key != "tax_due_today:2026-09" {
		t.Fatalf("got %+v", got)
	}
	if len(domain.TaxEmails(nil, "2026-10-17")) != 0 || len(domain.TaxEmails([]domain.TaxPeriod{{Period: "2026-09", DueDate: "2026-10-17"}}, "garbage")) != 0 {
		t.Error("nothing pending or an invalid today must not send")
	}
}

func TestSalaryMonth(t *testing.T) {
	tests := []struct {
		today, want string
	}{
		{"2026-09-30", "2026-09"}, // 30-day month
		{"2026-09-29", ""},        //
		{"2026-10-30", ""},        // 31-day month: the 30th is not the last day
		{"2026-10-31", "2026-10"}, //
		{"2026-02-28", "2026-02"}, // common year
		{"2028-02-28", ""},        // leap year: the 28th is not the last day
		{"2028-02-29", "2028-02"}, //
		{"2026-12-31", "2026-12"}, // year end
		{"garbage", ""},
	}
	for _, tt := range tests {
		got, ok := domain.SalaryMonth(tt.today)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("SalaryMonth(%q) = %q, %v; want %q", tt.today, got, ok, tt.want)
		}
	}
}

func TestSalaryEmail(t *testing.T) {
	e, ok := domain.SalaryEmail("2026-09", false)
	if !ok || e.Key != "salary_invoice:2026-09" || e.Kind != domain.KindSalaryInvoice || e.Month != "2026-09" {
		t.Fatalf("got %+v, %v", e, ok)
	}
	if _, ok := domain.SalaryEmail("2026-09", true); ok {
		t.Error("an already issued month must not be reminded")
	}
}

func TestComposeTax(t *testing.T) {
	tests := []struct {
		kind    domain.Kind
		days    int
		subject string
		want    []string
	}{
		{domain.KindTaxDueSoon, 5, "Declaración próxima a vencer: Septiembre 2026", []string{"17/10/2026", "en 5 días"}},
		{domain.KindTaxDueToday, 0, "Declaración vence hoy: Septiembre 2026", []string{"17/10/2026", "hoy"}},
		{domain.KindTaxOverdue, -4, "Declaración vencida: Septiembre 2026", []string{"17/10/2026", "venció hace 4 días"}},
	}
	for _, tt := range tests {
		e := domain.Email{Kind: tt.kind, Tax: &domain.TaxItem{Period: "2026-09", DueDate: "2026-10-17", DaysUntilDue: tt.days}}
		m := domain.Compose(e, "https://app.example.com")
		if m.Subject != tt.subject {
			t.Errorf("subject = %q, want %q", m.Subject, tt.subject)
		}
		for _, w := range append(tt.want, "Septiembre 2026", "https://app.example.com") {
			if !strings.Contains(m.Text, w) || !strings.Contains(m.HTML, w) {
				t.Errorf("%s: missing %q in %q / %q", tt.kind, w, m.Text, m.HTML)
			}
		}
	}
}

func TestComposeSalaryEscapesLink(t *testing.T) {
	m := domain.Compose(domain.Email{Kind: domain.KindSalaryInvoice, Month: "2028-02"}, `https://app/?a=1&b="2"`)
	if m.Subject != "Emite la factura de tu sueldo de Febrero 2028" {
		t.Errorf("subject = %q", m.Subject)
	}
	if !strings.Contains(m.Text, "Febrero 2028") || !strings.Contains(m.HTML, "&amp;b=&#34;2&#34;") {
		t.Errorf("text %q html %q", m.Text, m.HTML)
	}
}

func TestWeeklyEmail(t *testing.T) {
	const monday, tuesday = "2026-10-05", "2026-10-06"
	items := []domain.Item{
		{OccurrenceID: 1, Name: "Later", DueDate: "2026-10-12", DaysUntilDue: 7},
		{OccurrenceID: 2, Name: "Overdue", DueDate: "2026-10-01", DaysUntilDue: -4, Overdue: true},
		{OccurrenceID: 3, Name: "Too far", DueDate: "2026-10-13", DaysUntilDue: 8},
		{OccurrenceID: 4, Name: "Today", DueDate: "2026-10-05", DaysUntilDue: 0},
	}

	got, ok := domain.WeeklyEmail(items, monday)
	if !ok {
		t.Fatal("expected a summary on a Monday")
	}
	if got.Key != "weekly:2026-10-05" || got.Kind != domain.KindWeekly {
		t.Errorf("key/kind = %s/%s", got.Key, got.Kind)
	}
	var names []string
	for _, it := range got.Items {
		names = append(names, it.Name)
	}
	if strings.Join(names, ",") != "Overdue,Today,Later" {
		t.Errorf("items = %v, want overdue, today and day 7 by due date (day 8 excluded)", names)
	}

	if _, ok := domain.WeeklyEmail(items, tuesday); ok {
		t.Error("a summary was built on a Tuesday")
	}
	if _, ok := domain.WeeklyEmail(items[2:3], monday); ok {
		t.Error("an empty summary must be skipped")
	}
	if _, ok := domain.WeeklyEmail(items, "garbage"); ok {
		t.Error("an invalid date must not build a summary")
	}
}

func TestDiskAlertDue(t *testing.T) {
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	days := func(n int) *time.Time { v := now.Add(-time.Duration(n) * 24 * time.Hour); return &v }
	recent := now.Add(-7*24*time.Hour + time.Minute)

	tests := []struct {
		name string
		used float64
		last *time.Time
		want bool
	}{
		{"below the threshold", 79.9, nil, false},
		{"exactly at the threshold", 80, nil, true},
		{"above the threshold", 93.5, nil, true},
		{"above but alerted yesterday", 90, days(1), false},
		{"above but alerted just under 7 days ago", 90, &recent, false},
		{"above and alerted exactly 7 days ago", 90, days(7), true},
		{"below even after a long time", 50, days(30), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.DiskAlertDue(tt.used, 80, tt.last, now); got != tt.want {
				t.Errorf("DiskAlertDue = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComposeBill(t *testing.T) {
	it := domain.Item{
		OccurrenceID: 1, Name: `Luz <b>&"CFE"`, Amount: "550.00", Currency: "MXN", DueDate: "2026-10-05", DaysUntilDue: 3, DueSoon: true,
	}
	m := domain.Compose(domain.Email{Kind: domain.KindBillDue, Items: []domain.Item{it}}, "https://app.example.com")

	if !strings.Contains(m.Subject, "Pago próximo") || !strings.Contains(m.Subject, "Luz") {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"550.00 MXN", "05/10/2026", "en 3 días", "https://app.example.com"} {
		if !strings.Contains(m.Text, want) || !strings.Contains(m.HTML, want) {
			t.Errorf("missing %q in text %q or html %q", want, m.Text, m.HTML)
		}
	}
	if strings.Contains(m.HTML, "<b>") || !strings.Contains(m.HTML, "&lt;b&gt;&amp;&#34;CFE&#34;") {
		t.Errorf("the bill name must be HTML-escaped: %q", m.HTML)
	}
}

func TestComposeVariableAmountAndOverdue(t *testing.T) {
	it := domain.Item{OccurrenceID: 1, Name: "Agua", DueDate: "2026-10-01", DaysUntilDue: -2, Overdue: true}
	m := domain.Compose(domain.Email{Kind: domain.KindBillOverdue, Items: []domain.Item{it}}, "https://app")
	if !strings.Contains(m.Subject, "Pago vencido") {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"monto variable", "venció hace 2 días", "01/10/2026"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("text lacks %q: %q", want, m.Text)
		}
	}
}

func TestComposeWeekly(t *testing.T) {
	items := []domain.Item{
		{Name: "Vencido", DueDate: "2026-10-01", DaysUntilDue: -4, Overdue: true, Amount: "10.00", Currency: "USD"},
		{Name: "Hoy", DueDate: "2026-10-05", DaysUntilDue: 0},
		{Name: "Mañana", DueDate: "2026-10-06", DaysUntilDue: 1},
	}
	m := domain.Compose(domain.Email{Kind: domain.KindWeekly, Items: items}, "https://app")
	if m.Subject != "Resumen semanal de pagos" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"Vencidos", "Próximos 7 días", "hoy", "mañana", "10.00 USD"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("text lacks %q: %q", want, m.Text)
		}
	}
}

func TestDiskMessage(t *testing.T) {
	m := domain.DiskMessage(83.4, 80)
	if !strings.Contains(m.Subject, "83%") || !strings.Contains(m.Text, "80%") || !strings.Contains(m.HTML, "83%") {
		t.Errorf("unexpected message %+v", m)
	}
}
