package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

// The wishlist of a household user lives in personal_wishlist_items. The three
// links (future expense, Gasto, expense request) are read with the current
// state of the request, which is all the status derivation needs. Every query
// is scoped by user_id.

const wishColumns = `w.id, w.name, w.price::text, w.category_id, c.name, w.store_url, w.priority, w.note,
	w.future_expense_id, w.expense_movement_id, w.expense_request_id, r.status, w.created_at, w.updated_at`

const wishFrom = `personal_wishlist_items w
	JOIN personal_categories c ON c.id = w.category_id
	LEFT JOIN expense_requests r ON r.id = w.expense_request_id`

func scanWish(row pgx.Row) (domain.WishItem, error) {
	var (
		it                      domain.WishItem
		id, categoryID          int64
		price, priority         string
		storeURL, note          *string
		futureID, moveID, reqID *int64
		reqStatus               *string
		createdAt, updatedAt    time.Time
	)
	if err := row.Scan(&id, &it.Name, &price, &categoryID, &it.CategoryName, &storeURL, &priority, &note,
		&futureID, &moveID, &reqID, &reqStatus, &createdAt, &updatedAt); err != nil {
		return domain.WishItem{}, err
	}
	it.ID, it.CategoryID, it.Priority, it.CreatedAt, it.UpdatedAt = int(id), int(categoryID), domain.Priority(priority), createdAt, updatedAt
	var err error
	if it.Price, err = decimal.NewFromString(price); err != nil {
		return domain.WishItem{}, fmt.Errorf("parse price: %w", err)
	}
	if storeURL != nil {
		it.StoreURL = *storeURL
	}
	if note != nil {
		it.Note = *note
	}
	switch {
	case futureID != nil:
		it.Link = &domain.Conversion{Type: domain.ConvertedFuture, TargetID: int(*futureID)}
	case moveID != nil:
		it.Link = &domain.Conversion{Type: domain.ConvertedExpense, TargetID: int(*moveID)}
	case reqID != nil:
		it.Link = &domain.Conversion{Type: domain.ConvertedRequest, TargetID: int(*reqID)}
		if reqStatus != nil {
			it.Link.RequestState = *reqStatus
		}
	}
	return it, nil
}

// nullable maps an empty string to NULL.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// mapWishError turns the constraint violations of a write into the domain
// errors: the composite foreign key rejects a category of another user or of a
// kind other than Gasto.
func mapWishError(err error) error {
	switch pgCode(err) {
	case codeForeignKey:
		return fmt.Errorf("%w: category does not match the item", domain.ErrWishInvalid)
	case codeCheck:
		return fmt.Errorf("%w: a value is not allowed", domain.ErrWishInvalid)
	}
	return err
}

// CreateWish stores an active item (no link).
func (r *Repo) CreateWish(ctx context.Context, userID string, v domain.ValidWish) (domain.WishItem, error) {
	it, err := scanWish(r.pool.QueryRow(ctx, `
		WITH w AS (
			INSERT INTO personal_wishlist_items (user_id, name, price, category_id, store_url, priority, note)
			VALUES ($1::text::uuid, $2, $3::text::numeric, $4, $5, $6, $7) RETURNING *)
		SELECT `+wishColumns+` FROM w
			JOIN personal_categories c ON c.id = w.category_id
			LEFT JOIN expense_requests r ON r.id = w.expense_request_id`,
		userID, v.Name, v.Price.String(), int64(v.CategoryID), nullable(v.StoreURL), string(v.Priority), nullable(v.Note)))
	if err != nil {
		return domain.WishItem{}, mapWishError(err)
	}
	return it, nil
}

// GetWish returns one item of the user.
func (r *Repo) GetWish(ctx context.Context, userID string, id int) (domain.WishItem, error) {
	return getWish(ctx, r.pool, userID, id, "")
}

func getWish(ctx context.Context, q rowQuerier, userID string, id int, lock string) (domain.WishItem, error) {
	it, err := scanWish(q.QueryRow(ctx, `
		SELECT `+wishColumns+` FROM `+wishFrom+`
		WHERE w.id = $1 AND w.user_id = $2::text::uuid `+lock, int64(id), userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WishItem{}, domain.ErrNotFound
	}
	return it, err
}

// ListWish returns every item of the user, newest first.
func (r *Repo) ListWish(ctx context.Context, userID string) ([]domain.WishItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+wishColumns+` FROM `+wishFrom+`
		WHERE w.user_id = $1::text::uuid ORDER BY w.created_at DESC, w.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WishItem{}
	for rows.Next() {
		it, err := scanWish(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpdateWish replaces the fields of an item. The links are not in the statement.
func (r *Repo) UpdateWish(ctx context.Context, userID string, id int, v domain.ValidWish) (domain.WishItem, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE personal_wishlist_items
		SET name = $3, price = $4::text::numeric, category_id = $5, store_url = $6, priority = $7, note = $8, updated_at = now()
		WHERE id = $1 AND user_id = $2::text::uuid`,
		int64(id), userID, v.Name, v.Price.String(), int64(v.CategoryID), nullable(v.StoreURL), string(v.Priority), nullable(v.Note))
	if err != nil {
		return domain.WishItem{}, mapWishError(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.WishItem{}, domain.ErrNotFound
	}
	return r.GetWish(ctx, userID, id)
}

// DeleteWish removes the item only.
func (r *Repo) DeleteWish(ctx context.Context, userID string, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM personal_wishlist_items WHERE id = $1 AND user_id = $2::text::uuid`, int64(id), userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// lockActiveWish locks the item and refuses a converted one. A concurrent
// conversion of the same item waits on the lock and then finds it converted.
func lockActiveWish(ctx context.Context, tx pgx.Tx, userID string, id int) (domain.WishItem, error) {
	it, err := getWish(ctx, tx, userID, id, "FOR UPDATE OF w")
	if err != nil {
		return domain.WishItem{}, err
	}
	if it.Status() != domain.WishActive {
		return domain.WishItem{}, domain.ErrAlreadyConverted
	}
	return it, nil
}

// setLink replaces the link of the item: exactly one of the three is set.
func setLink(ctx context.Context, tx pgx.Tx, userID string, id int, future, movement, request *int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE personal_wishlist_items
		SET future_expense_id = $3, expense_movement_id = $4, expense_request_id = $5, updated_at = now()
		WHERE id = $1 AND user_id = $2::text::uuid`, int64(id), userID, future, movement, request)
	return err
}

// ConvertWishToFuture creates her future expense and links the item, atomically.
func (r *Repo) ConvertWishToFuture(ctx context.Context, userID string, id int, plan func(it domain.WishItem) (domain.FutureValidated, error)) (domain.WishItem, error) {
	return r.convert(ctx, userID, id, func(tx pgx.Tx, it domain.WishItem) error {
		v, err := plan(it)
		if err != nil {
			return err
		}
		f, err := insertFuture(ctx, tx, userID, v)
		if err != nil {
			return fmt.Errorf("create the future expense: %w", err)
		}
		fid := int64(f.ID)
		return setLink(ctx, tx, userID, id, &fid, nil, nil)
	})
}

// ConvertWishToExpense registers her Gasto and links the item, atomically.
func (r *Repo) ConvertWishToExpense(ctx context.Context, userID string, id int, plan func(it domain.WishItem) (domain.ValidMovement, error)) (domain.WishItem, error) {
	return r.convert(ctx, userID, id, func(tx pgx.Tx, it domain.WishItem) error {
		v, err := plan(it)
		if err != nil {
			return err
		}
		m, err := insertMovement(ctx, tx, userID, v, nil)
		if err != nil {
			return fmt.Errorf("register the expense: %w", err)
		}
		mid := int64(m.ID)
		return setLink(ctx, tx, userID, id, nil, &mid, nil)
	})
}

// LinkWishRequest links the item to a request that was created elsewhere.
func (r *Repo) LinkWishRequest(ctx context.Context, userID string, id, requestID int) (domain.WishItem, error) {
	return r.convert(ctx, userID, id, func(tx pgx.Tx, _ domain.WishItem) error {
		rid := int64(requestID)
		return setLink(ctx, tx, userID, id, nil, nil, &rid)
	})
}

// convert runs one conversion in a transaction: lock the active item, apply
// the writes, read the item back, commit.
func (r *Repo) convert(ctx context.Context, userID string, id int, apply func(tx pgx.Tx, it domain.WishItem) error) (domain.WishItem, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.WishItem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	it, err := lockActiveWish(ctx, tx, userID, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	if err := apply(tx, it); err != nil {
		return domain.WishItem{}, err
	}
	out, err := getWish(ctx, tx, userID, id, "")
	if err != nil {
		return domain.WishItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.WishItem{}, err
	}
	return out, nil
}
