package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// testSchema creates a minimal schema cache with a "users" table for testing.
func testSchema() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid"},
					{Name: "email", TypeName: "text"},
					{Name: "name", TypeName: "text", IsNullable: true},
				},
				PrimaryKey: []string{"id"},
			},
			"public.logs": {
				Schema:  "public",
				Name:    "logs",
				Kind:    "view",
				Columns: []*schema.Column{{Name: "id", TypeName: "integer"}, {Name: "message", TypeName: "text"}},
			},
			"public.nopk": {
				Schema:  "public",
				Name:    "nopk",
				Kind:    "table",
				Columns: []*schema.Column{{Name: "data", TypeName: "text"}},
			},
		},
		Schemas: []string{"public"},
	}
}

func testCacheHolder(sc *schema.SchemaCache) *schema.CacheHolder {
	ch := schema.NewCacheHolder(nil, slog.Default())
	if sc != nil {
		ch.SetForTesting(sc)
	}
	return ch
}

func testHandler(sc *schema.SchemaCache) http.Handler {
	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, slog.Default(), nil)
	return h.Routes()
}

func doRequest(handler http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) httputil.ErrorResponse {
	t.Helper()
	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// --- Schema not ready ---

func TestListSchemaCacheNotReady(t *testing.T) {
	h := testHandler(nil)
	w := doRequest(h, "GET", "/collections/users", "")
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "schema cache not ready")
}

func TestReadSchemaCacheNotReady(t *testing.T) {
	h := testHandler(nil)
	w := doRequest(h, "GET", "/collections/users/123", "")
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// --- Collection not found ---

func TestListCollectionNotFound(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "collection not found")
}

func TestReadCollectionNotFound(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent/123", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateCollectionNotFound(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/nonexistent", `{"name":"test"}`)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// --- Write on view ---

func TestCreateOnViewNotAllowed(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/logs", `{"message":"test"}`)
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations not allowed")
}

func TestUpdateOnViewNotAllowed(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/logs/1", `{"message":"test"}`)
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestDeleteOnViewNotAllowed(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "DELETE", "/collections/logs/1", "")
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// --- No primary key ---

func TestReadNoPrimaryKey(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nopk/1", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no primary key")
}

func TestUpdateNoPrimaryKey(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/nopk/1", `{"data":"test"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteNoPrimaryKey(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "DELETE", "/collections/nopk/1", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Invalid body ---

func TestCreateEmptyBody(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users", `{}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "empty request body")
}

func TestCreateInvalidJSON(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users", `{invalid`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid JSON body")
}

func TestCreateNoRecognizedColumns(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users", `{"unknown_field":"value"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no recognized columns")
}

func TestUpdateEmptyBody(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/users/123", `{}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateInvalidJSON(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/users/123", `not-json`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Invalid filter ---

func TestListInvalidFilter(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?filter=((broken", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid filter")
}

// --- parseFields ---

func TestParseFieldsEmpty(t *testing.T) {
	r := httptest.NewRequest("GET", "/?fields=", nil)
	fields := parseFields(r)
	testutil.True(t, fields == nil, "expected nil for empty fields")
}

func TestParseFieldsMultiple(t *testing.T) {
	r := httptest.NewRequest("GET", "/?fields=id,email,name", nil)
	fields := parseFields(r)
	testutil.Equal(t, 3, len(fields))
	testutil.Equal(t, "id", fields[0])
	testutil.Equal(t, "email", fields[1])
	testutil.Equal(t, "name", fields[2])
}

func TestParseFieldsTrimsSpaces(t *testing.T) {
	r := httptest.NewRequest("GET", "/?fields=+id+,+name+", nil)
	fields := parseFields(r)
	testutil.Equal(t, 2, len(fields))
}

// --- parseSortSQL ---

func TestParseSortSQLAscending(t *testing.T) {
	sc := testSchema()
	tbl := sc.TableByName("users")
	result := parseSortSQL(tbl, "email")
	testutil.Contains(t, result, `"email" ASC`)
}

func TestParseSortSQLDescending(t *testing.T) {
	sc := testSchema()
	tbl := sc.TableByName("users")
	result := parseSortSQL(tbl, "-email")
	testutil.Contains(t, result, `"email" DESC`)
}

func TestParseSortSQLSkipsUnknownColumns(t *testing.T) {
	sc := testSchema()
	tbl := sc.TableByName("users")
	result := parseSortSQL(tbl, "nonexistent")
	testutil.Equal(t, "", result)
}

// --- mapPGError ---

func TestMapPGErrorNilError(t *testing.T) {
	w := httptest.NewRecorder()
	handled := mapPGError(w, nil)
	testutil.True(t, !handled, "nil error should not be handled")
}

func TestMapPGErrorNoRows(t *testing.T) {
	w := httptest.NewRecorder()
	handled := mapPGError(w, pgx.ErrNoRows)
	testutil.True(t, handled, "ErrNoRows should be handled")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestMapPGErrorUniqueViolation(t *testing.T) {
	w := httptest.NewRecorder()
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key", Detail: "already exists"}
	handled := mapPGError(w, pgErr)
	testutil.True(t, handled, "unique violation should be handled")
	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestMapPGErrorForeignKeyViolation(t *testing.T) {
	w := httptest.NewRecorder()
	pgErr := &pgconn.PgError{Code: "23503", ConstraintName: "fk_user", Detail: "not present"}
	handled := mapPGError(w, pgErr)
	testutil.True(t, handled, "FK violation should be handled")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMapPGErrorNotNullViolation(t *testing.T) {
	w := httptest.NewRecorder()
	pgErr := &pgconn.PgError{Code: "23502", ColumnName: "email", Message: "null value"}
	handled := mapPGError(w, pgErr)
	testutil.True(t, handled, "not-null violation should be handled")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMapPGErrorCheckViolation(t *testing.T) {
	w := httptest.NewRecorder()
	pgErr := &pgconn.PgError{Code: "23514", ConstraintName: "positive_amount", Detail: "check failed"}
	handled := mapPGError(w, pgErr)
	testutil.True(t, handled, "check violation should be handled")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMapPGErrorInvalidText(t *testing.T) {
	w := httptest.NewRecorder()
	pgErr := &pgconn.PgError{Code: "22P02", Message: "invalid input syntax"}
	handled := mapPGError(w, pgErr)
	testutil.True(t, handled, "invalid text should be handled")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMapPGErrorUnknownCode(t *testing.T) {
	w := httptest.NewRecorder()
	pgErr := &pgconn.PgError{Code: "99999"}
	handled := mapPGError(w, pgErr)
	testutil.True(t, !handled, "unknown PG error code should not be handled")
}

// --- Content-Type on responses ---

func TestErrorResponseIsJSON(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent", "")
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))
}
