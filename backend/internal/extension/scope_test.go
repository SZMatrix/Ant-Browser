package extension

import (
	"testing"

	"ant-chrome/backend/internal/browser"
)

func TestResolveMatchesByInstance(t *testing.T) {
	ext := &Extension{Enabled: true, Scope: Scope{Kind: ScopeKindInstances, IDs: []string{"p1", "p2"}}}
	if !Resolve(ext, &browser.Profile{ProfileId: "p1"}) {
		t.Fatal("expected p1 to match")
	}
	if Resolve(ext, &browser.Profile{ProfileId: "p3"}) {
		t.Fatal("expected p3 not to match")
	}
}

func TestResolveMatchesByGroupDirect(t *testing.T) {
	ext := &Extension{Enabled: true, Scope: Scope{Kind: ScopeKindGroups, IDs: []string{"gA"}}}
	if !Resolve(ext, &browser.Profile{GroupId: "gA"}) {
		t.Fatal("profile in group gA should match")
	}
	if Resolve(ext, &browser.Profile{GroupId: "gB"}) {
		t.Fatal("profile in group gB should not match")
	}
}

// Explicit decision: parent group in scope.IDs MUST NOT match a profile whose
// groupId is a child of that parent. Guards the 'no hierarchy' rule.
func TestResolveDoesNotRecurseIntoChildGroups(t *testing.T) {
	ext := &Extension{Enabled: true, Scope: Scope{Kind: ScopeKindGroups, IDs: []string{"parent"}}}
	// Child profile: parent of its group is "parent", its own groupId is "child".
	if Resolve(ext, &browser.Profile{GroupId: "child"}) {
		t.Fatal("child profile must not match when only parent groupId is in scope")
	}
}

func TestResolveDisabled(t *testing.T) {
	ext := &Extension{Enabled: false, Scope: Scope{Kind: ScopeKindInstances, IDs: []string{"p1"}}}
	if Resolve(ext, &browser.Profile{ProfileId: "p1"}) {
		t.Fatal("disabled extension must never resolve")
	}
}

func TestResolveEmptyIDs(t *testing.T) {
	ext := &Extension{Enabled: true, Scope: Scope{Kind: ScopeKindInstances, IDs: nil}}
	if Resolve(ext, &browser.Profile{ProfileId: "p1"}) {
		t.Fatal("empty IDs must resolve to false")
	}
}

func TestPruneFiltersIDs(t *testing.T) {
	sc := Scope{Kind: ScopeKindInstances, IDs: []string{"a", "b", "c"}}
	got := PruneIDs(sc, map[string]struct{}{"b": {}}) // remove "b"
	want := []string{"a", "c"}
	if len(got.IDs) != 2 || got.IDs[0] != want[0] || got.IDs[1] != want[1] {
		t.Fatalf("PruneIDs want=%v got=%v", want, got.IDs)
	}
	if got.Kind != ScopeKindInstances {
		t.Fatalf("kind should be preserved")
	}
}
