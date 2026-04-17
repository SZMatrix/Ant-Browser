package extension

import (
	"sort"
	"testing"
)

func TestTrackerMarkAndSnapshot(t *testing.T) {
	tr := NewPendingRestartTracker()
	tr.Mark("ext-1", "p1")
	tr.Mark("ext-1", "p2")
	tr.Mark("ext-2", "p1")

	snap := tr.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("want 2 ext keys, got %d (%+v)", len(snap), snap)
	}
	ext1 := snap["ext-1"]
	sort.Strings(ext1)
	if len(ext1) != 2 || ext1[0] != "p1" || ext1[1] != "p2" {
		t.Fatalf("ext-1: %+v", ext1)
	}
	if got := tr.ForExtension("ext-2"); len(got) != 1 || got[0] != "p1" {
		t.Fatalf("ext-2: %+v", got)
	}
}

func TestTrackerClearExtension(t *testing.T) {
	tr := NewPendingRestartTracker()
	tr.Mark("ext-1", "p1")
	tr.Mark("ext-2", "p1")
	tr.ClearExtension("ext-1")
	if _, ok := tr.Snapshot()["ext-1"]; ok {
		t.Fatal("ext-1 should be gone")
	}
	if len(tr.ForExtension("ext-2")) != 1 {
		t.Fatal("ext-2 should remain")
	}
}

func TestTrackerClearProfile(t *testing.T) {
	tr := NewPendingRestartTracker()
	tr.Mark("ext-1", "p1")
	tr.Mark("ext-1", "p2")
	tr.Mark("ext-2", "p1")
	tr.ClearProfile("p1")

	// Snapshot omits ext keys whose sets are empty.
	snap := tr.Snapshot()
	if got := snap["ext-1"]; len(got) != 1 || got[0] != "p2" {
		t.Fatalf("ext-1 after clear: %+v", got)
	}
	if _, ok := snap["ext-2"]; ok {
		t.Fatal("ext-2 set is now empty; Snapshot should omit it")
	}
}

func TestTrackerForExtensionNilWhenEmpty(t *testing.T) {
	tr := NewPendingRestartTracker()
	if tr.ForExtension("missing") != nil {
		t.Fatal("missing ext should be nil slice")
	}
}
