package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
)

// handleRPC handles POST /rpc/{function}
func (h *Handler) handleRPC(w http.ResponseWriter, r *http.Request) {
	fn := h.resolveFunction(w, r)
	if fn == nil {
		return
	}

	// Decode JSON body as named arguments (empty body = no args).
	var args map[string]any
	if r.ContentLength > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, httputil.MaxBodySize)
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	query, queryArgs, err := buildRPCCall(fn, args)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if fn.IsVoid {
		_, err := q.Exec(r.Context(), query, queryArgs...)
		if err != nil {
			done(err)
			if !mapPGError(w, err) {
				h.logger.Error("rpc error", "error", err, "function", fn.Name)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		done(nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	rows, err := q.Query(r.Context(), query, queryArgs...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("rpc error", "error", err, "function", fn.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	defer rows.Close()

	if fn.ReturnsSet {
		items, err := scanRows(rows)
		if err != nil {
			done(err)
			h.logger.Error("rpc scan error", "error", err, "function", fn.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		done(nil)
		writeJSON(w, http.StatusOK, items)
		return
	}

	// Scalar or single-row return.
	record, err := scanRow(rows)
	if err != nil {
		done(err)
		h.logger.Error("rpc scan error", "error", err, "function", fn.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	done(nil)

	if record == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	// If the result has a single column named after the function, unwrap it.
	if len(record) == 1 {
		for _, v := range record {
			writeJSON(w, http.StatusOK, v)
			return
		}
	}
	writeJSON(w, http.StatusOK, record)
}

// resolveFunction looks up the function in the schema cache and validates it exists.
func (h *Handler) resolveFunction(w http.ResponseWriter, r *http.Request) *schema.Function {
	sc := h.schema.Get()
	if sc == nil {
		writeError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return nil
	}

	funcName := chi.URLParam(r, "function")
	fn := sc.FunctionByName(funcName)
	if fn == nil {
		writeError(w, http.StatusNotFound, "function not found: "+funcName)
		return nil
	}
	return fn
}

// buildRPCCall generates the SQL and args for calling a function.
// For set-returning functions: SELECT * FROM schema.func($1, $2, ...)
// For scalar/void functions: SELECT schema.func($1, $2, ...)
func buildRPCCall(fn *schema.Function, args map[string]any) (string, []any, error) {
	var queryArgs []any
	placeholders := make([]string, len(fn.Parameters))

	for i, param := range fn.Parameters {
		val, ok := args[param.Name]
		if !ok {
			// If param has no name, try positional matching is not supported —
			// require named args for safety.
			if param.Name == "" {
				return "", nil, fmt.Errorf("function %q has unnamed parameters; cannot match by name", fn.Name)
			}
			// Missing param — pass NULL.
			val = nil
		}
		queryArgs = append(queryArgs, val)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	funcRef := quoteIdent(fn.Schema) + "." + quoteIdent(fn.Name)
	argList := strings.Join(placeholders, ", ")

	var query string
	if fn.ReturnsSet {
		query = fmt.Sprintf("SELECT * FROM %s(%s)", funcRef, argList)
	} else {
		query = fmt.Sprintf("SELECT %s(%s)", funcRef, argList)
	}

	return query, queryArgs, nil
}
