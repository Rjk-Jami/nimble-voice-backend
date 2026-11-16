package server

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPresenceStore_SingleConnection(t *testing.T) {
	store := NewMemoryPresenceStore()

	if got := store.OnlineCount(); got != 0 {
		t.Fatalf("initial online count = %d, want 0", got)
	}
	if got := store.TotalConnections(); got != 0 {
		t.Fatalf("initial total connections = %d, want 0", got)
	}

	first, count := store.AddConnection("user-1", "conn-1")
	if !first || count != 1 {
		t.Fatalf("AddConnection single = (%v, %d), want (true, 1)", first, count)
	}
	if !store.IsUserOnline("user-1") {
		t.Fatal("expected user-1 to be online")
	}
	if store.TotalConnections() != 1 {
		t.Fatalf("total connections = %d, want 1", store.TotalConnections())
	}
	if uid := store.UserByConnection("conn-1"); uid != "user-1" {
		t.Fatalf("UserByConnection(conn-1) = %q, want user-1", uid)
	}

	last, count := store.RemoveConnection("user-1", "conn-1")
	if !last || count != 0 {
		t.Fatalf("RemoveConnection single = (%v, %d), want (true, 0)", last, count)
	}
	if store.IsUserOnline("user-1") {
		t.Fatal("expected user-1 to be offline")
	}
	if store.OnlineCount() != 0 {
		t.Fatalf("final online count = %d, want 0", store.OnlineCount())
	}
	if store.TotalConnections() != 0 {
		t.Fatalf("final total connections = %d, want 0", store.TotalConnections())
	}
}

func TestPresenceStore_MultiTabDeduplication(t *testing.T) {
	store := NewMemoryPresenceStore()

	// Tab 1 connects
	first, count := store.AddConnection("user-1", "tab-1")
	if !first || count != 1 {
		t.Fatalf("tab-1 connect = (%v, %d), want (true, 1)", first, count)
	}

	// Tab 2 connects
	first, count = store.AddConnection("user-1", "tab-2")
	if first || count != 1 {
		t.Fatalf("tab-2 connect = (%v, %d), want (false, 1)", first, count)
	}

	// Tab 3 connects
	first, count = store.AddConnection("user-1", "tab-3")
	if first || count != 1 {
		t.Fatalf("tab-3 connect = (%v, %d), want (false, 1)", first, count)
	}

	if store.OnlineCount() != 1 {
		t.Fatalf("online count = %d, want 1", store.OnlineCount())
	}
	if store.TotalConnections() != 3 {
		t.Fatalf("total connections = %d, want 3", store.TotalConnections())
	}

	conns := store.UserConnections("user-1")
	if len(conns) != 3 {
		t.Fatalf("UserConnections len = %d, want 3", len(conns))
	}

	// Tab 1 closes
	last, count := store.RemoveConnection("user-1", "tab-1")
	if last || count != 1 {
		t.Fatalf("tab-1 disconnect = (%v, %d), want (false, 1)", last, count)
	}

	// Tab 2 closes (omit userID to test reverse-index lookup)
	last, count = store.RemoveConnection("", "tab-2")
	if last || count != 1 {
		t.Fatalf("tab-2 disconnect with empty userID = (%v, %d), want (false, 1)", last, count)
	}

	// Tab 3 closes
	last, count = store.RemoveConnection("user-1", "tab-3")
	if !last || count != 0 {
		t.Fatalf("tab-3 disconnect = (%v, %d), want (true, 0)", last, count)
	}

	if store.OnlineCount() != 0 {
		t.Fatalf("online count after all tabs closed = %d, want 0", store.OnlineCount())
	}
}

func TestPresenceStore_MultiUser(t *testing.T) {
	store := NewMemoryPresenceStore()

	// User A connects 2 tabs
	fA1, cA1 := store.AddConnection("user-a", "a-1")
	if !fA1 || cA1 != 1 {
		t.Fatalf("user-a tab-1: want (true, 1), got (%v, %d)", fA1, cA1)
	}
	fA2, cA2 := store.AddConnection("user-a", "a-2")
	if fA2 || cA2 != 1 {
		t.Fatalf("user-a tab-2: want (false, 1), got (%v, %d)", fA2, cA2)
	}

	// User B connects 1 tab
	fB1, cB1 := store.AddConnection("user-b", "b-1")
	if !fB1 || cB1 != 2 {
		t.Fatalf("user-b tab-1: want (true, 2), got (%v, %d)", fB1, cB1)
	}

	// User C connects 3 tabs
	store.AddConnection("user-c", "c-1")
	store.AddConnection("user-c", "c-2")
	store.AddConnection("user-c", "c-3")

	if store.OnlineCount() != 3 {
		t.Fatalf("online count = %d, want 3", store.OnlineCount())
	}
	if store.TotalConnections() != 6 {
		t.Fatalf("total connections = %d, want 6", store.TotalConnections())
	}

	// User B disconnects completely
	lB, count := store.RemoveConnection("user-b", "b-1")
	if !lB || count != 2 {
		t.Fatalf("user-b disconnect: want (true, 2), got (%v, %d)", lB, count)
	}

	// User A closes 1 tab
	lA, count := store.RemoveConnection("user-a", "a-1")
	if lA || count != 2 {
		t.Fatalf("user-a tab-1 disconnect: want (false, 2), got (%v, %d)", lA, count)
	}

	// User A closes second tab
	lA, count = store.RemoveConnection("user-a", "a-2")
	if !lA || count != 1 {
		t.Fatalf("user-a tab-2 disconnect: want (true, 1), got (%v, %d)", lA, count)
	}

	if !store.IsUserOnline("user-c") {
		t.Fatal("user-c should still be online")
	}
	if store.IsUserOnline("user-a") {
		t.Fatal("user-a should be offline")
	}
}

func TestPresenceStore_Concurrency(t *testing.T) {
	store := NewMemoryPresenceStore()
	const numUsers = 20
	const tabsPerUser = 5

	var wg sync.WaitGroup

	// Concurrently connect all tabs
	for u := 0; u < numUsers; u++ {
		userID := fmt.Sprintf("concur-user-%d", u)
		for tab := 0; tab < tabsPerUser; tab++ {
			wg.Add(1)
			connID := fmt.Sprintf("conn-%d-%d", u, tab)
			go func(uid, cid string) {
				defer wg.Done()
				store.AddConnection(uid, cid)
				store.Touch(cid)
				_ = store.IsUserOnline(uid)
				_ = store.OnlineCount()
				_ = store.UserByConnection(cid)
			}(userID, connID)
		}
	}
	wg.Wait()

	if store.OnlineCount() != numUsers {
		t.Fatalf("online count after concurrent add = %d, want %d", store.OnlineCount(), numUsers)
	}
	if store.TotalConnections() != numUsers*tabsPerUser {
		t.Fatalf("total connections = %d, want %d", store.TotalConnections(), numUsers*tabsPerUser)
	}

	// Concurrently disconnect all tabs
	for u := 0; u < numUsers; u++ {
		userID := fmt.Sprintf("concur-user-%d", u)
		for tab := 0; tab < tabsPerUser; tab++ {
			wg.Add(1)
			connID := fmt.Sprintf("conn-%d-%d", u, tab)
			go func(uid, cid string) {
				defer wg.Done()
				store.RemoveConnection(uid, cid)
			}(userID, connID)
		}
	}
	wg.Wait()

	if store.OnlineCount() != 0 {
		t.Fatalf("online count after concurrent remove = %d, want 0", store.OnlineCount())
	}
	if store.TotalConnections() != 0 {
		t.Fatalf("total connections = %d, want 0", store.TotalConnections())
	}
}

func TestPresenceStore_StalePruning(t *testing.T) {
	memStore := NewMemoryPresenceStore().(*memoryPresenceStore)

	// Add user-1 with 2 connections, one old, one fresh
	memStore.AddConnection("user-1", "conn-stale-1")
	memStore.AddConnection("user-1", "conn-fresh-1")

	// Add user-2 with only 1 stale connection
	memStore.AddConnection("user-2", "conn-stale-2")

	// Manually backdate the stale connections
	memStore.mu.Lock()
	staleTime := time.Now().Add(-2 * time.Hour)
	memStore.users["user-1"]["conn-stale-1"] = staleTime
	memStore.users["user-2"]["conn-stale-2"] = staleTime
	memStore.mu.Unlock()

	prunedSockets, offlineUsers, newCount := memStore.PruneStale(1 * time.Hour)
	if prunedSockets != 2 {
		t.Fatalf("pruned sockets = %d, want 2", prunedSockets)
	}
	if len(offlineUsers) != 1 || offlineUsers[0] != "user-2" {
		t.Fatalf("offline users = %v, want [user-2]", offlineUsers)
	}
	if newCount != 1 {
		t.Fatalf("new online count = %d, want 1", newCount)
	}

	// user-1 should still have conn-fresh-1
	if !memStore.IsUserOnline("user-1") {
		t.Fatal("user-1 should still be online")
	}
	if memStore.IsUserOnline("user-2") {
		t.Fatal("user-2 should be offline")
	}
	if memStore.UserByConnection("conn-stale-1") != "" {
		t.Fatal("conn-stale-1 should have been deleted from reverse index")
	}
}

func TestPresenceStore_InvalidInputs(t *testing.T) {
	store := NewMemoryPresenceStore()

	first, count := store.AddConnection("", "conn-1")
	if first || count != 0 {
		t.Fatalf("AddConnection empty userID: got (%v, %d), want (false, 0)", first, count)
	}

	first, count = store.AddConnection("user-1", "")
	if first || count != 0 {
		t.Fatalf("AddConnection empty connID: got (%v, %d), want (false, 0)", first, count)
	}

	last, count := store.RemoveConnection("", "nonexistent")
	if last || count != 0 {
		t.Fatalf("RemoveConnection nonexistent: got (%v, %d), want (false, 0)", last, count)
	}
}
