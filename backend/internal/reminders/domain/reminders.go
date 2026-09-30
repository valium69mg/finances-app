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

// Email is an email the rules call for: its dedupe key and the bills it talks
// about. It is sent only when its key is not in the sent log yet.
type Email struct {
	Key   string
	Kind  Kind
	Items []Item
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
