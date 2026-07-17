package peerstore

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) PeerStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "peerstore.db")
	st, err := Open(Config{Path: path, Observer: "self-node"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestUpsertAndForServiceKey(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	success := time.Now().Add(-time.Minute)
	// verified peer
	if err := st.Upsert(ctx, Entry{
		ServiceKey: "aa", PeerID: "peerVerified",
		Multiaddrs: []string{"/ip4/1.2.3.4/tcp/4001/p2p/peerVerified"},
		Source:     SourceService, LastSuccess: &success,
	}); err != nil {
		t.Fatalf("upsert verified: %v", err)
	}
	// candidate-only peer (never dialed)
	if err := st.Upsert(ctx, Entry{
		ServiceKey: "aa", PeerID: "peerCandidate",
		Multiaddrs: []string{"/ip4/5.6.7.8/tcp/4001/p2p/peerCandidate"},
		Source:     SourceImported,
	}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}

	got, err := st.ForServiceKey(ctx, "aa", 0)
	if err != nil {
		t.Fatalf("ForServiceKey: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	// Verified peer must rank first.
	if got[0].PeerID != "peerVerified" {
		t.Fatalf("want peerVerified first, got %q", got[0].PeerID)
	}
	if got[0].Observer != "self-node" {
		t.Fatalf("observer default not applied: %q", got[0].Observer)
	}
	if len(got[0].Multiaddrs) != 1 || got[0].Multiaddrs[0] != "/ip4/1.2.3.4/tcp/4001/p2p/peerVerified" {
		t.Fatalf("multiaddrs round-trip failed: %v", got[0].Multiaddrs)
	}
	if got[1].LastSuccess != nil {
		t.Fatalf("candidate should have nil LastSuccess")
	}
}

func TestUpsertDoesNotClobberSuccessWithNil(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	success := time.Now().Add(-time.Hour)
	mustUpsert(t, st, Entry{ServiceKey: "bb", PeerID: "p", LastSuccess: &success, Multiaddrs: []string{"/a"}})
	// Re-observe as a candidate (nil success) with a fresh addr set.
	mustUpsert(t, st, Entry{ServiceKey: "bb", PeerID: "p", Multiaddrs: []string{"/b"}})

	got, _ := st.ForServiceKey(ctx, "bb", 0)
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].LastSuccess == nil {
		t.Fatalf("nil observation clobbered existing success")
	}
	if got[0].Multiaddrs[0] != "/b" {
		t.Fatalf("multiaddrs should update to newest: %v", got[0].Multiaddrs)
	}
}

func TestMarkSuccess(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	mustUpsert(t, st, Entry{ServiceKey: "cc", PeerID: "p", Multiaddrs: []string{"/a"}})

	when := time.Now().Add(-5 * time.Second)
	if err := st.MarkSuccess(ctx, "cc", "p", when); err != nil {
		t.Fatalf("MarkSuccess: %v", err)
	}
	got, _ := st.ForServiceKey(ctx, "cc", 0)
	if got[0].LastSuccess == nil || got[0].LastSuccess.Unix() != when.Unix() {
		t.Fatalf("MarkSuccess did not set last_success")
	}
}

func TestAllPerKeyCap(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	now := time.Now()
	for i, id := range []string{"p1", "p2", "p3"} {
		ts := now.Add(time.Duration(-i) * time.Minute)
		mustUpsert(t, st, Entry{ServiceKey: "dd", PeerID: id, LastSuccess: &ts, Multiaddrs: []string{"/a"}})
	}
	mustUpsert(t, st, Entry{ServiceKey: "ee", PeerID: "q1", Multiaddrs: []string{"/a"}})

	got, err := st.All(ctx, 2)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	byKey := map[string]int{}
	for _, e := range got {
		byKey[e.ServiceKey]++
	}
	if byKey["dd"] != 2 {
		t.Fatalf("per-key cap not applied for dd: got %d", byKey["dd"])
	}
	if byKey["ee"] != 1 {
		t.Fatalf("ee should have 1 entry, got %d", byKey["ee"])
	}
}

func TestEvict(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	old := time.Now().Add(-48 * time.Hour)
	fresh := time.Now()
	mustUpsert(t, st, Entry{ServiceKey: "ff", PeerID: "stale", LastSeen: old, Multiaddrs: []string{"/a"}})
	mustUpsert(t, st, Entry{ServiceKey: "ff", PeerID: "fresh", LastSeen: fresh, Multiaddrs: []string{"/a"}})

	n, err := st.Evict(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 evicted, got %d", n)
	}
	got, _ := st.ForServiceKey(ctx, "ff", 0)
	if len(got) != 1 || got[0].PeerID != "fresh" {
		t.Fatalf("wrong entry survived eviction: %v", got)
	}
}

func TestCapPerServiceKey(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	now := time.Now()
	// 5 entries for key "gg", 2 for "hh".
	for i := 0; i < 5; i++ {
		ts := now.Add(time.Duration(-i) * time.Minute)
		mustUpsert(t, st, Entry{ServiceKey: "gg", PeerID: fmt.Sprintf("p%d", i), LastSuccess: &ts, Multiaddrs: []string{"/a"}})
	}
	for i := 0; i < 2; i++ {
		mustUpsert(t, st, Entry{ServiceKey: "hh", PeerID: fmt.Sprintf("q%d", i), Multiaddrs: []string{"/a"}})
	}

	removed, err := st.CapPerServiceKey(ctx, 3)
	if err != nil {
		t.Fatalf("CapPerServiceKey: %v", err)
	}
	if removed != 2 {
		t.Fatalf("want 2 removed (5->3 for gg), got %d", removed)
	}
	gg, _ := st.ForServiceKey(ctx, "gg", 0)
	if len(gg) != 3 {
		t.Fatalf("gg should be capped to 3, got %d", len(gg))
	}
	// The kept entries must be the most-recently-successful (p0,p1,p2).
	kept := map[string]bool{}
	for _, e := range gg {
		kept[e.PeerID] = true
	}
	for _, id := range []string{"p0", "p1", "p2"} {
		if !kept[id] {
			t.Fatalf("expected %s to survive cap, kept=%v", id, kept)
		}
	}
	hh, _ := st.ForServiceKey(ctx, "hh", 0)
	if len(hh) != 2 {
		t.Fatalf("hh under cap should be untouched, got %d", len(hh))
	}
}

func TestExternalDSNUnsupported(t *testing.T) {
	if _, err := Open(Config{DSN: "postgres://x"}); err != ErrExternalUnsupported {
		t.Fatalf("want ErrExternalUnsupported, got %v", err)
	}
}

func mustUpsert(t *testing.T, st PeerStore, e Entry) {
	t.Helper()
	if err := st.Upsert(context.Background(), e); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}
