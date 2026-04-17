package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/extension"
	"testing"
)

func TestBrowserProfileDeletePrunesExtensionScopes(t *testing.T) {
	app, cleanup := newTestAppWithExtensions(t)
	defer cleanup()

	p, err := app.browserMgr.Create(browser.ProfileInput{ProfileName: "P1"})
	if err != nil {
		t.Fatal(err)
	}

	if err := app.extMgr.Store.Insert(&extension.Extension{
		ExtensionID: "ext-b", Name: "b", SourceType: extension.SourceTypeLocalZIP, Enabled: true,
		Scope: extension.Scope{Kind: extension.ScopeKindInstances, IDs: []string{p.ProfileId, "other"}},
	}); err != nil {
		t.Fatal(err)
	}
	app.extMgr.Tracker.Mark("ext-b", p.ProfileId)

	if err := app.BrowserProfileDelete(p.ProfileId); err != nil {
		t.Fatal(err)
	}

	ext, _ := app.extMgr.Store.GetByID("ext-b")
	if len(ext.Scope.IDs) != 1 || ext.Scope.IDs[0] != "other" {
		t.Fatalf("scope after prune: %+v", ext.Scope)
	}
	snap := app.extMgr.Tracker.Snapshot()
	if len(snap["ext-b"]) != 0 {
		t.Fatalf("tracker should have cleared profile from ext-b: %+v", snap)
	}
}
