package extension

import (
	"slices"

	"ant-chrome/backend/internal/browser"
)

// Resolve reports whether the extension should be loaded into the given profile.
// The match is strict and non-recursive: parent groups do NOT implicitly cover
// child-group profiles (see spec section 3 for rationale). Any change to this
// semantics MUST update TestResolveDoesNotRecurseIntoChildGroups first.
func Resolve(ext *Extension, profile *browser.Profile) bool {
	if ext == nil || profile == nil || !ext.Enabled {
		return false
	}
	switch ext.Scope.Kind {
	case ScopeKindInstances:
		return slices.Contains(ext.Scope.IDs, profile.ProfileId)
	case ScopeKindGroups:
		if profile.GroupId == "" {
			return false
		}
		return slices.Contains(ext.Scope.IDs, profile.GroupId)
	}
	return false
}

// PruneIDs returns a copy of the scope with any ID present in 'remove' filtered out.
// The kind field is preserved regardless of result.
func PruneIDs(sc Scope, remove map[string]struct{}) Scope {
	if len(sc.IDs) == 0 || len(remove) == 0 {
		return Scope{Kind: sc.Kind, IDs: append([]string(nil), sc.IDs...)}
	}
	out := make([]string, 0, len(sc.IDs))
	for _, id := range sc.IDs {
		if _, drop := remove[id]; drop {
			continue
		}
		out = append(out, id)
	}
	return Scope{Kind: sc.Kind, IDs: out}
}
