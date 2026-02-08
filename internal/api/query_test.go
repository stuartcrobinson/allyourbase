package api

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func testTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "users",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", Position: 2, TypeName: "text"},
			{Name: "email", Position: 3, TypeName: "varchar"},
			{Name: "age", Position: 4, TypeName: "integer"},
		},
		PrimaryKey: []string{"id"},
	}
}

func compositePKTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "order_items",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "order_id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "item_id", Position: 2, TypeName: "integer", IsPrimaryKey: true},
			{Name: "quantity", Position: 3, TypeName: "integer"},
		},
		PrimaryKey: []string{"order_id", "item_id"},
	}
}

func TestQuoteIdent(t *testing.T) {
	testutil.Equal(t, quoteIdent("name"), `"name"`)
	testutil.Equal(t, quoteIdent("user name"), `"user name"`)
	testutil.Equal(t, quoteIdent(`say"hello`), `"say""hello"`)
}

func TestTableRef(t *testing.T) {
	tbl := testTable()
	testutil.Equal(t, tableRef(tbl), `"public"."users"`)
}

func TestBuildSelectOne(t *testing.T) {
	tbl := testTable()

	q, args := buildSelectOne(tbl, nil, []string{"42"})
	testutil.Contains(t, q, `SELECT * FROM "public"."users"`)
	testutil.Contains(t, q, `"id" = $1`)
	testutil.Contains(t, q, "LIMIT 1")
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, args[0].(string), "42")
}

func TestBuildSelectOneWithFields(t *testing.T) {
	tbl := testTable()

	q, args := buildSelectOne(tbl, []string{"id", "name"}, []string{"1"})
	testutil.Contains(t, q, `"id", "name"`)
	testutil.Contains(t, q, `"id" = $1`)
	testutil.SliceLen(t, args, 1)
}

func TestBuildSelectOneFieldValidation(t *testing.T) {
	tbl := testTable()

	// Unknown fields should be ignored, falls back to *.
	q, _ := buildSelectOne(tbl, []string{"nonexistent"}, []string{"1"})
	testutil.Contains(t, q, "SELECT *")
}

func TestBuildInsert(t *testing.T) {
	tbl := testTable()

	// Use a single key to keep output deterministic.
	data := map[string]any{"name": "Alice"}
	q, args := buildInsert(tbl, data)
	testutil.Contains(t, q, "INSERT INTO")
	testutil.Contains(t, q, `"name"`)
	testutil.Contains(t, q, "$1")
	testutil.Contains(t, q, "RETURNING *")
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, args[0].(string), "Alice")
}

func TestBuildInsertSkipsUnknownColumns(t *testing.T) {
	tbl := testTable()

	data := map[string]any{"name": "Alice", "nonexistent": "val"}
	_, args := buildInsert(tbl, data)
	// Should only have the valid column.
	testutil.Equal(t, len(args), 1)
}

func TestBuildUpdate(t *testing.T) {
	tbl := testTable()

	data := map[string]any{"name": "Bob"}
	q, args := buildUpdate(tbl, data, []string{"1"})
	testutil.Contains(t, q, "UPDATE")
	testutil.Contains(t, q, "SET")
	testutil.Contains(t, q, `"name" = $1`)
	testutil.Contains(t, q, `"id" = $2`)
	testutil.Contains(t, q, "RETURNING *")
	testutil.SliceLen(t, args, 2)
}

func TestBuildDelete(t *testing.T) {
	tbl := testTable()

	q, args := buildDelete(tbl, []string{"5"})
	testutil.Contains(t, q, "DELETE FROM")
	testutil.Contains(t, q, `"id" = $1`)
	testutil.SliceLen(t, args, 1)
}

func TestBuildPKWhereComposite(t *testing.T) {
	tbl := compositePKTable()

	where, args := buildPKWhere(tbl, []string{"10", "20"})
	testutil.Contains(t, where, `"order_id" = $1`)
	testutil.Contains(t, where, `"item_id" = $2`)
	testutil.SliceLen(t, args, 2)
}

func TestBuildColumnListEmpty(t *testing.T) {
	tbl := testTable()
	testutil.Equal(t, buildColumnList(tbl, nil), "*")
	testutil.Equal(t, buildColumnList(tbl, []string{}), "*")
}

func TestBuildColumnListWithFields(t *testing.T) {
	tbl := testTable()
	result := buildColumnList(tbl, []string{"id", "name"})
	testutil.Contains(t, result, `"id"`)
	testutil.Contains(t, result, `"name"`)
}

func TestBuildList(t *testing.T) {
	tbl := testTable()

	opts := listOpts{
		page:    1,
		perPage: 20,
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)
	testutil.Contains(t, dataQ, "SELECT *")
	testutil.Contains(t, dataQ, "LIMIT $1")
	testutil.Contains(t, dataQ, "OFFSET $2")
	testutil.SliceLen(t, dataArgs, 2)
	testutil.Equal(t, dataArgs[0].(int), 20) // perPage
	testutil.Equal(t, dataArgs[1].(int), 0)  // offset

	testutil.Contains(t, countQ, "SELECT COUNT(*)")
	testutil.SliceLen(t, countArgs, 0)
}

func TestBuildListWithFilter(t *testing.T) {
	tbl := testTable()

	opts := listOpts{
		page:       2,
		perPage:    10,
		filterSQL:  `"name" = $1`,
		filterArgs: []any{"Alice"},
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)
	testutil.Contains(t, dataQ, "WHERE")
	testutil.Contains(t, dataQ, `"name" = $1`)
	testutil.Contains(t, dataQ, "LIMIT $2")
	testutil.Contains(t, dataQ, "OFFSET $3")
	testutil.SliceLen(t, dataArgs, 3) // filter arg + limit + offset
	testutil.Equal(t, dataArgs[0].(string), "Alice")
	testutil.Equal(t, dataArgs[1].(int), 10) // perPage
	testutil.Equal(t, dataArgs[2].(int), 10) // offset (page 2)

	testutil.Contains(t, countQ, "WHERE")
	testutil.SliceLen(t, countArgs, 1)
}

func TestBuildListSkipTotal(t *testing.T) {
	tbl := testTable()

	opts := listOpts{
		page:      1,
		perPage:   20,
		skipTotal: true,
	}

	_, _, countQ, countArgs := buildList(tbl, opts)
	testutil.Equal(t, countQ, "")
	testutil.True(t, countArgs == nil, "countArgs should be nil")
}

func TestBuildListWithSort(t *testing.T) {
	tbl := testTable()

	opts := listOpts{
		page:    1,
		perPage: 20,
		sortSQL: `"name" ASC, "age" DESC`,
	}

	dataQ, _, _, _ := buildList(tbl, opts)
	testutil.Contains(t, dataQ, `ORDER BY "name" ASC, "age" DESC`)
}

func TestParsePKValues(t *testing.T) {
	// Single PK.
	vals := parsePKValues("42", 1)
	testutil.SliceLen(t, vals, 1)
	testutil.Equal(t, vals[0], "42")

	// Composite PK.
	vals = parsePKValues("10,20", 2)
	testutil.SliceLen(t, vals, 2)
	testutil.Equal(t, vals[0], "10")
	testutil.Equal(t, vals[1], "20")
}
