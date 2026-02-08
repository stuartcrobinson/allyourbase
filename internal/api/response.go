package api

import (
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ListResponse is the envelope for paginated list endpoints.
type ListResponse struct {
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
	Items      []map[string]any `json:"items"`
}

// Package-level aliases for the shared HTTP helpers so existing call sites
// within this package continue to compile without changes.
var (
	writeJSON       = httputil.WriteJSON
	writeError      = httputil.WriteError
	writeFieldError = httputil.WriteFieldError
)

// mapPGError converts a pgx/pgconn error to an appropriate HTTP response.
// Returns true if a PG error was handled.
func mapPGError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "record not found")
		return true
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "23505": // unique_violation
		writeFieldError(w, http.StatusConflict, "unique constraint violation",
			pgErr.ConstraintName, "unique_violation", pgErr.Detail)
	case "23503": // foreign_key_violation
		writeFieldError(w, http.StatusBadRequest, "foreign key violation",
			pgErr.ConstraintName, "foreign_key_violation", pgErr.Detail)
	case "23502": // not_null_violation
		writeFieldError(w, http.StatusBadRequest, "missing required value",
			pgErr.ColumnName, "not_null_violation", pgErr.Message)
	case "23514": // check_violation
		writeFieldError(w, http.StatusBadRequest, "check constraint violation",
			pgErr.ConstraintName, "check_violation", pgErr.Detail)
	case "22P02": // invalid_text_representation
		writeError(w, http.StatusBadRequest, "invalid value: "+pgErr.Message)
	default:
		return false
	}
	return true
}
