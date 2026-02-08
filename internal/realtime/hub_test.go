package realtime_test

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSubscribeAndPublish(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	testutil.Equal(t, hub.ClientCount(), 1)
	testutil.True(t, client.ID != "", "client should have an ID")

	hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 1, "title": "Hello"},
	})

	select {
	case event := <-client.Events():
		testutil.Equal(t, event.Action, "create")
		testutil.Equal(t, event.Table, "posts")
		testutil.Equal(t, event.Record["title"], "Hello")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishToNonSubscribedTable(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "comments",
		Record: map[string]any{"id": 1},
	})

	select {
	case <-client.Events():
		t.Fatal("should not receive event for unsubscribed table")
	case <-time.After(10 * time.Millisecond):
		// Expected: no event received.
	}
}

func TestUnsubscribeRemovesClient(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	testutil.Equal(t, hub.ClientCount(), 1)

	hub.Unsubscribe(client.ID)
	testutil.Equal(t, hub.ClientCount(), 0)

	// Channel should be closed.
	_, ok := <-client.Events()
	testutil.False(t, ok, "channel should be closed after unsubscribe")
}

func TestUnsubscribeIdempotent(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	hub.Unsubscribe(client.ID)
	hub.Unsubscribe(client.ID) // Should not panic.
	testutil.Equal(t, hub.ClientCount(), 0)
}

func TestMultipleClients(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(c1.ID)
	c2 := hub.Subscribe(map[string]bool{"posts": true, "comments": true})
	defer hub.Unsubscribe(c2.ID)
	c3 := hub.Subscribe(map[string]bool{"comments": true})
	defer hub.Unsubscribe(c3.ID)

	testutil.Equal(t, hub.ClientCount(), 3)

	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	// c1 and c2 subscribed to posts.
	for _, c := range []*realtime.Client{c1, c2} {
		select {
		case event := <-c.Events():
			testutil.Equal(t, event.Table, "posts")
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("client %s should receive posts event", c.ID)
		}
	}

	// c3 not subscribed to posts.
	select {
	case <-c3.Events():
		t.Fatal("c3 should not receive posts event")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestPublishMultipleActions(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	events := []realtime.Event{
		{Action: "create", Table: "posts", Record: map[string]any{"id": 1}},
		{Action: "update", Table: "posts", Record: map[string]any{"id": 1, "title": "Updated"}},
		{Action: "delete", Table: "posts", Record: map[string]any{"id": 1}},
	}

	for i := range events {
		hub.Publish(&events[i])
	}

	for _, want := range events {
		select {
		case got := <-client.Events():
			testutil.Equal(t, got.Action, want.Action)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timed out waiting for %s event", want.Action)
		}
	}
}

func TestClientIDsAreUnique(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.Subscribe(map[string]bool{"posts": true})
	c2 := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(c1.ID)
	defer hub.Unsubscribe(c2.ID)

	testutil.NotEqual(t, c1.ID, c2.ID)
}

func TestPublishNoClientsIsNoop(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	// Should not panic.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})
}

func TestBufferFullDropsEvent(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	// Fill the 256-event buffer.
	for i := 0; i < 256; i++ {
		hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": i}})
	}

	// The 257th event should be dropped (non-blocking), not block the publisher.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 257}})

	// Drain and verify we got exactly 256 events.
	count := 0
	for count < 256 {
		select {
		case <-client.Events():
			count++
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("expected 256 events, got %d", count)
		}
	}

	// Channel should be empty now (the 257th was dropped).
	select {
	case <-client.Events():
		t.Fatal("should not receive the dropped event")
	case <-time.After(10 * time.Millisecond):
		// Expected.
	}
}

func TestCloseDisconnectsAllClients(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.Subscribe(map[string]bool{"posts": true})
	c2 := hub.Subscribe(map[string]bool{"comments": true})
	testutil.Equal(t, hub.ClientCount(), 2)

	hub.Close()
	testutil.Equal(t, hub.ClientCount(), 0)

	// Both client channels should be closed.
	_, ok1 := <-c1.Events()
	testutil.False(t, ok1, "c1 channel should be closed")
	_, ok2 := <-c2.Events()
	testutil.False(t, ok2, "c2 channel should be closed")
}

func TestCloseIdempotent(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())

	hub.Subscribe(map[string]bool{"posts": true})
	hub.Close()
	hub.Close() // Should not panic.
	testutil.Equal(t, hub.ClientCount(), 0)
}
