package extension

import "sync"

type PendingRestartTracker struct {
	mu    sync.RWMutex
	byExt map[string]map[string]struct{}
}

func NewPendingRestartTracker() *PendingRestartTracker {
	return &PendingRestartTracker{byExt: map[string]map[string]struct{}{}}
}

func (t *PendingRestartTracker) Mark(extID, profileID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	set, ok := t.byExt[extID]
	if !ok {
		set = map[string]struct{}{}
		t.byExt[extID] = set
	}
	set[profileID] = struct{}{}
}

func (t *PendingRestartTracker) ClearExtension(extID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byExt, extID)
}

func (t *PendingRestartTracker) ClearProfile(profileID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ext, set := range t.byExt {
		delete(set, profileID)
		if len(set) == 0 {
			delete(t.byExt, ext)
		}
	}
}

func (t *PendingRestartTracker) Snapshot() map[string][]string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string][]string, len(t.byExt))
	for ext, set := range t.byExt {
		if len(set) == 0 {
			continue
		}
		arr := make([]string, 0, len(set))
		for pid := range set {
			arr = append(arr, pid)
		}
		out[ext] = arr
	}
	return out
}

func (t *PendingRestartTracker) ForExtension(extID string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	set := t.byExt[extID]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for pid := range set {
		out = append(out, pid)
	}
	return out
}
