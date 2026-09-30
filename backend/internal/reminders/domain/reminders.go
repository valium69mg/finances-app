// Package domain holds the pure rules of the email reminders: which emails a
// set of bills calls for on a given day, the dedupe key of each one, when the
// disk alert fires, and the Spanish copy of the messages. Nothing here touches
// the clock, the database or the network.
package domain

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Schedule constants.
const (
	// SendFromHour is the local hour before which nothing is sent.
	SendFromHour = 8
	// OverdueRepeatDays is how often an overdue payment is repeated.
	OverdueRepeatDays = 3
	// WeeklySummaryDays is how far ahead the Monday summary looks.
	WeeklySummaryDays = 7
	// DiskRealertDays is how long the disk alert stays quiet after it fired.
	DiskRealertDays = 7
	// DiskKeyPrefix prefixes every disk alert key, so the log can be asked when
	// the last one was sent.
	DiskKeyPrefix = "disk:"

	dateLayout = "2006-01-02"
)

// ErrNoOwner is returned when there is no verified user to notify.
var ErrNoOwner = errors.New("no verified owner to notify")

// Kind is the type of a reminder email.
type Kind string

// Reminder kinds.
const (
	KindBillDue     Kind = "bill_due"
	KindBillOverdue Kind = "bill_overdue"
	KindWeekly      Kind = "weekly"

	KindTaxDueSoon    Kind = "tax_due_soon"
	KindTaxDueToday   Kind = "tax_due_today"
	KindTaxOverdue    Kind = "tax_overdue"
	KindSalaryInvoice Kind = "salary_invoice"
)

// Item is the pending occurrence of an active bill, as the reminders see it.
// Amount is a decimal string, empty for a variable bill. DaysUntilDue is
// negative when the occurrence is overdue.
type Item struct {
	OccurrenceID int
	Name         string
	Amount       string
	Currency     string
	DueDate      string // YYYY-MM-DD
	DaysUntilDue int
	DueSoon      bool
	Overdue      bool
}

// TaxPeriod is a period with issued invoices and no filing, as the reminders
// see it. DueDate is the filing deadline (YYYY-MM-DD).
type TaxPeriod struct {
	Period  string // YYYY-MM
	DueDate string // YYYY-MM-DD
}

// TaxItem is the tax period an email talks about. DaysUntilDue is negative when
// the deadline passed.
type TaxItem struct {
	Period       string
	DueDate      string
	DaysUntilDue int
}

// Tax reminder schedule.
const (
	// TaxDueSoonDays is how many days before the deadline the reminders start.
	TaxDueSoonDays = 5
	// TaxOverdueRepeatDays is how often an overdue filing is repeated.
	TaxOverdueRepeatDays = 3
)

// Email is an email the rules call for: its dedupe key and what it talks about
// (bills, a tax period or a salary month, depending on Kind). It is sent only
// when its key is not in the sent log yet.
type Email struct {
	Key   string
	Kind  Kind
	Items []Item
	// Tax is set for the tax filing kinds.
	Tax *TaxItem
	// Month (YYYY-MM) is set for KindSalaryInvoice.
	Month string
}

// TaxKeyDueSoon is the dedupe key of the "coming due" email of a period.
func TaxKeyDueSoon(period string) string { return "tax_due_soon:" + period }

// TaxKeyDueToday is the dedupe key of the due-day email of a period.
func TaxKeyDueToday(period string) string { return "tax_due_today:" + period }

// TaxKeyOverdue is the dedupe key of the overdue email of a period for the given
// number of days overdue (at least 1). The first overdue day is bucket 0 and the
// bucket advances every TaxOverdueRepeatDays days: day 1, 4, 7, ...
func TaxKeyOverdue(period string, daysOverdue int) string {
	return fmt.Sprintf("tax_overdue:%s:%d", period, (daysOverdue-1)/TaxOverdueRepeatDays)
}

// SalaryKey is the dedupe key of the salary invoice reminder of a YYYY-MM month.
func SalaryKey(month string) string { return "salary_invoice:" + month }

// TaxEmails returns the tax filing emails the pending periods call for on today
// (YYYY-MM-DD): one when the deadline is 1 to TaxDueSoonDays days away, one on
// the due day and, once overdue, one every TaxOverdueRepeatDays days (through
// its key). A period with an unparsable date is skipped. The caller drops the
// ones already sent.
func TaxEmails(pending []TaxPeriod, today string) []Email {
	var out []Email
	for _, p := range pending {
		days, ok := daysBetween(today, p.DueDate)
		if !ok {
			continue
		}
		item := &TaxItem{Period: p.Period, DueDate: p.DueDate, DaysUntilDue: days}
		switch {
		case days < 0:
			out = append(out, Email{Key: TaxKeyOverdue(p.Period, -days), Kind: KindTaxOverdue, Tax: item})
		case days == 0:
			out = append(out, Email{Key: TaxKeyDueToday(p.Period), Kind: KindTaxDueToday, Tax: item})
		case days <= TaxDueSoonDays:
			out = append(out, Email{Key: TaxKeyDueSoon(p.Period), Kind: KindTaxDueSoon, Tax: item})
		}
	}
	return out
}

// SalaryMonth returns the YYYY-MM month of today (YYYY-MM-DD) when today is its
// last calendar day, which is the only day the salary invoice reminder fires.
func SalaryMonth(today string) (string, bool) {
	day, err := time.Parse(dateLayout, today)
	if err != nil || day.AddDate(0, 0, 1).Month() == day.Month() {
		return "", false
	}
	return day.Format("2006-01"), true
}

// SalaryEmail returns the salary invoice reminder of a month, or false when an
// invoice of that month is already issued.
func SalaryEmail(month string, alreadyIssued bool) (Email, bool) {
	if alreadyIssued {
		return Email{}, false
	}
	return Email{Key: SalaryKey(month), Kind: KindSalaryInvoice, Month: month}, true
}

// daysBetween is the number of calendar days from a to b (negative when b is
// earlier), both YYYY-MM-DD.
func daysBetween(a, b string) (int, bool) {
	from, err1 := time.Parse(dateLayout, a)
	to, err2 := time.Parse(dateLayout, b)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return int(to.Sub(from).Hours() / 24), true
}

// BillKeyDue is the dedupe key of the "coming due" email of an occurrence.
func BillKeyDue(occurrenceID int) string { return fmt.Sprintf("bill_due:%d", occurrenceID) }

// BillKeyOverdue is the dedupe key of the overdue email of an occurrence for the
// given number of days overdue (at least 1). The first overdue day is bucket 0
// and the bucket advances every OverdueRepeatDays days, so the email repeats
// every 3 days: day 1, 4, 7, ...
func BillKeyOverdue(occurrenceID, daysOverdue int) string {
	return fmt.Sprintf("bill_overdue:%d:%d", occurrenceID, (daysOverdue-1)/OverdueRepeatDays)
}

// WeeklyKey is the dedupe key of the summary of the week starting on monday.
func WeeklyKey(monday string) string { return "weekly:" + monday }

// DiskKey is the dedupe key of a disk alert sent on the given day.
func DiskKey(today string) string { return DiskKeyPrefix + today }

// BillEmails returns the per-payment emails the items call for today: one
// "coming due" email for an item inside its reminder window and one overdue
// email (repeated every OverdueRepeatDays days, through its key) for an overdue
// item. The caller drops the ones already sent.
func BillEmails(items []Item) []Email {
	var out []Email
	for _, it := range items {
		switch {
		case it.Overdue:
			out = append(out, Email{Key: BillKeyOverdue(it.OccurrenceID, -it.DaysUntilDue), Kind: KindBillOverdue, Items: []Item{it}})
		case it.DueSoon:
			out = append(out, Email{Key: BillKeyDue(it.OccurrenceID), Kind: KindBillDue, Items: []Item{it}})
		}
	}
	return out
}

// WeeklyEmail returns the Monday summary for today (YYYY-MM-DD): the items due
// within the next WeeklySummaryDays days plus the overdue ones, earliest due
// date first. It reports false when today is not a Monday or there is nothing
// to list.
func WeeklyEmail(items []Item, today string) (Email, bool) {
	day, err := time.Parse(dateLayout, today)
	if err != nil || day.Weekday() != time.Monday {
		return Email{}, false
	}
	var listed []Item
	for _, it := range items {
		if it.Overdue || (it.DaysUntilDue >= 0 && it.DaysUntilDue <= WeeklySummaryDays) {
			listed = append(listed, it)
		}
	}
	if len(listed) == 0 {
		return Email{}, false
	}
	sort.SliceStable(listed, func(i, j int) bool { return listed[i].DueDate < listed[j].DueDate })
	return Email{Key: WeeklyKey(today), Kind: KindWeekly, Items: listed}, true
}

// DiskAlertDue reports whether the disk alert must be sent: the used percentage
// reached the threshold and no disk alert went out in the last DiskRealertDays
// days (lastSent is nil when none ever did).
func DiskAlertDue(usedPct float64, thresholdPct int, lastSent *time.Time, now time.Time) bool {
	if usedPct < float64(thresholdPct) {
		return false
	}
	return lastSent == nil || now.Sub(*lastSent) >= DiskRealertDays*24*time.Hour
}
