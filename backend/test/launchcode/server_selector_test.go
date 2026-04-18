package launchcode_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/launchcode"
)

func buildTestHandlerWithManager(svc *launchcode.LaunchCodeService, starter launchcode.BrowserStarter, mgr *browser.Manager) http.Handler {
	srv := launchcode.NewLaunchServer(svc, starter, mgr, 0)
	return launchcode.NewTestHandler(srv)
}

func newSelectorTestManager(profiles ...*browser.Profile) *browser.Manager {
	items := make(map[string]*browser.Profile, len(profiles))
	for _, profile := range profiles {
		items[profile.ProfileId] = profile
	}
	return &browser.Manager{
		Profiles: items,
	}
}

// TestLaunchByTagSelectorFiltersCorrectly verifies that profiles can be selected by tag.
func TestLaunchByTagSelectorFiltersCorrectly(t *testing.T) {
	svc := newInMemoryService()
	starter := newMockStarterWithParams()

	profileMatch := &browser.Profile{
		ProfileId:   "profile-tag-match",
		ProfileName: "Tagged Profile",
		Tags:        []string{"电商", "北美"},
		Pid:         9527,
		DebugPort:   9333,
	}
	profileNoMatch := &browser.Profile{
		ProfileId:   "profile-tag-no-match",
		ProfileName: "Other Profile",
		Tags:        []string{"欧洲"},
		Pid:         9528,
		DebugPort:   9334,
	}
	starter.addProfile(profileMatch)
	starter.addProfile(profileNoMatch)
	manager := newSelectorTestManager(profileMatch, profileNoMatch)

	handler := buildTestHandlerWithManager(svc, starter, manager)
	req := httptest.NewRequest(http.MethodPost, "/api/launch", bytes.NewBufferString(`{"selector":{"tags":["电商"]}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d，body=%s", w.Code, w.Body.String())
	}
	if starter.lastProfile != profileMatch.ProfileId {
		t.Fatalf("tag 过滤命中错误实例: got=%s want=%s", starter.lastProfile, profileMatch.ProfileId)
	}
}

// TestLaunchByTagSelectorNoMatchReturns404 verifies 404 when no profile has the given tag.
func TestLaunchByTagSelectorNoMatchReturns404(t *testing.T) {
	svc := newInMemoryService()
	starter := newMockStarterWithParams()

	profile := &browser.Profile{
		ProfileId:   "profile-no-tag",
		ProfileName: "Untagged",
		Tags:        []string{"other"},
		Pid:         9529,
		DebugPort:   9335,
	}
	starter.addProfile(profile)
	manager := newSelectorTestManager(profile)

	handler := buildTestHandlerWithManager(svc, starter, manager)
	req := httptest.NewRequest(http.MethodPost, "/api/launch", bytes.NewBufferString(`{"selector":{"tags":["nonexistent"]}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d，body=%s", w.Code, w.Body.String())
	}
	if starter.lastProfile != "" {
		t.Fatalf("未命中场景不应启动实例: %s", starter.lastProfile)
	}
}
