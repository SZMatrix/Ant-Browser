package extension

import "testing"

func TestPruneProfileFromAllScopes(t *testing.T) {
	s := newTestStore(t)
	_ = s.Insert(&Extension{
		ExtensionID: "e1", Name: "x", SourceType: SourceTypeLocalZIP, Enabled: true,
		Scope: Scope{Kind: ScopeKindInstances, IDs: []string{"p1", "p2", "p3"}},
	})
	_ = s.Insert(&Extension{
		ExtensionID: "e2", Name: "y", SourceType: SourceTypeLocalZIP, Enabled: true,
		Scope: Scope{Kind: ScopeKindInstances, IDs: []string{"p2"}},
	})
	_ = s.Insert(&Extension{
		ExtensionID: "e3", Name: "z", SourceType: SourceTypeLocalZIP, Enabled: true,
		Scope: Scope{Kind: ScopeKindGroups, IDs: []string{"p2"}}, // different kind; must not be touched
	})

	if err := s.PruneProfileFromAllScopes("p2"); err != nil {
		t.Fatal(err)
	}

	e1, _ := s.GetByID("e1")
	if len(e1.Scope.IDs) != 2 || e1.Scope.IDs[0] != "p1" || e1.Scope.IDs[1] != "p3" {
		t.Fatalf("e1 scope: %+v", e1.Scope)
	}
	e2, _ := s.GetByID("e2")
	if len(e2.Scope.IDs) != 0 {
		t.Fatalf("e2 scope should be empty: %+v", e2.Scope)
	}
	e3, _ := s.GetByID("e3")
	if len(e3.Scope.IDs) != 1 {
		t.Fatalf("e3 (groups scope) must be untouched: %+v", e3.Scope)
	}
}

func TestPruneGroupFromAllScopes(t *testing.T) {
	s := newTestStore(t)
	_ = s.Insert(&Extension{
		ExtensionID: "e1", Name: "x", SourceType: SourceTypeLocalZIP, Enabled: true,
		Scope: Scope{Kind: ScopeKindGroups, IDs: []string{"gA", "gB"}},
	})
	_ = s.Insert(&Extension{
		ExtensionID: "e2", Name: "y", SourceType: SourceTypeLocalZIP, Enabled: true,
		Scope: Scope{Kind: ScopeKindInstances, IDs: []string{"gA"}}, // different kind; must not be touched
	})
	if err := s.PruneGroupFromAllScopes("gA"); err != nil {
		t.Fatal(err)
	}
	e1, _ := s.GetByID("e1")
	if len(e1.Scope.IDs) != 1 || e1.Scope.IDs[0] != "gB" {
		t.Fatalf("e1 scope: %+v", e1.Scope)
	}
	e2, _ := s.GetByID("e2")
	if e2.Scope.IDs[0] != "gA" {
		t.Fatalf("instances-scoped ext must be untouched: %+v", e2.Scope)
	}
}

func TestPruneProfileEmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	if err := s.PruneProfileFromAllScopes(""); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneGroupFromAllScopes(""); err != nil {
		t.Fatal(err)
	}
}
