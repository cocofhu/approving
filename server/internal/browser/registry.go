package browser

import (
	"sort"
	"time"
)

// tabInfo tracks one live tab for accounting (no browser handles here).
type tabInfo struct {
	sessionID  string
	container  string
	lastActive time.Time
}

// tabRegistry is the pure, lock-free-by-caller accounting core for the tab pool:
// how many tabs exist, per container, and which are idle/LRU. The Service holds
// the actual browser resources in parallel and always mutates them together with
// the registry under the Service lock. Kept dependency-free so it is unit-tested
// without a real browser.
type tabRegistry struct {
	maxTabs         int
	maxPerContainer int
	tabs            map[string]*tabInfo // sessionID -> info
	perContainer    map[string]int      // container -> tab count
	emptySince      map[string]time.Time
	now             func() time.Time
}

func newTabRegistry(maxTabs, maxPerContainer int) *tabRegistry {
	return &tabRegistry{
		maxTabs:         maxTabs,
		maxPerContainer: maxPerContainer,
		tabs:            map[string]*tabInfo{},
		perContainer:    map[string]int{},
		emptySince:      map[string]time.Time{},
		now:             time.Now,
	}
}

func (r *tabRegistry) count() int { return len(r.tabs) }

func (r *tabRegistry) containerCount(name string) int { return r.perContainer[name] }

// full reports whether the global tab cap is reached.
func (r *tabRegistry) full() bool { return len(r.tabs) >= r.maxTabs }

// add registers a new tab. The container's empty marker is cleared.
func (r *tabRegistry) add(sessionID, container string) {
	r.tabs[sessionID] = &tabInfo{sessionID: sessionID, container: container, lastActive: r.now()}
	r.perContainer[container]++
	delete(r.emptySince, container)
}

// remove drops a tab; when its container hits zero tabs, it is marked empty as
// of now (for later idle reclamation).
func (r *tabRegistry) remove(sessionID string) (container string, ok bool) {
	t, ok := r.tabs[sessionID]
	if !ok {
		return "", false
	}
	delete(r.tabs, sessionID)
	r.perContainer[t.container]--
	if r.perContainer[t.container] <= 0 {
		r.perContainer[t.container] = 0
		r.emptySince[t.container] = r.now()
	}
	return t.container, true
}

func (r *tabRegistry) touch(sessionID string) {
	if t, ok := r.tabs[sessionID]; ok {
		t.lastActive = r.now()
	}
}

// lru returns the least-recently-active session, used to evict when at the cap.
func (r *tabRegistry) lru() (sessionID string, ok bool) {
	var oldest *tabInfo
	for _, t := range r.tabs {
		if oldest == nil || t.lastActive.Before(oldest.lastActive) {
			oldest = t
		}
	}
	if oldest == nil {
		return "", false
	}
	return oldest.sessionID, true
}

// idle returns sessions with no activity for at least ttl.
func (r *tabRegistry) idle(ttl time.Duration) []string {
	if ttl <= 0 {
		return nil
	}
	cutoff := r.now().Add(-ttl)
	var out []string
	for id, t := range r.tabs {
		if t.lastActive.Before(cutoff) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// reapableContainers returns sandboxes that have held zero tabs for at least
// ttl (candidates for dropping the cached CDP engine).
func (r *tabRegistry) reapableContainers(ttl time.Duration) []string {
	if ttl <= 0 {
		return nil
	}
	cutoff := r.now().Add(-ttl)
	var out []string
	for c, since := range r.emptySince {
		if r.perContainer[c] == 0 && since.Before(cutoff) {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// forgetContainer clears all accounting for a container (after it is removed).
func (r *tabRegistry) forgetContainer(name string) {
	delete(r.perContainer, name)
	delete(r.emptySince, name)
}
