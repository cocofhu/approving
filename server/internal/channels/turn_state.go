package channels

import (
	"strings"
	"sync"
)

// turnState is what one conversation has already been told during the turn in
// flight, and it exists to stop the platform saying the same thing twice.
//
// Three markers rather than one, because three different things are allowed to
// suppress three different things:
//
//   - replied: something substantive reached the user. pm_reply runs in the MCP
//     host and the final summary runs in the manager, so without a shared
//     marker one turn answers twice.
//   - answered: the actual answer was delivered, not merely something visible.
//     A progress milestone sets replied but not this — withholding the answer
//     because a progress line went out first is worse than the double reply the
//     marker exists to prevent.
//   - acknowledged: the conversation layer already said the work is being
//     picked up. Only one thing may be suppressed on the strength of it — the
//     platform's own run-acceptance notice — because suppressing that for any
//     progress line would lose the only confirmation some delegations send.
//
// One lock for all three. They are read and written on the same turn boundary,
// and splitting them would make "did anything reach the user" a question with
// three answers taken at three different instants.
type turnState struct {
	mu           sync.Mutex
	replied      map[string]bool
	answered     map[string]bool
	acknowledged map[string]bool
}

func newTurnState() *turnState {
	return &turnState{
		replied:      map[string]bool{},
		answered:     map[string]bool{},
		acknowledged: map[string]bool{},
	}
}

func (t *turnState) mark(table map[string]bool, scope string) {
	if strings.TrimSpace(scope) == "" {
		return
	}
	t.mu.Lock()
	table[scope] = true
	t.mu.Unlock()
}

func (t *turnState) has(table map[string]bool, scope string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return table[scope]
}

func (t *turnState) markReplied(scope string)      { t.mark(t.replied, scope) }
func (t *turnState) markAnswered(scope string)     { t.mark(t.answered, scope) }
func (t *turnState) markAcknowledged(scope string) { t.mark(t.acknowledged, scope) }

func (t *turnState) hasReplied(scope string) bool      { return t.has(t.replied, scope) }
func (t *turnState) hasAnswered(scope string) bool     { return t.has(t.answered, scope) }
func (t *turnState) hasAcknowledged(scope string) bool { return t.has(t.acknowledged, scope) }

// clearReplied resets what has been said without forgetting the
// acknowledgement. Used where a turn hands off mid-flight: the next stretch may
// speak again, but it must not re-announce work it already announced.
func (t *turnState) clearReplied(scope string) {
	t.mu.Lock()
	delete(t.replied, scope)
	delete(t.answered, scope)
	t.mu.Unlock()
}

// clearTurn ends a turn: nothing it said may suppress anything in the next one.
func (t *turnState) clearTurn(scope string) {
	t.mu.Lock()
	delete(t.replied, scope)
	delete(t.answered, scope)
	delete(t.acknowledged, scope)
	t.mu.Unlock()
}

// The Manager keeps these methods as a facade. Every call site in the package
// and every test that constructs a Manager directly goes through them, so the
// component underneath can move without touching any of them.

func (m *Manager) markReplied(scope string)      { m.turns.markReplied(scope) }
func (m *Manager) markAnswered(scope string)     { m.turns.markAnswered(scope) }
func (m *Manager) markAcknowledged(scope string) { m.turns.markAcknowledged(scope) }

func (m *Manager) hasReplied(scope string) bool      { return m.turns.hasReplied(scope) }
func (m *Manager) hasAnswered(scope string) bool     { return m.turns.hasAnswered(scope) }
func (m *Manager) hasAcknowledged(scope string) bool { return m.turns.hasAcknowledged(scope) }

func (m *Manager) clearReplied(scope string)     { m.turns.clearReplied(scope) }
func (m *Manager) clearTurnMarkers(scope string) { m.turns.clearTurn(scope) }

// MarkConversationReplied lets the MCP host record an explicit agent reply for
// the conversation's current turn.
func (m *Manager) MarkConversationReplied(projectID string, scene Scene, conversationID string) {
	m.markReplied(conversationTurnScope(projectID, scene, conversationID))
}
