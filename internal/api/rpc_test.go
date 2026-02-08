package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func testSchemaWithFunctions() *schema.SchemaCache {
	sc := testSchema()
	sc.Functions = map[string]*schema.Function{
		"public.add_numbers": {
			Schema:     "public",
			Name:       "add_numbers",
			ReturnType: "integer",
			Parameters: []*schema.FuncParam{
				{Name: "a", Type: "integer", Position: 1},
				{Name: "b", Type: "integer", Position: 2},
			},
		},
		"public.get_active_users": {
			Schema:     "public",
			Name:       "get_active_users",
			ReturnType: "SETOF record",
			ReturnsSet: true,
			Parameters: []*schema.FuncParam{
				{Name: "min_age", Type: "integer", Position: 1},
			},
		},
		"public.cleanup_old_data": {
			Schema:     "public",
			Name:       "cleanup_old_data",
			ReturnType: "void",
			IsVoid:     true,
		},
		"public.no_args": {
			Schema:     "public",
			Name:       "no_args",
			ReturnType: "text",
		},
	}
	return sc
}

func rpcRequest(handler http.Handler, funcName string, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest("POST", "/rpc/"+funcName, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest("POST", "/rpc/"+funcName, nil)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// --- Schema not ready ---

func TestRPCSchemaCacheNotReady(t *testing.T) {
	h := testHandler(nil)
	w := rpcRequest(h, "add_numbers", `{"a": 1, "b": 2}`)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "schema cache not ready")
}

// --- Function not found ---

func TestRPCFunctionNotFound(t *testing.T) {
	h := testHandler(testSchemaWithFunctions())
	w := rpcRequest(h, "nonexistent", `{}`)
	testutil.Equal(t, http.StatusNotFound, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "function not found")
}

// --- Invalid body ---

func TestRPCInvalidJSON(t *testing.T) {
	h := testHandler(testSchemaWithFunctions())
	w := rpcRequest(h, "add_numbers", `{broken`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid JSON body")
}

// --- buildRPCCall ---

func TestBuildRPCCallScalar(t *testing.T) {
	fn := &schema.Function{
		Schema:     "public",
		Name:       "add_numbers",
		ReturnType: "integer",
		Parameters: []*schema.FuncParam{
			{Name: "a", Type: "integer", Position: 1},
			{Name: "b", Type: "integer", Position: 2},
		},
	}
	query, args, err := buildRPCCall(fn, map[string]any{"a": 1, "b": 2})
	testutil.NoError(t, err)
	testutil.Contains(t, query, `SELECT "public"."add_numbers"($1, $2)`)
	testutil.Equal(t, 2, len(args))
}

func TestBuildRPCCallSetReturning(t *testing.T) {
	fn := &schema.Function{
		Schema:     "public",
		Name:       "get_users",
		ReturnType: "SETOF record",
		ReturnsSet: true,
		Parameters: []*schema.FuncParam{
			{Name: "min_age", Type: "integer", Position: 1},
		},
	}
	query, args, err := buildRPCCall(fn, map[string]any{"min_age": 18})
	testutil.NoError(t, err)
	testutil.Contains(t, query, `SELECT * FROM "public"."get_users"($1)`)
	testutil.Equal(t, 1, len(args))
}

func TestBuildRPCCallNoArgs(t *testing.T) {
	fn := &schema.Function{
		Schema:     "public",
		Name:       "now_utc",
		ReturnType: "timestamptz",
	}
	query, args, err := buildRPCCall(fn, nil)
	testutil.NoError(t, err)
	testutil.Contains(t, query, `SELECT "public"."now_utc"()`)
	testutil.Equal(t, 0, len(args))
}

func TestBuildRPCCallMissingArgPassesNull(t *testing.T) {
	fn := &schema.Function{
		Schema:     "public",
		Name:       "greet",
		ReturnType: "text",
		Parameters: []*schema.FuncParam{
			{Name: "name", Type: "text", Position: 1},
		},
	}
	query, args, err := buildRPCCall(fn, map[string]any{})
	testutil.NoError(t, err)
	testutil.Contains(t, query, `"greet"($1)`)
	testutil.Equal(t, 1, len(args))
	testutil.True(t, args[0] == nil, "missing arg should be nil")
}

func TestBuildRPCCallUnnamedParamErrors(t *testing.T) {
	fn := &schema.Function{
		Schema:     "public",
		Name:       "bad_func",
		ReturnType: "integer",
		Parameters: []*schema.FuncParam{
			{Name: "", Type: "integer", Position: 1},
		},
	}
	_, _, err := buildRPCCall(fn, map[string]any{})
	testutil.True(t, err != nil, "expected error for unnamed params")
	testutil.Contains(t, err.Error(), "unnamed parameters")
}

// --- FunctionByName ---

func TestFunctionByNamePublic(t *testing.T) {
	sc := testSchemaWithFunctions()
	fn := sc.FunctionByName("add_numbers")
	testutil.True(t, fn != nil, "expected to find add_numbers")
	testutil.Equal(t, "add_numbers", fn.Name)
}

func TestFunctionByNameNotFound(t *testing.T) {
	sc := testSchemaWithFunctions()
	fn := sc.FunctionByName("nonexistent")
	testutil.True(t, fn == nil, "expected nil for nonexistent function")
}

func TestFunctionByNameNilMap(t *testing.T) {
	sc := &schema.SchemaCache{}
	fn := sc.FunctionByName("anything")
	testutil.True(t, fn == nil, "expected nil when functions map is nil")
}

// --- Response format ---

func TestRPCErrorResponseIsJSON(t *testing.T) {
	h := testHandler(testSchemaWithFunctions())
	w := rpcRequest(h, "nonexistent", `{}`)
	ct := w.Header().Get("Content-Type")
	testutil.Equal(t, "application/json", ct)

	var resp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.True(t, resp["code"] != nil, "expected error code in response")
}
