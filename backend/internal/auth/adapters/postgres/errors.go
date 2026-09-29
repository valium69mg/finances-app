package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isInvalidText reports a PostgreSQL invalid_text_representation error (22P02),
// raised for example when a malformed uuid is cast.
func isInvalidText(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}
