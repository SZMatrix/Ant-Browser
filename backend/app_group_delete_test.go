package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/extension"
	"testing"
)

func TestDeleteGroupPrunesExtensionScopes(t *testing.T) {
	app, cleanup := newTestAppWithExtensions(t)
	defer cleanup()

	g, err := app.browserMgr.GroupDAO.Create(browser.GroupInput{GroupName: "G1"})
	if err != nil {
		t.Fatal(err)
	}

	if err := app.extMgr.Store.Insert(&extension.Extension{
		ExtensionID: "ext-a", Name: "a", SourceType: extension.SourceTypeLocalZIP, Enabled: true,
		Scope: extension.Scope{Kind: extension.ScopeKindGroups, IDs: []string{g.GroupId, "other"}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := app.DeleteGroup(g.GroupId); err != nil {
		t.Fatal(err)
	}

	ext, _ := app.extMgr.Store.GetByID("ext-a")
	if len(ext.Scope.IDs) != 1 || ext.Scope.IDs[0] != "other" {
		t.Fatalf("scope after prune: %+v", ext.Scope)
	}
}
