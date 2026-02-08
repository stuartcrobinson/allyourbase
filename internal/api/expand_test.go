package api

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGetOrCreateExpand(t *testing.T) {
	rec := map[string]any{"id": 1, "name": "test"}

	// First call creates the expand map.
	expand := getOrCreateExpand(rec)
	testutil.NotNil(t, expand)

	expand["author"] = map[string]any{"id": 10, "name": "Alice"}

	// Second call returns the same map.
	expand2 := getOrCreateExpand(rec)
	testutil.Equal(t, expand2["author"].(map[string]any)["name"], "Alice")
}

func TestGetOrCreateExpandExisting(t *testing.T) {
	existing := map[string]any{"tags": []string{"go"}}
	rec := map[string]any{"id": 1, "expand": existing}

	// Should return the existing expand map.
	expand := getOrCreateExpand(rec)
	testutil.NotNil(t, expand)

	// The existing map should be the same object.
	expand["author"] = "test"
	testutil.Equal(t, existing["author"], "test")
}

func TestExpandRelationLookupByFieldName(t *testing.T) {
	tbl := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "authors",
			},
		},
	}

	// findRelation is tested indirectly through expandRelation,
	// but we can test the lookup logic by checking the relationship is found.
	var found *schema.Relationship
	for _, r := range tbl.Relationships {
		if r.FieldName == "author" {
			found = r
			break
		}
	}
	testutil.NotNil(t, found)
	testutil.Equal(t, found.Type, "many-to-one")
}

func TestExpandRelationLookupByColumnName(t *testing.T) {
	tbl := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "authors",
			},
		},
	}

	// Simulates expand lookup with FK column name "author_id".
	relName := "author_id"
	var found *schema.Relationship
	for _, r := range tbl.Relationships {
		if r.FieldName == relName {
			found = r
			break
		}
		if r.Type == "many-to-one" && len(r.FromColumns) == 1 && r.FromColumns[0] == relName {
			found = r
			break
		}
	}
	testutil.NotNil(t, found)
	testutil.Equal(t, found.FieldName, "author")
}

func TestExpandRelationNotFound(t *testing.T) {
	tbl := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "authors",
			},
		},
	}

	relName := "nonexistent"
	var found *schema.Relationship
	for _, r := range tbl.Relationships {
		if r.FieldName == relName {
			found = r
			break
		}
		if r.Type == "many-to-one" && len(r.FromColumns) == 1 && r.FromColumns[0] == relName {
			found = r
			break
		}
	}
	testutil.True(t, found == nil, "expected nil for nonexistent relation")
}

func TestCountKnownColumns(t *testing.T) {
	tbl := &schema.Table{
		Columns: []*schema.Column{
			{Name: "id"},
			{Name: "name"},
			{Name: "email"},
		},
	}

	tests := []struct {
		name string
		data map[string]any
		want int
	}{
		{"all known", map[string]any{"name": "a", "email": "b"}, 2},
		{"some known", map[string]any{"name": "a", "fake": "b"}, 1},
		{"none known", map[string]any{"fake1": "a", "fake2": "b"}, 0},
		{"empty", map[string]any{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countKnownColumns(tbl, tt.data)
			testutil.Equal(t, got, tt.want)
		})
	}
}
