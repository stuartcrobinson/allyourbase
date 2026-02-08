package schema

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRelkindToString(t *testing.T) {
	tests := []struct {
		relkind string
		want    string
	}{
		{"r", "table"},
		{"v", "view"},
		{"m", "materialized_view"},
		{"p", "partitioned_table"},
		{"x", "table"}, // unknown defaults to table
		{"", "table"},
	}
	for _, tt := range tests {
		t.Run(tt.relkind+"->"+tt.want, func(t *testing.T) {
			testutil.Equal(t, relkindToString(tt.relkind), tt.want)
		})
	}
}

func TestFkActionToString(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{"a", "NO ACTION"},
		{"r", "RESTRICT"},
		{"c", "CASCADE"},
		{"n", "SET NULL"},
		{"d", "SET DEFAULT"},
		{"x", "NO ACTION"}, // unknown defaults to NO ACTION
		{"", "NO ACTION"},
	}
	for _, tt := range tests {
		t.Run(tt.action+"->"+tt.want, func(t *testing.T) {
			testutil.Equal(t, fkActionToString(tt.action), tt.want)
		})
	}
}

func TestTableByName(t *testing.T) {
	sc := &SchemaCache{
		Tables: map[string]*Table{
			"public.users": {Schema: "public", Name: "users"},
			"public.posts": {Schema: "public", Name: "posts"},
			"other.items":  {Schema: "other", Name: "items"},
		},
	}

	t.Run("finds public table by name", func(t *testing.T) {
		tbl := sc.TableByName("users")
		testutil.NotNil(t, tbl)
		testutil.Equal(t, tbl.Name, "users")
		testutil.Equal(t, tbl.Schema, "public")
	})

	t.Run("finds non-public table by fallback scan", func(t *testing.T) {
		tbl := sc.TableByName("items")
		testutil.NotNil(t, tbl)
		testutil.Equal(t, tbl.Name, "items")
		testutil.Equal(t, tbl.Schema, "other")
	})

	t.Run("returns nil for missing table", func(t *testing.T) {
		tbl := sc.TableByName("nonexistent")
		testutil.True(t, tbl == nil, "expected nil for nonexistent table")
	})

	t.Run("prefers public schema", func(t *testing.T) {
		sc2 := &SchemaCache{
			Tables: map[string]*Table{
				"public.data": {Schema: "public", Name: "data"},
				"other.data":  {Schema: "other", Name: "data"},
			},
		}
		tbl := sc2.TableByName("data")
		testutil.NotNil(t, tbl)
		testutil.Equal(t, tbl.Schema, "public")
	})
}

func TestColumnByName(t *testing.T) {
	tbl := &Table{
		Columns: []*Column{
			{Name: "id", Position: 1},
			{Name: "name", Position: 2},
			{Name: "email", Position: 3},
		},
	}

	t.Run("finds existing column", func(t *testing.T) {
		col := tbl.ColumnByName("name")
		testutil.NotNil(t, col)
		testutil.Equal(t, col.Name, "name")
		testutil.Equal(t, col.Position, 2)
	})

	t.Run("returns nil for missing column", func(t *testing.T) {
		col := tbl.ColumnByName("nonexistent")
		testutil.True(t, col == nil, "expected nil for missing column")
	})
}

func TestTableList(t *testing.T) {
	sc := &SchemaCache{
		Tables: map[string]*Table{
			"public.a": {Schema: "public", Name: "a"},
			"public.b": {Schema: "public", Name: "b"},
			"other.c":  {Schema: "other", Name: "c"},
		},
	}

	tables := sc.TableList()
	testutil.SliceLen(t, tables, 3)
}

func TestDeriveFieldName(t *testing.T) {
	tests := []struct {
		name      string
		columns   []string
		refTable  string
		want      string
	}{
		{
			name:     "single column with _id suffix",
			columns:  []string{"author_id"},
			refTable: "users",
			want:     "author",
		},
		{
			name:     "single column without _id suffix",
			columns:  []string{"creator"},
			refTable: "users",
			want:     "users",
		},
		{
			name:     "composite FK uses table name",
			columns:  []string{"org_id", "team_id"},
			refTable: "teams",
			want:     "teams",
		},
		{
			name:     "user_id maps to user",
			columns:  []string{"user_id"},
			refTable: "users",
			want:     "user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveFieldName(tt.columns, tt.refTable)
			testutil.Equal(t, got, tt.want)
		})
	}
}

func TestBuildRelationships(t *testing.T) {
	tables := map[string]*Table{
		"public.users": {Schema: "public", Name: "users"},
		"public.posts": {
			Schema: "public",
			Name:   "posts",
			ForeignKeys: []*ForeignKey{
				{
					ConstraintName:    "fk_posts_author",
					Columns:           []string{"author_id"},
					ReferencedSchema:  "public",
					ReferencedTable:   "users",
					ReferencedColumns: []string{"id"},
				},
			},
		},
	}

	buildRelationships(tables)

	// posts should have a many-to-one relationship to users.
	posts := tables["public.posts"]
	testutil.SliceLen(t, posts.Relationships, 1)
	testutil.Equal(t, posts.Relationships[0].Type, "many-to-one")
	testutil.Equal(t, posts.Relationships[0].ToTable, "users")
	testutil.Equal(t, posts.Relationships[0].FieldName, "author")

	// users should have a one-to-many relationship from posts.
	users := tables["public.users"]
	testutil.SliceLen(t, users.Relationships, 1)
	testutil.Equal(t, users.Relationships[0].Type, "one-to-many")
	testutil.Equal(t, users.Relationships[0].ToTable, "posts")
	testutil.Equal(t, users.Relationships[0].FieldName, "posts")
}

func TestSchemaFilter(t *testing.T) {
	clause, args := schemaFilter("n", 1)

	// Should exclude information_schema, pg_catalog, pg_toast, and pg_% pattern.
	testutil.Contains(t, clause, "n.nspname != $1")
	testutil.Contains(t, clause, "n.nspname NOT LIKE")
	testutil.True(t, len(args) == 4, "expected 4 args")

	// Args should contain the excluded schema names.
	found := map[string]bool{}
	for _, a := range args {
		if s, ok := a.(string); ok {
			found[s] = true
		}
	}
	testutil.True(t, found["information_schema"], "missing information_schema")
	testutil.True(t, found["pg_catalog"], "missing pg_catalog")
	testutil.True(t, found["pg_toast"], "missing pg_toast")
	testutil.True(t, found["pg_%"], "missing pg_% pattern")
}

func TestSchemaFilterParamOffset(t *testing.T) {
	clause, args := schemaFilter("s", 5)

	testutil.Contains(t, clause, "s.nspname != $5")
	testutil.Contains(t, clause, "s.nspname != $6")
	testutil.Contains(t, clause, "s.nspname != $7")
	testutil.Contains(t, clause, "s.nspname NOT LIKE $8")
	testutil.True(t, len(args) == 4, "expected 4 args")
}
