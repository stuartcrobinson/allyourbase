package realtime

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildVisibilityCheckSinglePK(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
	}
	record := map[string]any{"id": 42, "title": "Hello"}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, `SELECT 1 FROM "public"."posts" WHERE "id" = $1`, query)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, 42, args[0])
}

func TestBuildVisibilityCheckCompositePK(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "order_items",
		PrimaryKey: []string{"order_id", "item_id"},
	}
	record := map[string]any{"order_id": 1, "item_id": 5, "qty": 3}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, `SELECT 1 FROM "public"."order_items" WHERE "order_id" = $1 AND "item_id" = $2`, query)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 1, args[0])
	testutil.Equal(t, 5, args[1])
}

func TestBuildVisibilityCheckMissingPK(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
	}
	record := map[string]any{"title": "Hello"} // no "id"
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, "", query)
	testutil.True(t, args == nil, "args should be nil when PK is missing")
}

func TestCanSeeRecordNilPool(t *testing.T) {
	h := &Handler{pool: nil}
	event := &Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}}
	testutil.True(t, h.canSeeRecord(nil, nil, event), "nil pool should allow all events")
}

func TestCanSeeRecordNilClaims(t *testing.T) {
	h := &Handler{pool: nil}
	event := &Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}}
	testutil.True(t, h.canSeeRecord(nil, nil, event), "nil claims should allow all events")
}

func TestCanSeeRecordDeleteAction(t *testing.T) {
	h := &Handler{pool: nil}
	event := &Event{Action: "delete", Table: "posts", Record: map[string]any{"id": 1}}
	testutil.True(t, h.canSeeRecord(nil, nil, event), "delete events should always be allowed")
}
