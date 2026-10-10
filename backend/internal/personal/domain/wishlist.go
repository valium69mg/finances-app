package domain

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// The wishlist of a household user: things she wants, with an estimated price,
// a category of hers and a priority. An active item converts into her future
// expense, her Gasto or an expense request to the owner; the item then links to
// what it became. The status is DERIVED from those links at read time (never
// stored), so deleting the converted row, or a rejected or cancelled request,
// reactivates the item with no extra write.

// Errors of the wishlist.
var (
	// ErrWishInvalid wraps every validation failure of an item or a conversion,
	// including a category that is not hers (400 invalid_wishlist_item).
	ErrWishInvalid = errors.New("invalid wishlist item")
	// ErrAlreadyConverted is returned when a conversion targets an item that is
	// already converted (409 already_converted).
	ErrAlreadyConverted = errors.New("the wishlist item is already converted")
)

func wishInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrWishInvalid, fmt.Sprintf(format, args...))
}

// Wishlist limits.
const (
	MaxWishNameLength = 120
	MaxWishURLLength  = 500
	MaxWishNoteLength = 500
)

// Priority of an item.
type Priority string

const (
	PriorityHigh   Priority = "alta"
	PriorityMedium Priority = "media"
	PriorityLow    Priority = "baja"
)

// DefaultPriority is used when the priority is empty.
const DefaultPriority = PriorityMedium

func (p Priority) rank() int {
	switch p {
	case PriorityHigh:
		return 0
	case PriorityMedium:
		return 1
	default:
		return 2
	}
}

// ParsePriority checks a priority; an empty one is the default (media).
func ParsePriority(s string) (Priority, error) {
	p := Priority(strings.TrimSpace(s))
	switch p {
	case "":
		return DefaultPriority, nil
	case PriorityHigh, PriorityMedium, PriorityLow:
		return p, nil
	}
	return "", wishInvalid("priority must be alta, media or baja")
}

// WishStatus is the derived state of an item.
type WishStatus string

const (
	WishActive    WishStatus = "activa"
	WishConverted WishStatus = "convertida"
)

// ConversionType is what an item became.
type ConversionType string

const (
	ConvertedFuture  ConversionType = "gasto_futuro"
	ConvertedExpense ConversionType = "gasto"
	ConvertedRequest ConversionType = "peticion"
)

// Conversion is the link of an item: what it became, the id of the target and,
// for a request, its current state (solicitada, aprobada, rechazada or
// cancelada).
type Conversion struct {
	Type         ConversionType
	TargetID     int
	RequestState string
}

// WishItem is one item of the wishlist with its (at most one) link.
type WishItem struct {
	ID           int
	Name         string
	Price        decimal.Decimal
	CategoryID   int
	CategoryName string
	StoreURL     string // empty when none
	Priority     Priority
	Note         string // empty when none
	Link         *Conversion
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Status derives the state: converted while the link points at a future
// expense, a Gasto or a request that is solicitada or aprobada; active
// otherwise (no link, or the request was rechazada or cancelada).
func (w WishItem) Status() WishStatus {
	if w.Link == nil {
		return WishActive
	}
	if w.Link.Type == ConvertedRequest && w.Link.RequestState != "solicitada" && w.Link.RequestState != "aprobada" {
		return WishActive
	}
	return WishConverted
}

// SortWishlist orders items: the active ones first (priority alta to baja, then
// newest first), then the converted ones (newest first).
func SortWishlist(items []WishItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		ac, bc := a.Status() == WishConverted, b.Status() == WishConverted
		if ac != bc {
			return !ac
		}
		if !ac && a.Priority.rank() != b.Priority.rank() {
			return a.Priority.rank() < b.Priority.rank()
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ID > b.ID
	})
}

// WishInput is the raw data of a new or updated item.
type WishInput struct {
	Name       string
	Price      decimal.Decimal
	CategoryID int
	StoreURL   string
	Priority   string
	Note       string
}

// ValidWish is a WishInput after validation.
type ValidWish struct {
	Name       string
	Price      decimal.Decimal
	CategoryID int
	StoreURL   string
	Priority   Priority
	Note       string
}

// ValidateWishPrice checks a price or an amount of a conversion: in range
// first, positive, at most 2 decimals (MXN).
func ValidateWishPrice(field string, v decimal.Decimal) error {
	if err := ValidateAmount(KindExpense, v); err != nil {
		return wishInvalid("%s: %s", field, strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": "))
	}
	return nil
}

// ValidateStoreURL checks an optional store link: http or https with a host.
func ValidateStoreURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if utf8.RuneCountInString(raw) > MaxWishURLLength {
		return "", wishInvalid("store_url must be at most %d characters", MaxWishURLLength)
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return "", wishInvalid("store_url must be a valid http or https link")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", wishInvalid("store_url must be a valid http or https link")
	}
	return raw, nil
}

// ValidateWish checks the input of a create or an update. Whether the category
// is hers and a Gasto one is checked by the service against her categories.
func ValidateWish(in WishInput) (ValidWish, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return ValidWish{}, wishInvalid("name is required")
	case utf8.RuneCountInString(name) > MaxWishNameLength:
		return ValidWish{}, wishInvalid("name must be at most %d characters", MaxWishNameLength)
	}
	if err := ValidateWishPrice("price", in.Price); err != nil {
		return ValidWish{}, err
	}
	if in.CategoryID <= 0 {
		return ValidWish{}, wishInvalid("category_id is required")
	}
	storeURL, err := ValidateStoreURL(in.StoreURL)
	if err != nil {
		return ValidWish{}, err
	}
	priority, err := ParsePriority(in.Priority)
	if err != nil {
		return ValidWish{}, err
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > MaxWishNoteLength {
		return ValidWish{}, wishInvalid("note must be at most %d characters", MaxWishNoteLength)
	}
	return ValidWish{Name: name, Price: in.Price, CategoryID: in.CategoryID, StoreURL: storeURL, Priority: priority, Note: note}, nil
}
